package admin

import (
	"context"
	"testing"
	"time"

	"github.com/psking307/Muggle/backend/internal/platform/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试专用常量：JWT 密钥和 TTL 不依赖真实配置。
const (
	testJWTSecret         = "service-test-secret-that-is-long-enough"
	testAccessTokenTTL    = 15 * time.Minute
	testRefreshSessionTTL = 24 * time.Hour
)

// fakeRepository 是 Repository 的内存实现，只服务于 Service 单元测试。
// 它记录每次调用收到的参数，让测试可以断言 Service 的编排是否正确。
type fakeRepository struct {
	// adminByUsername 保存可登录的账号；value 为 nil 表示“用户不存在”。
	adminByUsername map[string]*Admin
	// adminByID 支持按 ID 查询（/admin/me 测试用）。
	adminByID map[uint64]*Admin

	// findErr 让 FindByUsername/FindByID 返回指定错误（模拟数据库故障）。
	findErr error

	// createErr 让 Create 返回指定错误（例如 ErrUsernameTaken）。
	createErr error

	// createdAdmins 记录所有成功写入的管理员。
	createdAdmins []*Admin

	// sessions 记录所有成功写入的 Refresh Session（含登录与轮转）。
	sessions []*RefreshSession

	// rotateErr 让 RotateRefreshSession 返回指定错误（模拟重放/禁用等）。
	rotateErr error
	// rotateAdmin 是 RotateRefreshSession 成功时返回的管理员。
	rotateAdmin *Admin

	// revokedTokenHashes 记录退出时被撤销的 Token 摘要。
	revokedTokenHashes []string
}

// 下面一组方法把 fakeRepository 实现成完整的 Repository 接口。

func (f *fakeRepository) FindByUsername(_ context.Context, username string) (*Admin, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	admin, exists := f.adminByUsername[username]
	if !exists {
		return nil, ErrNotFound
	}
	return admin, nil
}

func (f *fakeRepository) FindByID(_ context.Context, id uint64) (*Admin, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	admin, exists := f.adminByID[id]
	if !exists {
		return nil, ErrNotFound
	}
	return admin, nil
}

func (f *fakeRepository) Create(_ context.Context, admin *Admin) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createdAdmins = append(f.createdAdmins, admin)
	return nil
}

func (f *fakeRepository) CreateRefreshSession(_ context.Context, session *RefreshSession) error {
	f.sessions = append(f.sessions, session)
	return nil
}

func (f *fakeRepository) RotateRefreshSession(
	_ context.Context,
	_ string,
	newTokenHash string,
	expiresAt time.Time,
) (*Admin, error) {
	if f.rotateErr != nil {
		return nil, f.rotateErr
	}
	f.sessions = append(f.sessions, &RefreshSession{
		TokenHash: newTokenHash,
		ExpiresAt: expiresAt,
	})
	return f.rotateAdmin, nil
}

func (f *fakeRepository) RevokeRefreshSession(_ context.Context, tokenHash string) error {
	f.revokedTokenHashes = append(f.revokedTokenHashes, tokenHash)
	return nil
}

// newFakeRepository 返回一个带有常用初始值的 fake。
func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		adminByUsername: map[string]*Admin{},
		adminByID:       map[uint64]*Admin{},
	}
}

// addActiveAdmin 生成一个带 bcrypt 哈希的 active 管理员并登记到 fake 中。
func (f *fakeRepository) addActiveAdmin(t *testing.T, id uint64, username, password string) *Admin {
	t.Helper()

	hash, err := security.HashPassword(password)
	require.NoError(t, err)

	admin := &Admin{
		ID:           id,
		Username:     username,
		PasswordHash: hash,
		Status:       AdminStatusActive,
	}
	f.adminByUsername[username] = admin
	f.adminByID[id] = admin
	return admin
}

func newTestService(repository Repository) *Service {
	return NewService(
		repository,
		testJWTSecret,
		testAccessTokenTTL,
		testRefreshSessionTTL,
	)
}

// TestLoginSuccess 验证正确凭据能签发 Access Token 并创建 Refresh Session。
func TestLoginSuccess(t *testing.T) {
	repository := newFakeRepository()
	admin := repository.addActiveAdmin(t, 7, "alice", "correct-password")
	service := newTestService(repository)

	result, err := service.Login(context.Background(), "alice", "correct-password")

	require.NoError(t, err)
	// Access Token 必须能被同一密钥解析，且携带正确身份。
	claims, err := security.ParseAccessToken(
		testJWTSecret,
		result.AccessToken,
		time.Now().UTC(),
	)
	require.NoError(t, err)
	assert.Equal(t, admin.ID, claims.AdminID)
	assert.Equal(t, "alice", claims.Username)
	assert.Equal(t, "alice", result.Admin.Username)

	// 数据库只应保存 Refresh Token 的 SHA-256，绝不能保存明文。
	require.Len(t, repository.sessions, 1)
	assert.Equal(t, security.HashRefreshToken(result.RefreshToken), repository.sessions[0].TokenHash)
	assert.NotEqual(t, result.RefreshToken, repository.sessions[0].TokenHash)
}

// TestLoginWrongPassword 验证密码错误返回 ErrInvalidCredentials，且不创建会话。
func TestLoginWrongPassword(t *testing.T) {
	repository := newFakeRepository()
	repository.addActiveAdmin(t, 7, "alice", "correct-password")
	service := newTestService(repository)

	_, err := service.Login(context.Background(), "alice", "wrong-password")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
	assert.Empty(t, repository.sessions, "密码错误时不允许创建会话")
}

// TestLoginUnknownUsername 验证不存在的用户名与密码错误返回同一个错误。
// 这是刻意的设计：不向攻击者泄露哪些用户名真实存在。
func TestLoginUnknownUsername(t *testing.T) {
	repository := newFakeRepository()
	service := newTestService(repository)

	_, err := service.Login(context.Background(), "ghost", "whatever-password")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

// TestLoginDisabledAdmin 验证禁用账号即使密码正确也拒绝登录。
func TestLoginDisabledAdmin(t *testing.T) {
	repository := newFakeRepository()
	admin := repository.addActiveAdmin(t, 7, "alice", "correct-password")
	admin.Status = AdminStatusDisabled
	service := newTestService(repository)

	_, err := service.Login(context.Background(), "alice", "correct-password")

	assert.ErrorIs(t, err, ErrAdminDisabled)
	assert.Empty(t, repository.sessions)
}

// TestRefreshSuccessRotatesSession 验证 Refresh 返回全新 Token 对并记录新会话。
func TestRefreshSuccessRotatesSession(t *testing.T) {
	repository := newFakeRepository()
	admin := repository.addActiveAdmin(t, 7, "alice", "correct-password")
	repository.rotateAdmin = admin
	service := newTestService(repository)

	oldToken := "old-raw-token"
	result, err := service.Refresh(context.Background(), oldToken)

	require.NoError(t, err)
	// 新旧 Refresh Token 必须不同（轮转的语义就是旧 Token 作废）。
	assert.NotEqual(t, oldToken, result.RefreshToken)
	// 新会话的 TokenHash 必须等于新 Token 的摘要。
	require.Len(t, repository.sessions, 1)
	assert.Equal(t, security.HashRefreshToken(result.RefreshToken), repository.sessions[0].TokenHash)
}

// TestRefreshRejectsReplayedSession 验证旧会话无效（重放/过期/撤销）时原样上抛错误。
func TestRefreshRejectsReplayedSession(t *testing.T) {
	repository := newFakeRepository()
	repository.rotateErr = ErrInvalidSession
	service := newTestService(repository)

	_, err := service.Refresh(context.Background(), "replayed-token")

	assert.ErrorIs(t, err, ErrInvalidSession)
}

// TestRefreshRejectsDisabledAdmin 验证轮转时账号已被禁用则拒绝。
func TestRefreshRejectsDisabledAdmin(t *testing.T) {
	repository := newFakeRepository()
	repository.rotateErr = ErrAdminDisabled
	service := newTestService(repository)

	_, err := service.Refresh(context.Background(), "valid-but-disabled")

	assert.ErrorIs(t, err, ErrAdminDisabled)
}

// TestLogoutRevokesHashedToken 验证退出时撤销的是摘要而不是明文。
func TestLogoutRevokesHashedToken(t *testing.T) {
	repository := newFakeRepository()
	service := newTestService(repository)
	const rawToken = "token-to-revoke"

	err := service.Logout(context.Background(), rawToken)

	require.NoError(t, err)
	require.Len(t, repository.revokedTokenHashes, 1)
	assert.Equal(t, security.HashRefreshToken(rawToken), repository.revokedTokenHashes[0])
}

// TestMeReturnsActiveAdmin 验证 /admin/me 返回摘要且不含密码哈希。
func TestMeReturnsActiveAdmin(t *testing.T) {
	repository := newFakeRepository()
	repository.addActiveAdmin(t, 7, "alice", "correct-password")
	service := newTestService(repository)

	summary, err := service.Me(context.Background(), 7)

	require.NoError(t, err)
	assert.Equal(t, "alice", summary.Username)
	assert.Equal(t, uint64(7), summary.ID)
}

// TestMeRejectsDisabledAdmin 验证 /admin/me 会重新检查数据库中的账号状态。
func TestMeRejectsDisabledAdmin(t *testing.T) {
	repository := newFakeRepository()
	admin := repository.addActiveAdmin(t, 7, "alice", "correct-password")
	admin.Status = AdminStatusDisabled
	service := newTestService(repository)

	_, err := service.Me(context.Background(), 7)

	assert.ErrorIs(t, err, ErrAdminDisabled)
}

// TestCreateInitialAdminHashesPassword 验证离线建号保存的是 bcrypt 哈希。
func TestCreateInitialAdminHashesPassword(t *testing.T) {
	repository := newFakeRepository()
	service := newTestService(repository)

	err := service.CreateInitialAdmin(context.Background(), "newadmin", "long-enough-password")

	require.NoError(t, err)
	require.Len(t, repository.createdAdmins, 1)
	created := repository.createdAdmins[0]
	assert.Equal(t, AdminStatusActive, created.Status)
	// 库中绝不能出现明文密码。
	assert.NotEqual(t, "long-enough-password", created.PasswordHash)
	assert.True(t, security.CheckPassword(created.PasswordHash, "long-enough-password"))
}

// TestCreateInitialAdminRejectsTakenUsername 验证用户名冲突被原样上抛。
func TestCreateInitialAdminRejectsTakenUsername(t *testing.T) {
	repository := newFakeRepository()
	repository.createErr = ErrUsernameTaken
	service := newTestService(repository)

	err := service.CreateInitialAdmin(context.Background(), "alice", "long-enough-password")

	assert.ErrorIs(t, err, ErrUsernameTaken)
}

// TestCreateInitialAdminRejectsBadFormat 验证离线建号与 HTTP 共用格式校验。
func TestCreateInitialAdminRejectsBadFormat(t *testing.T) {
	repository := newFakeRepository()
	service := newTestService(repository)

	err := service.CreateInitialAdmin(context.Background(), "ab", "short")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "用户名长度")
	assert.Empty(t, repository.createdAdmins)
}
