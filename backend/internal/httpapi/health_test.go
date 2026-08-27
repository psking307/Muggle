package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePinger struct {
	err error
}

func (p fakePinger) PingContext(_ context.Context) error {
	return p.err
}

// fakeRedisPinger 模拟 readiness 检查 Redis 的可选探针。
// 注意与 fakePinger 方法名不同：MySQL 用 PingContext，Redis 用 Ping，
// 分别对应 DatabasePinger 与 RedisPinger 两个接口。
type fakeRedisPinger struct {
	err error
}

func (p fakeRedisPinger) Ping(_ context.Context) error {
	return p.err
}

func TestReadyWhenMySQLIsAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ready", ready(fakePinger{}, nil))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"ok","checks":{"mysql":"up"}}`, response.Body.String())
}

func TestReadyWhenMySQLIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ready", ready(fakePinger{err: errors.New("mysql down")}, nil))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(
		t,
		`{"status":"unavailable","checks":{"mysql":"down"}}`,
		response.Body.String(),
	)
}

// TestReadyRedisDownDoesNotFailReadiness 验证 Redis 故障只标记 degraded，
// 不拖垮整体就绪状态（MySQL 仍正常时应返回 200）。
func TestReadyRedisDownDoesNotFailReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ready", ready(fakePinger{}, fakeRedisPinger{err: errors.New("redis down")}))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(
		t,
		`{"status":"ok","checks":{"mysql":"up","redis":"down"}}`,
		response.Body.String(),
	)
}

// TestReadyRedisUp 验证 Redis 正常时 checks.redis 为 up。
func TestReadyRedisUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ready", ready(fakePinger{}, fakeRedisPinger{}))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(
		t,
		`{"status":"ok","checks":{"mysql":"up","redis":"up"}}`,
		response.Body.String(),
	)
}
