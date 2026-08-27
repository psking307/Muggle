package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// refreshTokenBytes 是 Refresh Token 的随机字节数。
// 设计文档要求 32 字节以上安全随机数；32 字节即 256 位熵，
// 暴力枚举在可预见的时间内不可行。
const refreshTokenBytes = 32

// NewRefreshToken 生成一个新的 Refresh Token。
//
// 返回值是 base64url 编码的随机字符串，只会通过 HttpOnly Cookie 发给浏览器；
// 数据库只保存它的 SHA-256 摘要（见 HashRefreshToken），即使数据库泄露，
// 攻击者也无法还原出可用于登录的明文 Token。
func NewRefreshToken() (string, error) {
	randomBytes := make([]byte, refreshTokenBytes)
	// rand.Read 使用操作系统提供的密码学安全随机源；
	// 这里失败极罕见，但一旦失败必须让调用方感知，绝不能退回可预测的随机数。
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	// base64.RawURLEncoding 不会产生 "+"、"/" 和末尾 "="，
	// 直接放进 Cookie 或 URL 都不需要再做转义。
	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

// HashRefreshToken 计算 Refresh Token 的 SHA-256 摘要。
//
// 摘要固定为 64 个十六进制字符，与 refresh_sessions.token_hash 的 CHAR(64) 对应。
// SHA-256 是快速哈希，但这不会带来风险：Refresh Token 本身是 256 位随机数，
// 攻击者没有足够的熵可枚举，因此不需要 bcrypt 那样故意变慢。
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
