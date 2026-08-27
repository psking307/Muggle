package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/psking307/Muggle/backend/internal/platform/security"
)

// ErrInvalidCredentials 表示用户名或密码错误。
// “用户名不存在”和“密码错误”对外返回同一个错误，避免攻击者通过
// 不同的错误文案枚举系统中存在哪些用户名。
var ErrInvalidCredentials = errors.New("invalid username or password")

// dummyPasswordHash 是用户名不存在时用于“假比对”的固定 bcrypt 哈希。
// 对不存在的用户也执行一次 bcrypt 比较，让响应时间与真实密码比对接近，
// 避免攻击者通过计时差异判断用户名是否存在。
// 该哈希对应明文无实际意义，仅用于恒定时间的假比较。
var dummyPasswordHash = "$2a$12$8cFsjYMcn.JduGegeob7eO.QRop1r0Nz5QPn7P2ccTUS1zgo35lFK"

// SessionResult 是登录或 Refresh 成功后的内部结果。
// RefreshToken 只在生成它的这一次出现，Service 只把它交给 Handler 写 Cookie，
// 不进日志、不进数据库（数据库只存它的 SHA-256 摘要）。
type SessionResult struct {
	AccessToken  string
	RefreshToken string
	Admin        AdminSummary
}

// Service 实现管理员认证的业务规则。
//
// 字段全是不可变配置；now 函数便于测试注入固定时间验证过期逻辑。
type Service struct {
	repository        Repository
	jwtSecret         string
	accessTokenTTL    time.Duration
	refreshSessionTTL time.Duration
	now               func() time.Time
}

// NewService 组装认证 Service。
func NewService(
	repository Repository,
	jwtSecret string,
	accessTokenTTL time.Duration,
	refreshSessionTTL time.Duration,
) *Service {
	return &Service{
		repository:        repository,
		jwtSecret:         jwtSecret,
		accessTokenTTL:    accessTokenTTL,
		refreshSessionTTL: refreshSessionTTL,
		now:               time.Now,
	}
}

// Login 验证用户名与密码，成功时签发 Access Token 并创建 Refresh Session。
func (s *Service) Login(
	ctx context.Context,
	username string,
	password string,
) (*SessionResult, error) {
	admin, err := s.repository.FindByUsername(ctx, username)
	if errors.Is(err, ErrNotFound) {
		// 用户名不存在：也执行一次 bcrypt 假比对（见 dummyPasswordHash 注释），
		// 然后返回与“密码错误”完全相同的错误。
		_ = security.CheckPassword(dummyPasswordHash, password)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	// 禁用账号一律拒绝登录；即使密码正确也不能通过。
	if admin.Status != AdminStatusActive {
		return nil, ErrAdminDisabled
	}

	if !security.CheckPassword(admin.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}

	return s.issueSession(ctx, admin)
}

// Refresh 验证旧 Refresh Token 并轮转出全新会话。
//
// 旧 Token 的撤销与新会话的创建在 Repository 的同一个事务里完成；
// 事务成功返回后，调用方（Handler）才可以把新 Token 写入 Cookie。
func (s *Service) Refresh(
	ctx context.Context,
	rawToken string,
) (*SessionResult, error) {
	// 先准备好新 Token 的明文和摘要，事务内只需要写入。
	newRawToken, err := security.NewRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}

	admin, err := s.repository.RotateRefreshSession(
		ctx,
		security.HashRefreshToken(rawToken),
		security.HashRefreshToken(newRawToken),
		s.now().UTC().Add(s.refreshSessionTTL),
	)
	if err != nil {
		// ErrInvalidSession（重放/过期/撤销）和 ErrAdminDisabled 原样上抛，
		// Handler 会把它们映射为 401。
		return nil, err
	}

	accessToken, err := security.IssueAccessToken(
		s.jwtSecret,
		s.accessTokenTTL,
		admin.ID,
		admin.Username,
		s.now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}

	return &SessionResult{
		AccessToken:  accessToken,
		RefreshToken: newRawToken,
		Admin:        toSummary(admin),
	}, nil
}

// Logout 撤销当前 Refresh Session。
// 即使 Token 已经失效也返回 nil：退出登录对用户来说总是“成功”。
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if err := s.repository.RevokeRefreshSession(
		ctx,
		security.HashRefreshToken(rawToken),
	); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// Me 返回指定管理员的摘要，并在返回前重新检查账号状态。
// 认证中间件只证明“Token 有效”；账号是否仍可用必须以数据库为准。
func (s *Service) Me(ctx context.Context, adminID uint64) (*AdminSummary, error) {
	admin, err := s.repository.FindByID(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if admin.Status != AdminStatusActive {
		return nil, ErrAdminDisabled
	}

	summary := toSummary(admin)
	return &summary, nil
}

// CreateInitialAdmin 供 cmd/admin 离线命令创建初始管理员。
//
// 设计规定 v1.0 不提供网页注册管理员，因此这是唯一的建号入口：
// 它仍然走 Service 而不是直接操作 GORM，保证密码哈希等规则一致。
func (s *Service) CreateInitialAdmin(
	ctx context.Context,
	username string,
	password string,
) error {
	if err := ValidateCredentials(username, password); err != nil {
		return err
	}

	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return fmt.Errorf("create initial admin: %w", err)
	}

	err = s.repository.Create(ctx, &Admin{
		Username:     username,
		PasswordHash: passwordHash,
		Status:       AdminStatusActive,
	})
	if err != nil {
		// ErrUsernameTaken 原样上抛，CLI 会打印更友好的提示。
		return err
	}
	return nil
}

// issueSession 是登录成功后的公共收尾：签发 Access Token、生成 Refresh Token
// 并落库会话记录。登录与 Refresh 共用同一套“Token 对 + 会话”的生成逻辑。
func (s *Service) issueSession(
	ctx context.Context,
	admin *Admin,
) (*SessionResult, error) {
	rawToken, err := security.NewRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("issue session: %w", err)
	}

	now := s.now().UTC()
	accessToken, err := security.IssueAccessToken(
		s.jwtSecret,
		s.accessTokenTTL,
		admin.ID,
		admin.Username,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("issue session: %w", err)
	}

	if err := s.repository.CreateRefreshSession(ctx, &RefreshSession{
		AdminID:   admin.ID,
		TokenHash: security.HashRefreshToken(rawToken),
		ExpiresAt: now.Add(s.refreshSessionTTL),
	}); err != nil {
		return nil, fmt.Errorf("issue session: %w", err)
	}

	return &SessionResult{
		AccessToken:  accessToken,
		RefreshToken: rawToken,
		Admin:        toSummary(admin),
	}, nil
}
