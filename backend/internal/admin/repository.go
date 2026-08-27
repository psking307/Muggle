package admin

import (
	"context"
	"errors"
	"time"
)

// 本文件集中定义 Service 可以理解的业务错误。
// Repository 不返回 HTTP 状态码，也不让上层依赖 GORM 自己的错误类型；
// Service 通过 errors.Is 识别这些哨兵错误并决定响应语义。

var (
	// ErrNotFound 表示按用户名或 ID 找不到管理员。
	ErrNotFound = errors.New("admin not found")

	// ErrUsernameTaken 表示创建管理员时用户名已存在（唯一索引冲突）。
	ErrUsernameTaken = errors.New("username is already taken")

	// ErrAdminDisabled 表示账号已被禁用，登录和 Refresh 都必须拒绝。
	// 单独区分它（而不是并入“凭据错误”），方便前端提示和管理员排查。
	ErrAdminDisabled = errors.New("admin is disabled")

	// ErrInvalidSession 表示 Refresh Session 不存在、已撤销或已过期。
	// 旧 Refresh Token 被重放时也会得到这个错误。
	ErrInvalidSession = errors.New("refresh session is not valid")
)

// Repository 描述 Service 可以向管理员数据层提出的请求。
// 测试时可以用内存 fake 实现替代真实 MySQL，验证业务编排逻辑。
type Repository interface {
	// FindByUsername 按用户名查找管理员（无论账号状态）。
	// 找不到时返回 ErrNotFound。
	FindByUsername(ctx context.Context, username string) (*Admin, error)

	// FindByID 按主键查找管理员（无论账号状态）。
	// /admin/me 等接口用它重新检查账号最新状态，而不是只相信 Token。
	FindByID(ctx context.Context, id uint64) (*Admin, error)

	// Create 写入一个新管理员。用户名冲突时返回 ErrUsernameTaken。
	Create(ctx context.Context, admin *Admin) error

	// CreateRefreshSession 写入一条新的 Refresh Session（登录时使用）。
	CreateRefreshSession(ctx context.Context, session *RefreshSession) error

	// RotateRefreshSession 在一个数据库事务中完成 Refresh 轮转：
	//   1. 读取旧会话并检查未撤销、未过期；
	//   2. 重新读取管理员并检查账号仍是 active；
	//   3. 用条件更新撤销旧会话（WHERE revoked_at IS NULL），
	//      更新行数不为 1 说明旧 Token 已被并发请求抢先轮转，按重放拒绝；
	//   4. 用 newTokenHash / expiresAt 创建新会话。
	// 返回轮转后仍然有效的管理员，供 Service 签发新的 Access Token。
	RotateRefreshSession(
		ctx context.Context,
		oldTokenHash string,
		newTokenHash string,
		expiresAt time.Time,
	) (*Admin, error)

	// RevokeRefreshSession 撤销一条 Refresh Session（退出登录时使用）。
	// 设计为幂等：会话不存在或已经撤销时仍然返回 nil，退出永远可以成功。
	RevokeRefreshSession(ctx context.Context, tokenHash string) error
}
