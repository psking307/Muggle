// Package admin 包含管理员账号、登录认证与 Refresh Session 的业务分层：
// Model、DTO、Repository、Service、Handler 和路由注册。
package admin

import "time"

// AdminStatus 是管理员账号状态，而不是任意字符串。
// 使用自定义类型可以减少把普通字符串误当作状态传入的机会，
// 与 post.Status 的做法保持一致。
type AdminStatus string

const (
	// AdminStatusActive 表示账号可以正常登录和刷新会话。
	AdminStatusActive AdminStatus = "active"
	// AdminStatusDisabled 表示账号已被禁用：新的登录和 Refresh 都必须被拒绝。
	AdminStatusDisabled AdminStatus = "disabled"
)

// Admin 是 admins 表在 Go 中的映射，也叫 GORM Model。
//
// PasswordHash 永远不会出现在 API 响应中：对外只使用 dto.go 中的 AdminSummary。
type Admin struct {
	ID           uint64      `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string      `gorm:"column:username"`
	PasswordHash string      `gorm:"column:password_hash"`
	Status       AdminStatus `gorm:"column:status"`
	CreatedAt    time.Time   `gorm:"column:created_at"`
	UpdatedAt    time.Time   `gorm:"column:updated_at"`
}

// TableName 明确告诉 GORM：Admin 对应 MySQL 中的 admins 表。
func (Admin) TableName() string {
	return "admins"
}

// RefreshSession 是 refresh_sessions 表在 Go 中的映射。
//
// 注意：这里只有 token_hash（SHA-256 摘要），明文的 Refresh Token
// 只存在于浏览器 Cookie 和服务端刚生成它的那一瞬间，绝不入库。
type RefreshSession struct {
	ID        uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	AdminID   uint64     `gorm:"column:admin_id"`
	TokenHash string     `gorm:"column:token_hash"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

// TableName 明确告诉 GORM：RefreshSession 对应 refresh_sessions 表。
func (RefreshSession) TableName() string {
	return "refresh_sessions"
}
