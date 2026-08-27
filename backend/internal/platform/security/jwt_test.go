package security

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "unit-test-secret-that-is-long-enough"

// TestAccessTokenRoundTrip 验证“签发 -> 解析”的正常闭环。
func TestAccessTokenRoundTrip(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	signed, err := IssueAccessToken(testSecret, 15*time.Minute, 42, "alice", now)
	require.NoError(t, err)

	claims, err := ParseAccessToken(testSecret, signed, now.Add(time.Minute))

	require.NoError(t, err)
	assert.Equal(t, uint64(42), claims.AdminID)
	assert.Equal(t, "alice", claims.Username)
}

// TestAccessTokenExpired 验证过期 Token 返回专门的 ErrTokenExpired。
func TestAccessTokenExpired(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	signed, err := IssueAccessToken(testSecret, time.Minute, 42, "alice", now)
	require.NoError(t, err)

	// 用 TTL 之后的时间解析，必须被判定为过期。
	_, err = ParseAccessToken(testSecret, signed, now.Add(2*time.Minute))

	assert.ErrorIs(t, err, ErrTokenExpired)
}

// TestAccessTokenWithWrongSecret 验证用错误密钥签发的 Token 无法通过验证。
func TestAccessTokenWithWrongSecret(t *testing.T) {
	now := time.Now().UTC()

	signed, err := IssueAccessToken("another-secret-value", time.Minute, 42, "alice", now)
	require.NoError(t, err)

	_, err = ParseAccessToken(testSecret, signed, now)

	assert.ErrorIs(t, err, ErrTokenInvalid)
}

// TestAccessTokenTampered 验证被篡改的 Token（伪造 JWT）无法通过验证。
func TestAccessTokenTampered(t *testing.T) {
	now := time.Now().UTC()

	signed, err := IssueAccessToken(testSecret, time.Minute, 42, "alice", now)
	require.NoError(t, err)

	// 把负载中的一个字符改掉（注意避开签名段），模拟攻击者篡改。
	tampered := signed[:len(signed)-5] + "AAAAA"

	_, err = ParseAccessToken(testSecret, tampered, now)

	assert.ErrorIs(t, err, ErrTokenInvalid)
}

// TestAccessTokenWrongAlgorithm 验证算法混淆攻击被拒绝。
// 攻击者可能尝试用其他算法重新签发 Token 并声明 HS256；
// WithValidMethods 白名单必须挡住这类 Token。
func TestAccessTokenWrongAlgorithm(t *testing.T) {
	now := time.Now().UTC()

	// 故意用 HS384 签名，但声明算法写成 HS256，模拟算法混淆。
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, AccessClaims{
		AdminID:  42,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	})
	token.Header["alg"] = jwt.SigningMethodHS256.Alg()
	signed, err := token.SignedString([]byte(testSecret))
	require.NoError(t, err)

	_, err = ParseAccessToken(testSecret, signed, now)

	assert.ErrorIs(t, err, ErrTokenInvalid)
}

// TestAccessTokenWithoutExpiry 验证缺少过期时间的 Token 被拒绝。
// Access Token 的生命周期必须显式受控，防止“永不过期”的 Token。
func TestAccessTokenWithoutExpiry(t *testing.T) {
	now := time.Now().UTC()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, AccessClaims{
		AdminID:  42,
		Username: "alice",
	})
	signed, err := token.SignedString([]byte(testSecret))
	require.NoError(t, err)

	_, err = ParseAccessToken(testSecret, signed, now)

	assert.ErrorIs(t, err, ErrTokenInvalid)
}
