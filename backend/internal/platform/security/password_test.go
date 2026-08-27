package security

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHashPasswordRoundTrip 验证“哈希 -> 校验”的正常闭环。
func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("muggle-secret-123")

	require.NoError(t, err)
	// 哈希必须与明文不同，且 bcrypt 哈希以 $2a$/$2b$/$2y$ 开头。
	assert.NotEqual(t, "muggle-secret-123", hash)
	assert.True(t, strings.HasPrefix(hash, "$2"))

	// 正确密码可以通过校验，错误密码不能。
	assert.True(t, CheckPassword(hash, "muggle-secret-123"))
	assert.False(t, CheckPassword(hash, "wrong-password"))
}

// TestHashPasswordProducesDifferentSalts 验证同一密码两次哈希结果不同。
// 如果结果相同，说明盐没有随机化，数据库中重复密码会被一眼看出。
func TestHashPasswordProducesDifferentSalts(t *testing.T) {
	first, err := HashPassword("same-password")
	require.NoError(t, err)
	second, err := HashPassword("same-password")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	// 两个哈希都应该能通过同一明文校验。
	assert.True(t, CheckPassword(first, "same-password"))
	assert.True(t, CheckPassword(second, "same-password"))
}

// TestHashPasswordRejectsTooLongInput 验证超过 bcrypt 72 字节上限的密码被拒绝。
// bcrypt 会静默截断超长输入，这里提前报错避免“只哈希了前 72 字节”的假象。
func TestHashPasswordRejectsTooLongInput(t *testing.T) {
	_, err := HashPassword(strings.Repeat("a", 73))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bcrypt limit")
}

// TestCheckPasswordWithGarbageHash 验证损坏的哈希不会 panic，只会返回不匹配。
func TestCheckPasswordWithGarbageHash(t *testing.T) {
	assert.False(t, CheckPassword("not-a-bcrypt-hash", "any-password"))
}
