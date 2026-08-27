//go:build integration

package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/psking307/Muggle/backend/internal/platform/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 本文件属于 Repository 集成测试：需要真实 MySQL（make test-integration）。
//
// 测试 DSN 从环境变量 MYSQL_TEST_DSN 读取（根 Makefile 的 test-integration
// 目标会从 .env 拼出）。这些测试直接写库，请只对本地开发库执行。

// openTestDB 打开集成测试专用的 GORM 连接。
// 迁移由 make migrate-up 统一执行，测试自身不改表结构。
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("MYSQL_TEST_DSN")
	require.NotEmpty(t, dsn, "集成测试需要 MYSQL_TEST_DSN 环境变量")

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	return db
}

// cleanTestData 清空本测试写入的数据，保证测试可重复执行。
// 顺序很重要：先删 refresh_sessions（外键指向 admins），再删 admins。
func cleanTestData(t *testing.T, db *gorm.DB) {
	t.Helper()

	require.NoError(t, db.Exec("DELETE FROM refresh_sessions WHERE admin_id IN (SELECT id FROM admins WHERE username LIKE ?)", "it-admin-%").Error)
	require.NoError(t, db.Exec("DELETE FROM admins WHERE username LIKE ?", "it-admin-%").Error)
}

// createTestAdmin 直接通过 Repository 创建一个 active 管理员。
func createTestAdmin(t *testing.T, repository Repository, username string) *Admin {
	t.Helper()

	hash, err := security.HashPassword("integration-password")
	require.NoError(t, err)

	admin := &Admin{
		Username:     username,
		PasswordHash: hash,
		Status:       AdminStatusActive,
	}
	require.NoError(t, repository.Create(context.Background(), admin))
	return admin
}

// TestRepositoryCreateAdminAndRejectDuplicate 验证管理员创建与用户名唯一约束。
func TestRepositoryCreateAdminAndRejectDuplicate(t *testing.T) {
	db := openTestDB(t)
	cleanTestData(t, db)
	t.Cleanup(func() { cleanTestData(t, db) })

	repository := NewGORMRepository(db)
	admin := createTestAdmin(t, repository, "it-admin-create")

	// 自增 ID 必须已经写回。
	assert.NotZero(t, admin.ID)

	// 同名再次创建必须返回 ErrUsernameTaken（依赖数据库唯一索引）。
	err := repository.Create(context.Background(), &Admin{
		Username:     "it-admin-create",
		PasswordHash: admin.PasswordHash,
		Status:       AdminStatusActive,
	})
	assert.ErrorIs(t, err, ErrUsernameTaken)
}

// TestRepositoryFindByUsernameAndID 验证两种查询路径都能找到账号。
func TestRepositoryFindByUsernameAndID(t *testing.T) {
	db := openTestDB(t)
	cleanTestData(t, db)
	t.Cleanup(func() { cleanTestData(t, db) })

	repository := NewGORMRepository(db)
	created := createTestAdmin(t, repository, "it-admin-find")

	foundByName, err := repository.FindByUsername(context.Background(), "it-admin-find")
	require.NoError(t, err)
	assert.Equal(t, created.ID, foundByName.ID)

	foundByID, err := repository.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "it-admin-find", foundByID.Username)

	// 不存在的用户名返回 ErrNotFound。
	_, err = repository.FindByUsername(context.Background(), "it-admin-missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestRepositoryRotateRefreshSessionSingleUse 验证轮转成功且旧 Token 不能重放。
// 这是设计文档 6.4 的核心验收点：同一旧 Refresh Token 只能成功轮转一次。
func TestRepositoryRotateRefreshSessionSingleUse(t *testing.T) {
	db := openTestDB(t)
	cleanTestData(t, db)
	t.Cleanup(func() { cleanTestData(t, db) })

	repository := NewGORMRepository(db)
	admin := createTestAdmin(t, repository, "it-admin-rotate")
	ctx := context.Background()

	// 模拟登录：创建一条有效会话（“旧 Token”）。
	oldRaw, err := security.NewRefreshToken()
	require.NoError(t, err)
	require.NoError(t, repository.CreateRefreshSession(ctx, &RefreshSession{
		AdminID:   admin.ID,
		TokenHash: security.HashRefreshToken(oldRaw),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}))

	// 第一次轮转：成功，并返回 active 的管理员。
	newRaw, err := security.NewRefreshToken()
	require.NoError(t, err)
	rotated, err := repository.RotateRefreshSession(
		ctx,
		security.HashRefreshToken(oldRaw),
		security.HashRefreshToken(newRaw),
		time.Now().UTC().Add(time.Hour),
	)
	require.NoError(t, err)
	assert.Equal(t, admin.ID, rotated.ID)
	assert.Equal(t, AdminStatusActive, rotated.Status)

	// 第二次用同一个旧 Token：必须被当作重放拒绝。
	_, err = repository.RotateRefreshSession(
		ctx,
		security.HashRefreshToken(oldRaw),
		security.HashRefreshToken(newRaw),
		time.Now().UTC().Add(time.Hour),
	)
	assert.ErrorIs(t, err, ErrInvalidSession)
}

// TestRepositoryRotateRefreshSessionRejectsExpired 验证过期会话不能轮转。
func TestRepositoryRotateRefreshSessionRejectsExpired(t *testing.T) {
	db := openTestDB(t)
	cleanTestData(t, db)
	t.Cleanup(func() { cleanTestData(t, db) })

	repository := NewGORMRepository(db)
	admin := createTestAdmin(t, repository, "it-admin-expired")
	ctx := context.Background()

	expiredRaw, err := security.NewRefreshToken()
	require.NoError(t, err)
	require.NoError(t, repository.CreateRefreshSession(ctx, &RefreshSession{
		AdminID:   admin.ID,
		TokenHash: security.HashRefreshToken(expiredRaw),
		// 过期时间设为过去，模拟已经过期的会话。
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
	}))

	_, err = repository.RotateRefreshSession(
		ctx,
		security.HashRefreshToken(expiredRaw),
		security.HashRefreshToken("some-new-hash"),
		time.Now().UTC().Add(time.Hour),
	)
	assert.ErrorIs(t, err, ErrInvalidSession)
}

// TestRepositoryRotateRefreshSessionRejectsDisabledAdmin 验证禁用账号不能轮转。
func TestRepositoryRotateRefreshSessionRejectsDisabledAdmin(t *testing.T) {
	db := openTestDB(t)
	cleanTestData(t, db)
	t.Cleanup(func() { cleanTestData(t, db) })

	repository := NewGORMRepository(db)
	admin := createTestAdmin(t, repository, "it-admin-disabled")
	ctx := context.Background()

	// 直接把账号改成 disabled（模拟管理操作）。
	require.NoError(t, db.Model(&Admin{}).Where("id = ?", admin.ID).
		Update("status", AdminStatusDisabled).Error)

	raw, err := security.NewRefreshToken()
	require.NoError(t, err)
	require.NoError(t, repository.CreateRefreshSession(ctx, &RefreshSession{
		AdminID:   admin.ID,
		TokenHash: security.HashRefreshToken(raw),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}))

	_, err = repository.RotateRefreshSession(
		ctx,
		security.HashRefreshToken(raw),
		security.HashRefreshToken("some-new-hash"),
		time.Now().UTC().Add(time.Hour),
	)
	assert.ErrorIs(t, err, ErrAdminDisabled)

	// 事务回滚后，旧会话必须仍然有效（不能被半途撤销）。
	var session RefreshSession
	require.NoError(t, db.Where("token_hash = ?", security.HashRefreshToken(raw)).Take(&session).Error)
	assert.Nil(t, session.RevokedAt, "禁用账号的轮转失败时不能留下已撤销的会话")
}

// TestRepositoryRevokeRefreshSessionIsIdempotent 验证退出撤销幂等。
func TestRepositoryRevokeRefreshSessionIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	cleanTestData(t, db)
	t.Cleanup(func() { cleanTestData(t, db) })

	repository := NewGORMRepository(db)
	admin := createTestAdmin(t, repository, "it-admin-revoke")
	ctx := context.Background()

	raw, err := security.NewRefreshToken()
	require.NoError(t, err)
	hash := security.HashRefreshToken(raw)
	require.NoError(t, repository.CreateRefreshSession(ctx, &RefreshSession{
		AdminID:   admin.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}))

	// 第一次撤销：成功。
	require.NoError(t, repository.RevokeRefreshSession(ctx, hash))
	// 第二次撤销（或撤销根本不存在的 Token）：同样不报错。
	require.NoError(t, repository.RevokeRefreshSession(ctx, hash))
	require.NoError(t, repository.RevokeRefreshSession(ctx, "0"+hash[1:]))

	var session RefreshSession
	require.NoError(t, db.Where("token_hash = ?", hash).Take(&session).Error)
	assert.NotNil(t, session.RevokedAt, "撤销后 revoked_at 必须写入时间")
}
