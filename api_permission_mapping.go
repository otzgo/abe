package abe

import "time"

// APIPermissionMapping API权限映射
// 维护 RESTful 接口路径与权限资源的映射关系
type APIPermissionMapping struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	Method      string    `gorm:"size:10;not null;index:idx_method_path,priority:1" json:"method"` // HTTP方法: GET/POST/PUT/DELETE
	Path        string    `gorm:"size:255;not null;index:idx_method_path,priority:2" json:"path"`  // 路由路径: /api/members/:id
	Resource    string    `gorm:"size:50;not null" json:"resource"`                                // 权限资源: member
	Action      string    `gorm:"size:50;not null" json:"action"`                                  // 权限操作: read/write/delete
	Description string    `gorm:"size:255" json:"description"`                                     // 接口说明
	IsActive    bool      `gorm:"default:true;not null" json:"is_active"`                          // 是否启用
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (APIPermissionMapping) TableName() string {
	return "api_permission_mappings"
}

// Code 返回权限码 (resource:action)
func (m *APIPermissionMapping) Code() string {
	return m.Resource + ":" + m.Action
}
