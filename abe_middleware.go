package abe

import (
	"errors"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	ErrDuplicateName = errors.New("中间件名称已经存在")
	ErrNotFound      = errors.New("未找到中间件")
	ErrInvalidMerge  = errors.New("无效中间件组合并")
)

type MiddlewareManager struct {
	mu      sync.RWMutex
	globals []gin.HandlerFunc
	shared  map[string]gin.HandlerFunc
	groups  map[string][]gin.HandlerFunc
}

func newMiddlewareManager() *MiddlewareManager {
	return &MiddlewareManager{
		shared: make(map[string]gin.HandlerFunc),
		groups: make(map[string][]gin.HandlerFunc),
	}
}

func (m *MiddlewareManager) RegisterGlobal(handlers ...gin.HandlerFunc) {
	if len(handlers) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globals = append(m.globals, handlers...)
}

func (m *MiddlewareManager) getGlobals() []gin.HandlerFunc {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.globals) == 0 {
		return nil
	}
	cp := make([]gin.HandlerFunc, len(m.globals))
	copy(cp, m.globals)
	return cp
}

func (m *MiddlewareManager) RegisterShared(name string, handler gin.HandlerFunc) error {
	if name == "" || handler == nil {
		return ErrInvalidMerge
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.shared[name]; ok {
		return ErrDuplicateName
	}
	m.shared[name] = handler
	return nil
}

func (m *MiddlewareManager) UpdateShared(name string, handler gin.HandlerFunc) error {
	if name == "" || handler == nil {
		return ErrInvalidMerge
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.shared[name]; !ok {
		return ErrNotFound
	}
	m.shared[name] = handler
	return nil
}

func (m *MiddlewareManager) GetShared(name string) (gin.HandlerFunc, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.shared[name]
	return h, ok
}

func (m *MiddlewareManager) MustShared(name string) gin.HandlerFunc {
	if h, ok := m.GetShared(name); ok {
		return h
	}
	panic(ErrNotFound)
}

func (m *MiddlewareManager) RemoveShared(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.shared[name]; !ok {
		return false
	}
	delete(m.shared, name)
	return true
}

func (m *MiddlewareManager) ListShared() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.shared))
	for k := range m.shared {
		keys = append(keys, k)
	}
	return keys
}

func (m *MiddlewareManager) CreateGroup(name string, handlers ...gin.HandlerFunc) error {
	if name == "" {
		return ErrInvalidMerge
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[name]; ok {
		return ErrDuplicateName
	}
	cp := append([]gin.HandlerFunc(nil), handlers...)
	m.groups[name] = cp
	return nil
}

func (m *MiddlewareManager) CreateGroupFromShared(name string, sharedNames ...string) error {
	if name == "" {
		return ErrInvalidMerge
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[name]; ok {
		return ErrDuplicateName
	}
	handlers := make([]gin.HandlerFunc, 0, len(sharedNames))
	for _, n := range sharedNames {
		h, ok := m.shared[n]
		if !ok {
			return ErrNotFound
		}
		handlers = append(handlers, h)
	}
	m.groups[name] = handlers
	return nil
}

func (m *MiddlewareManager) CreateGroupFromGroups(name string, groupNames ...string) error {
	if name == "" {
		return ErrInvalidMerge
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[name]; ok {
		return ErrDuplicateName
	}
	visited := map[string]bool{}
	handlers, err := m.flattenGroupsLocked(groupNames, visited)
	if err != nil {
		return err
	}
	m.groups[name] = handlers
	return nil
}

func (m *MiddlewareManager) flattenGroupsLocked(groupNames []string, visited map[string]bool) ([]gin.HandlerFunc, error) {
	var result []gin.HandlerFunc
	for _, gn := range groupNames {
		if visited[gn] {
			return nil, ErrInvalidMerge
		}
		visited[gn] = true
		gs, ok := m.groups[gn]
		if !ok {
			return nil, ErrNotFound
		}
		result = append(result, gs...)
	}
	return result, nil
}

func (m *MiddlewareManager) GetGroup(name string) ([]gin.HandlerFunc, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	gs, ok := m.groups[name]
	if !ok {
		return nil, false
	}
	cp := make([]gin.HandlerFunc, len(gs))
	copy(cp, gs)
	return cp, true
}

func (m *MiddlewareManager) MustGroup(name string) []gin.HandlerFunc {
	if gs, ok := m.GetGroup(name); ok {
		return gs
	}
	panic(ErrNotFound)
}

func (m *MiddlewareManager) UpdateGroup(name string, handlers ...gin.HandlerFunc) error {
	if name == "" {
		return ErrInvalidMerge
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[name]; !ok {
		return ErrNotFound
	}
	cp := append([]gin.HandlerFunc(nil), handlers...)
	m.groups[name] = cp
	return nil
}

func (m *MiddlewareManager) AppendToGroup(name string, handlers ...gin.HandlerFunc) error {
	if len(handlers) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	gs, ok := m.groups[name]
	if !ok {
		return ErrNotFound
	}
	m.groups[name] = append(gs, handlers...)
	return nil
}

func (m *MiddlewareManager) AppendSharedToGroup(name string, sharedNames ...string) error {
	if len(sharedNames) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	gs, ok := m.groups[name]
	if !ok {
		return ErrNotFound
	}
	for _, sn := range sharedNames {
		h, ok := m.shared[sn]
		if !ok {
			return ErrNotFound
		}
		gs = append(gs, h)
	}
	m.groups[name] = gs
	return nil
}

func (m *MiddlewareManager) RemoveGroup(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[name]; !ok {
		return false
	}
	delete(m.groups, name)
	return true
}

func (m *MiddlewareManager) ListGroups() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.groups))
	for k := range m.groups {
		keys = append(keys, k)
	}
	return keys
}
