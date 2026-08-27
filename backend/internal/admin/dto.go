package admin

import (
	"fmt"
	"regexp"
	"strings"
)

// 管理员凭据的格式规则。
// 规则同时被 HTTP 登录接口和 cmd/admin 离线命令使用，
// 保证两条创建/登录路径的校验完全一致。
const (
	usernameMinLength = 3
	usernameMaxLength = 64
	passwordMinLength = 8
	// bcrypt 只能处理 72 字节以内的输入，超过部分会被静默截断，
	// 因此把上限定在这里而不是让用户误以为超长密码都参与了哈希。
	passwordMaxLength = 72
)

// usernamePattern 限制用户名只包含字母、数字、下划线和连字符，
// 避免登录名中出现空白、控制字符等难以排查的值。
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidateCredentials 校验用户名与密码的格式，返回面向使用者的中文错误。
// 这里只检查“格式”，不检查“是否正确”；密码比对属于 Service 的职责。
func ValidateCredentials(username, password string) error {
	if len(username) < usernameMinLength || len(username) > usernameMaxLength {
		return fmt.Errorf(
			"用户名长度必须在 %d 到 %d 个字符之间",
			usernameMinLength, usernameMaxLength,
		)
	}
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("用户名只能包含字母、数字、下划线和连字符")
	}
	if len(password) < passwordMinLength || len(password) > passwordMaxLength {
		return fmt.Errorf(
			"密码长度必须在 %d 到 %d 个字符之间",
			passwordMinLength, passwordMaxLength,
		)
	}
	return nil
}

// LoginRequest 是 POST /admin/session 的请求体。
// 字段不写 validate 标签：格式校验统一由 ValidateCredentials 完成，
// 这样 HTTP 接口和 CLI 命令不会维护两套不一致的规则。
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AdminSummary 是允许返回给前端的“管理员摘要”。
// 绝不含 password_hash；新增敏感字段时也不能直接透传 Model。
type AdminSummary struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
}

// SessionResponse 是登录和 Refresh 共用的成功响应内容。
type SessionResponse struct {
	// AccessToken 只进入前端内存（Zustand），不写入 LocalStorage。
	AccessToken string       `json:"access_token"`
	Admin       AdminSummary `json:"admin"`
}

// SessionDataResponse 用统一 data 外壳包装会话响应。
type SessionDataResponse struct {
	Data SessionResponse `json:"data"`
}

// MeDataResponse 是 GET /admin/me 的响应。
type MeDataResponse struct {
	Data AdminSummary `json:"data"`
}

// toSummary 把数据库 Model 转换成对外安全的管理员摘要。
func toSummary(model *Admin) AdminSummary {
	return AdminSummary{
		ID:       model.ID,
		Username: strings.Clone(model.Username),
	}
}
