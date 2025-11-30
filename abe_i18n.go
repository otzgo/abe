package abe

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/viper"
	"golang.org/x/text/language"
)

//go:embed internal/i18n/locales/active.*.yaml
var abeLocales embed.FS

// newI18nBundle 初始化并加载 YAML 消息文件，仅在启动阶段调用
// 加载顺序：1. 框架内置翻译（embed.FS）；2. 应用层路径（文件系统）
// 后加载的同名 ID 会覆盖先加载的，实现应用层可覆盖框架默认
func newI18nBundle(v *viper.Viper, logger *slog.Logger) *i18n.Bundle {
	if v == nil {
		return nil
	}
	if !v.GetBool("i18n.enabled") {
		return nil
	}

	base := v.GetString("i18n.default_language")
	if base == "" {
		base = "en"
	}
	bundle := i18n.NewBundle(language.MustParse(base))
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	// 1. 加载框架内置翻译（embed.FS，无条件加载）
	loadEmbeddedLocales(bundle, logger)

	// 2. 加载应用层翻译（文件系统，按配置路径）
	paths := v.GetStringSlice("i18n.message_paths")
	for _, dir := range paths {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if logger != nil {
				logger.Warn("加载翻译目录失败", "dir", dir, "error", err)
			}
			continue
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			name := ent.Name()
			if !strings.HasPrefix(name, "active.") || !strings.HasSuffix(name, ".yaml") {
				continue
			}
			fp := filepath.Join(dir, name)
			if _, err := bundle.LoadMessageFile(fp); err != nil {
				if logger != nil {
					logger.Warn("加载翻译文件失败", "file", fp, "error", err)
				}
			} else {
				if logger != nil {
					logger.Info("已加载翻译文件", "file", fp)
				}
			}
		}
	}

	return bundle
}

// loadEmbeddedLocales 从 embed.FS 加载框架内置翻译文件
func loadEmbeddedLocales(bundle *i18n.Bundle, logger *slog.Logger) {
	const localesDir = "internal/i18n/locales"
	entries, err := fs.ReadDir(abeLocales, localesDir)
	if err != nil {
		if logger != nil {
			logger.Warn("读取框架内置翻译目录失败", "error", err)
		}
		return
	}

	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasPrefix(name, "active.") || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		// 注意：embed.FS 始终使用正斜杠作为路径分隔符，不能使用 filepath.Join
		filePath := localesDir + "/" + name
		if _, err := bundle.LoadMessageFileFS(abeLocales, filePath); err != nil {
			if logger != nil {
				logger.Warn("加载框架内置翻译文件失败", "file", filePath, "error", err)
			}
		} else {
			if logger != nil {
				logger.Info("已加载框架内置翻译", "file", filePath)
			}
		}
	}
}
