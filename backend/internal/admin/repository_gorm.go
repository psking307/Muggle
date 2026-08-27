package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// gormRepository 是 Repository 接口基于 GORM + MySQL 的实现。
// 字段只保存 *gorm.DB，所有数据库操作都通过传入的 Context 控制生命周期。
type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository 创建以 GORM 和 MySQL 为底层实现的管理员 Repository。
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// FindByUsername 按用户名查询管理员。
func (r *gormRepository) FindByUsername(
	ctx context.Context,
	username string,
) (*Admin, error) {
	var result Admin

	// Take 在结果超过一行或没有结果时都会报错；
	// username 有唯一索引，因此这里要么恰好一行，要么 ErrRecordNotFound。
	err := r.db.
		WithContext(ctx).
		Where("username = ?", username).
		Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find admin by username: %w", err)
	}

	return &result, nil
}

// FindByID 按主键查询管理员。
func (r *gormRepository) FindByID(ctx context.Context, id uint64) (*Admin, error) {
	var result Admin

	err := r.db.WithContext(ctx).Take(&result, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find admin by id: %w", err)
	}

	return &result, nil
}

// Create 写入新管理员，并把自增主键写回 admin.ID 供调用方使用。
func (r *gormRepository) Create(ctx context.Context, admin *Admin) error {
	err := r.db.WithContext(ctx).Create(admin).Error
	if isDuplicateEntryError(err) {
		// MySQL 唯一索引冲突（错误码 1062）翻译成业务错误。
		return ErrUsernameTaken
	}
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	return nil
}

// CreateRefreshSession 写入一条新会话（登录成功后调用）。
func (r *gormRepository) CreateRefreshSession(
	ctx context.Context,
	session *RefreshSession,
) error {
	err := r.db.WithContext(ctx).Create(session).Error
	if err != nil {
		return fmt.Errorf("create refresh session: %w", err)
	}
	return nil
}

// RotateRefreshSession 在事务中完成 Refresh 会话轮转，实现细节见接口注释。
//
// 并发安全的关键在第 3 步：条件 UPDATE 由数据库原子执行，
// 两个同时拿着同一个旧 Token 的请求中只有一个能更新成功（RowsAffected == 1），
// 另一个按重放拒绝，保证“同一旧 Refresh Token 只能成功轮转一次”。
func (r *gormRepository) RotateRefreshSession(
	ctx context.Context,
	oldTokenHash string,
	newTokenHash string,
	expiresAt time.Time,
) (*Admin, error) {
	var admin Admin

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()

		// 第 1 步：读取旧会话。找不到说明 Token 从未存在过（伪造或已清理）。
		var oldSession RefreshSession
		if err := tx.
			Where("token_hash = ?", oldTokenHash).
			Take(&oldSession).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidSession
			}
			return fmt.Errorf("load refresh session: %w", err)
		}

		// 第 2 步：已撤销或已过期的会话直接拒绝。
		if oldSession.RevokedAt != nil || !oldSession.ExpiresAt.After(now) {
			return ErrInvalidSession
		}

		// 第 2.5 步：重新读取管理员并检查账号状态。
		// 设计文档要求“永远重新检查管理员状态，不只相信长期会话记录”，
		// 因此即使会话本身有效，账号被禁用也必须拒绝轮转。
		if err := tx.Take(&admin, oldSession.AdminID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 账号已被删除（外键级联理论上会先删掉会话，这里只做兜底）。
				return ErrInvalidSession
			}
			return fmt.Errorf("load admin for refresh: %w", err)
		}
		if admin.Status != AdminStatusActive {
			return ErrAdminDisabled
		}

		// 第 3 步：条件撤销旧会话。
		// WHERE 中重复了“未撤销且未过期”的条件，让撤销动作自身也具备防重放能力。
		result := tx.Model(&RefreshSession{}).
			Where("id = ? AND revoked_at IS NULL AND expires_at > ?", oldSession.ID, now).
			Update("revoked_at", now)
		if result.Error != nil {
			return fmt.Errorf("revoke old refresh session: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			// 有并发请求抢先撤销了它，本次请求按旧 Token 重放处理。
			return ErrInvalidSession
		}

		// 第 4 步：创建新会话。整个事务提交后新旧状态才同时对其他连接可见。
		newSession := RefreshSession{
			AdminID:   admin.ID,
			TokenHash: newTokenHash,
			ExpiresAt: expiresAt,
		}
		if err := tx.Create(&newSession).Error; err != nil {
			return fmt.Errorf("create new refresh session: %w", err)
		}

		return nil
	})
	if err != nil {
		// 业务哨兵错误原样返回，数据库错误保留上下文。
		if errors.Is(err, ErrInvalidSession) || errors.Is(err, ErrAdminDisabled) {
			return nil, err
		}
		return nil, fmt.Errorf("rotate refresh session: %w", err)
	}

	return &admin, nil
}

// RevokeRefreshSession 撤销会话，幂等。
func (r *gormRepository) RevokeRefreshSession(
	ctx context.Context,
	tokenHash string,
) error {
	now := time.Now().UTC()

	// 条件更新：已撤销的会话不再重复写时间；找不到行也视为成功。
	err := r.db.
		WithContext(ctx).
		Model(&RefreshSession{}).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		Update("revoked_at", now).Error
	if err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	return nil
}

// isDuplicateEntryError 判断错误是否是 MySQL 的“唯一键冲突”（错误码 1062）。
// GORM 会把底层驱动错误包装起来，因此用 errors.As 而不是直接类型断言。
func isDuplicateEntryError(err error) bool {
	var mysqlErr *drivermysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
