package security

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewRefreshTokenIsRandomAndUnique 验证 Token 是安全随机数且不重复。
func TestNewRefreshTokenIsRandomAndUnique(t *testing.T) {
	first, err := NewRefreshToken()
	require.NoError(t, err)
	second, err := NewRefreshToken()
	require.NoError(t, err)

	// 连续两次生成的 Token 必须不同（随机源有效）。
	assert.NotEqual(t, first, second)

	// 32 字节随机数 base64url 编码后固定 43 个字符。
	// 这个断言同时检查了编码格式：只包含 base64url 字符集。
	assert.Len(t, first, 43)
	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9_-]+$`), first)
}

// TestHashRefreshTokenIsStableSHA256Hex 验证摘要格式稳定且是标准的 SHA-256 十六进制。
func TestHashRefreshTokenIsStableSHA256Hex(t *testing.T) {
	const token = "some-raw-refresh-token"

	first := HashRefreshToken(token)
	second := HashRefreshToken(token)

	// 同一 Token 两次摘要必须相同；长度固定 64 个十六进制字符。
	assert.Equal(t, first, second)
	assert.Len(t, first, 64)
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), first)

	// 不同 Token 的摘要必须不同，防止哈希碰撞造成会话串号。
	assert.NotEqual(t, first, HashRefreshToken("another-token"))
}
