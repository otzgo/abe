// Plugin ABE插件，定义插件的基础能力与生命周期钩子
// 每个 abe-plugin 模块都应实现此接口
// Init 会在插件注册时被调用，并注入全局唯一的 Engine 实例
// 其他钩子为可选（通过额外接口声明），由框架在关键阶段触发

package abe

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
)

// Plugin ABE插件
type Plugin interface {
	// Name 返回插件名称
	Name() string

	// Version 返回插件版本
	Version() string

	// Init 在插件注册时调用，用于注入全局 Engine 并完成基础初始化
	// 插件可在此阶段注册控制器、中间件、事件订阅、定时任务等
	Init(engine *Engine) error
}

// BeforeMountHook 在挂载控制器与全局中间件前触发
// 适合做：补充或调整中间件组、最终确定路由注册等
type BeforeMountHook interface {
	OnBeforeMount(engine *Engine) error
}

// BeforeServerStartHook 在 HTTP Server 初始化完成、启动前触发
// 适合做：与外部资源的握手、热数据预加载等
type BeforeServerStartHook interface {
	OnBeforeServerStart(engine *Engine) error
}

// ShutdownHook 在应用优雅退出开始阶段触发（事件总线、协程池仍可用）
// 适合做：释放插件内部资源、取消订阅、持久化缓存等
type ShutdownHook interface {
	OnShutdown(engine *Engine) error
}

// AfterMountHook 在挂载控制器完成后触发
// 适合做：基于已构建好的路由树进行最终索引/校验、输出摘要等
type AfterMountHook interface {
	OnAfterMount(engine *Engine) error
}

// EngineVersionRequirement 可选：声明对 abe 引擎的最低版本要求（SemVer，形如 x.y.z）
type EngineVersionRequirement interface {
	MinEngineVersion() string
}

// PluginManager 插件管理器，负责插件注册与钩子调度
type PluginManager struct {
	mu         sync.RWMutex
	engine     *Engine
	plugins    []Plugin
	index      map[string]Plugin   // key -> plugin
	alias      map[string]string   // key -> alias
	aliasIndex map[string]string   // alias -> key
	nameIndex  map[string][]string // name -> keys
}

func newPluginManager(engine *Engine) *PluginManager {
	return &PluginManager{
		engine:     engine,
		index:      make(map[string]Plugin),
		alias:      make(map[string]string),
		aliasIndex: make(map[string]string),
		nameIndex:  make(map[string][]string),
	}
}

// Register 注册插件，并立即调用其 Init(engine)
// 若同名插件已存在则返回错误，不重复注册
func (pm *PluginManager) Register(p Plugin) error {
	if p == nil {
		return nil
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 计算唯一键（包路径 + 类型名）
	t := reflect.TypeOf(p)
	key := t.PkgPath() + "." + t.Name()
	name := p.Name()

	// 启用配置判定：默认启用，可通过 plugins.enabled 与 plugins.enable.<unique_key> 覆盖
	enabled := true
	if pm.engine.Config().IsSet("plugins.enabled") {
		enabled = pm.engine.Config().GetBool("plugins.enabled")
	}
	perKey := "plugins.enable." + key
	if pm.engine.Config().IsSet(perKey) {
		enabled = pm.engine.Config().GetBool(perKey)
	}
	if !enabled {
		pm.engine.Logger().Info("插件禁用，跳过注册", "name", name, "unique_key", key)
		return nil
	}

	// 重复插件（按唯一键）直接拒绝
	if _, ok := pm.index[key]; ok {
		return ErrDuplicatePlugin(key)
	}

	// 兼容性校验：若插件声明 MinEngineVersion 且当前 abe 版本不满足
	minEngine := ""
	if req, ok := p.(EngineVersionRequirement); ok {
		minEngine = strings.TrimSpace(req.MinEngineVersion())
	}
	if minEngine != "" {
		current, err1 := semver.NewVersion(Version)
		constraint, err2 := semver.NewConstraint(">= " + minEngine)
		if err1 != nil || err2 != nil {
			pm.engine.Logger().Warn("版本字符串解析失败，继续注册", "name", name, "unique_key", key, "engine_version", Version, "required_min", minEngine, "error", fmt.Sprintf("%v %v", err1, err2))
		} else if !constraint.Check(current) {
			strict := pm.engine.Config().GetBool("plugins.compat.strict")
			if strict {
				pm.engine.Logger().Error("插件与引擎版本不兼容，拒绝注册", "name", name, "unique_key", key, "engine_version", Version, "required_min", minEngine)
				return fmt.Errorf("engine version %s does not satisfy >= %s for plugin %s", Version, minEngine, name)
			}
			pm.engine.Logger().Warn("插件与引擎版本不兼容，继续注册", "name", name, "unique_key", key, "engine_version", Version, "required_min", minEngine)
		}
	}

	// 冲突模式：默认 alias，可配置 plugins.conflict_mode=error
	mode := strings.ToLower(pm.engine.Config().GetString("plugins.conflict_mode"))
	if mode == "" {
		mode = "alias"
	}

	// 读取别名覆盖配置：plugins.aliases.<key>
	alias := pm.normalizeAlias(pm.engine.Config().GetString("plugins.aliases." + key))

	// 检查名称冲突
	conflictKeys := pm.nameIndex[name]
	hasConflict := len(conflictKeys) > 0

	if alias == "" && hasConflict {
		if mode == "error" {
			pm.engine.Logger().Error("插件名称冲突，拒绝注册", "name", name, "unique_key", key, "conflict_with", conflictKeys, "hint", "设置 plugins.conflict_mode=alias 或配置 plugins.aliases.<key> 指定别名")
			return ErrDuplicatePlugin(name)
		}
		// alias 模式：生成稳定别名并 WARN
		alias = pm.generateAlias(name, key)
		pm.engine.Logger().Warn("插件名称冲突，使用别名", "name", name, "assigned_alias", alias, "unique_key", key, "conflict_with", conflictKeys, "hint", "设置 plugins.conflict_mode=error 可阻止注册；或通过 plugins.aliases.<key> 覆盖别名")
	}

	// 别名唯一性保证
	if alias != "" {
		if _, exists := pm.aliasIndex[alias]; exists {
			newAlias := pm.makeAliasUnique(alias)
			pm.engine.Logger().Warn("插件别名冲突，调整别名", "original_alias", alias, "new_alias", newAlias, "unique_key", key)
			alias = newAlias
		}
	}

	// 初始化插件
	if err := p.Init(pm.engine); err != nil {
		pm.engine.Logger().Error("插件初始化失败", "plugin", name, "unique_key", key, "error", err)
		return err
	}

	// 记录索引与元数据
	pm.plugins = append(pm.plugins, p)
	pm.index[key] = p
	pm.nameIndex[name] = append(pm.nameIndex[name], key)
	if alias != "" {
		pm.alias[key] = alias
		pm.aliasIndex[alias] = key
	}

	// 成功日志：展示名优先 alias，其次 name
	display := alias
	if display == "" {
		display = name
	}
	pm.engine.Logger().Info("插件注册成功", "display", display, "name", name, "unique_key", key, "version", p.Version())

	return nil
}

// List 返回已注册插件的快照
func (pm *PluginManager) List() []Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	cp := make([]Plugin, len(pm.plugins))
	copy(cp, pm.plugins)
	return cp
}

// OnBeforeMount 触发所有实现 BeforeMountHook 的插件
func (pm *PluginManager) OnBeforeMount() {
	pm.mu.RLock()
	plugins := append([]Plugin(nil), pm.plugins...)
	pm.mu.RUnlock()
	mode := strings.ToLower(pm.engine.Config().GetString("plugins.hook_failure_mode"))
	if mode == "" {
		mode = "warn"
	}
	for _, p := range plugins {
		if hook, ok := p.(BeforeMountHook); ok {
			t := reflect.TypeOf(p)
			key := t.PkgPath() + "." + t.Name()
			display := pm.ResolveDisplayName(p)
			start := time.Now()
			func() {
				defer func() {
					if r := recover(); r != nil {
						pm.engine.Logger().Error("插件 BeforeMount 发生 panic", "display", display, "unique_key", key, "panic", r)
						if mode == "error" {
							panic(fmt.Errorf("plugin panic in BeforeMount: %v", r))
						}
					}
				}()
				if err := hook.OnBeforeMount(pm.engine); err != nil {
					if mode == "error" {
						pm.engine.Logger().Error("插件 BeforeMount 执行失败", "display", display, "unique_key", key, "error", err)
						panic(fmt.Errorf("plugin BeforeMount failed: %v", err))
					} else {
						pm.engine.Logger().Warn("插件 BeforeMount 执行失败", "display", display, "unique_key", key, "error", err)
					}
				}
			}()
			pm.engine.Logger().Info("插件钩子执行完成", "phase", "before_mount", "display", display, "unique_key", key, "duration", time.Since(start))
		}
	}
}

// OnAfterMount 触发所有实现 AfterMountHook 的插件
func (pm *PluginManager) OnAfterMount() {
	pm.mu.RLock()
	plugins := append([]Plugin(nil), pm.plugins...)
	pm.mu.RUnlock()
	mode := strings.ToLower(pm.engine.Config().GetString("plugins.hook_failure_mode"))
	if mode == "" {
		mode = "warn"
	}
	for _, p := range plugins {
		if hook, ok := p.(AfterMountHook); ok {
			t := reflect.TypeOf(p)
			key := t.PkgPath() + "." + t.Name()
			display := pm.ResolveDisplayName(p)
			start := time.Now()
			func() {
				defer func() {
					if r := recover(); r != nil {
						pm.engine.Logger().Error("插件 AfterMount 发生 panic", "display", display, "unique_key", key, "panic", r)
						if mode == "error" {
							panic(fmt.Errorf("plugin panic in AfterMount: %v", r))
						}
					}
				}()
				if err := hook.OnAfterMount(pm.engine); err != nil {
					if mode == "error" {
						pm.engine.Logger().Error("插件 AfterMount 执行失败", "display", display, "unique_key", key, "error", err)
						panic(fmt.Errorf("plugin AfterMount failed: %v", err))
					} else {
						pm.engine.Logger().Warn("插件 AfterMount 执行失败", "display", display, "unique_key", key, "error", err)
					}
				}
			}()
			pm.engine.Logger().Info("插件钩子执行完成", "phase", "after_mount", "display", display, "unique_key", key, "duration", time.Since(start))
		}
	}
}

// OnBeforeServerStart 触发所有实现 BeforeServerStartHook 的插件
func (pm *PluginManager) OnBeforeServerStart() {
	pm.mu.RLock()
	plugins := append([]Plugin(nil), pm.plugins...)
	pm.mu.RUnlock()
	mode := strings.ToLower(pm.engine.Config().GetString("plugins.hook_failure_mode"))
	if mode == "" {
		mode = "warn"
	}
	for _, p := range plugins {
		if hook, ok := p.(BeforeServerStartHook); ok {
			t := reflect.TypeOf(p)
			key := t.PkgPath() + "." + t.Name()
			display := pm.ResolveDisplayName(p)
			start := time.Now()
			func() {
				defer func() {
					if r := recover(); r != nil {
						pm.engine.Logger().Error("插件 BeforeServerStart 发生 panic", "display", display, "unique_key", key, "panic", r)
						if mode == "error" {
							panic(fmt.Errorf("plugin panic in BeforeServerStart: %v", r))
						}
					}
				}()
				if err := hook.OnBeforeServerStart(pm.engine); err != nil {
					if mode == "error" {
						pm.engine.Logger().Error("插件 BeforeServerStart 执行失败", "display", display, "unique_key", key, "error", err)
						panic(fmt.Errorf("plugin BeforeServerStart failed: %v", err))
					} else {
						pm.engine.Logger().Warn("插件 BeforeServerStart 执行失败", "display", display, "unique_key", key, "error", err)
					}
				}
			}()
			pm.engine.Logger().Info("插件钩子执行完成", "phase", "before_server_start", "display", display, "unique_key", key, "duration", time.Since(start))
		}
	}
}

// OnShutdown 触发所有实现 ShutdownHook 的插件
func (pm *PluginManager) OnShutdown() {
	pm.mu.RLock()
	plugins := append([]Plugin(nil), pm.plugins...)
	pm.mu.RUnlock()
	mode := strings.ToLower(pm.engine.Config().GetString("plugins.hook_failure_mode"))
	if mode == "" {
		mode = "warn"
	}
	for _, p := range plugins {
		if hook, ok := p.(ShutdownHook); ok {
			t := reflect.TypeOf(p)
			key := t.PkgPath() + "." + t.Name()
			display := pm.ResolveDisplayName(p)
			start := time.Now()
			func() {
				defer func() {
					if r := recover(); r != nil {
						pm.engine.Logger().Error("插件 Shutdown 发生 panic", "display", display, "unique_key", key, "panic", r)
						// 关闭阶段不阻断
					}
				}()
				if err := hook.OnShutdown(pm.engine); err != nil {
					// 关闭阶段不阻断
					pm.engine.Logger().Error("插件 Shutdown 执行失败", "display", display, "unique_key", key, "error", err)
				}
			}()
			pm.engine.Logger().Info("插件钩子执行完成", "phase", "shutdown", "display", display, "unique_key", key, "duration", time.Since(start))
		}
	}
}

// ErrDuplicatePlugin 构造重复插件错误
func ErrDuplicatePlugin(name string) error {
	return errors.New("duplicate plugin: " + name)
}

// ResolveDisplayNameByKey 返回插件的对外展示名（优先别名，其次作者名）
func (pm *PluginManager) ResolveDisplayNameByKey(key string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if a, ok := pm.alias[key]; ok && a != "" {
		return a
	}
	if p, ok := pm.index[key]; ok {
		return p.Name()
	}
	return ""
}

// ResolveDisplayName 返回插件的展示名（通过实例计算唯一键）
func (pm *PluginManager) ResolveDisplayName(p Plugin) string {
	t := reflect.TypeOf(p)
	key := t.PkgPath() + "." + t.Name()
	return pm.ResolveDisplayNameByKey(key)
}

// LookupByKey 按唯一键查找插件
func (pm *PluginManager) LookupByKey(key string) (Plugin, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.index[key]
	return p, ok
}

// LookupByAliasOrName 先按别名，再按名称查找插件
// 若名称对应多个插件，则返回 false（避免歧义）
func (pm *PluginManager) LookupByAliasOrName(s string) (Plugin, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if key, ok := pm.aliasIndex[s]; ok {
		return pm.index[key], true
	}
	keys := pm.nameIndex[s]
	if len(keys) == 1 {
		return pm.index[keys[0]], true
	}
	return nil, false
}

// generateAlias 基于名称与唯一键的来源生成稳定别名
func (pm *PluginManager) generateAlias(name, key string) string {
	src := shortSourceFromKey(key)
	base := fmt.Sprintf("%s@%s", name, src)
	if _, exists := pm.aliasIndex[base]; !exists {
		return base
	}
	return pm.makeAliasUnique(base)
}

// makeAliasUnique 为别名追加递增后缀保证唯一性
func (pm *PluginManager) makeAliasUnique(alias string) string {
	suffix := 2
	for {
		candidate := fmt.Sprintf("%s-%d", alias, suffix)
		if _, exists := pm.aliasIndex[candidate]; !exists {
			return candidate
		}
		suffix++
	}
}

// normalizeAlias 别名规范化（当前仅去除空白）
func (pm *PluginManager) normalizeAlias(alias string) string {
	alias = strings.TrimSpace(alias)
	return alias
}

// shortSourceFromKey 从唯一键中提取短来源标识（取包路径最后一级）
func shortSourceFromKey(key string) string {
	if idx := strings.LastIndex(key, "."); idx > 0 {
		pkg := key[:idx]
		if index := strings.LastIndex(pkg, "/"); index >= 0 {
			return pkg[index+1:]
		}
		if index := strings.LastIndex(pkg, "."); index >= 0 {
			return pkg[index+1:]
		}
		return pkg
	}
	return key
}
