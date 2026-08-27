package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessClaims 是 Access JWT 的负载（Payload）。
//
// 除了标准字段，还携带 admin_id 和 username，这样认证中间件
// 验证签名后就能直接从 Token 里取出当前管理员身份，不必每次都查数据库。
// 需要最新账号状态（例如是否被禁用）的接口再单独查询数据库。
type AccessClaims struct {
	AdminID  uint64 `json:"admin_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// ErrTokenExpired 表示 Token 签名有效但已经超过有效期。
// 与 ErrTokenInvalid 分开，是为了让 HTTP 层给出更准确的 401 错误码。
var ErrTokenExpired = errors.New("access token is expired")

// ErrTokenInvalid 表示 Token 无法通过签名或格式验证（伪造、篡改、算法不符等）。
var ErrTokenInvalid = errors.New("access token is invalid")

// IssueAccessToken 使用 HMAC-SHA256 签发一个短期 Access Token。
//
// now 参数显式传入当前时间，测试可以通过修改 now 验证过期逻辑，
// 而不需要真的等待或修改系统时钟。
func IssueAccessToken(
	secret string,
	ttl time.Duration,
	adminID uint64,
	username string,
	now time.Time,
) (string, error) {
	claims := AccessClaims{
		AdminID:  adminID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			// 签发时间便于审计 Token 的产生时刻。
			IssuedAt: jwt.NewNumericDate(now),
			// 过期时间由“签发时刻 + TTL”计算，与验证端保持一致。
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("issue access token: %w", err)
	}
	return signed, nil
}

// ParseAccessToken 验证 Access Token 的签名与有效期，并返回负载。
//
// 安全要点：
//   - jwt.WithValidMethods 只接受 HS256，防止攻击者把算法改成 "none"
//     或换成非对称算法后绕过签名验证（算法混淆攻击）。
//   - now 显式传入，测试可以构造“已经过期”的 Token 进行验证。
func ParseAccessToken(secret, tokenString string, now time.Time) (*AccessClaims, error) {
	claims := &AccessClaims{}

	// keyfunc 返回用于验证签名的密钥；本项目固定使用 HMAC 对称密钥。
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(_ *jwt.Token) (any, error) {
			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		// jwt 库用 ErrTokenExpired 表示“签名有效但已过期”，
		// 其余错误（签名不符、格式错误、算法不符）统一归为无效 Token。
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	if token == nil || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}
