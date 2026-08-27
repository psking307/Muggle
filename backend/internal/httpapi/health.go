// Package httpapi 负责定义 HTTP 路由以及各个接口的处理函数。
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const readyTimeout = time.Second

// DatabasePinger 只描述 readiness 真正需要的数据库能力。
// *sql.DB 自动满足该接口；测试可以提供无需 MySQL 的 fake 实现。
type DatabasePinger interface {
	PingContext(ctx context.Context) error
}

// LiveResponse 是存活探针的稳定响应结构。
// 使用明确 DTO 而不是临时 gin.H，可以让运行时代码、测试和 Swagger 共用同一份契约。
type LiveResponse struct {
	Status string `json:"status"`
}

// ReadyChecks 列出 readiness 当前检查的必要依赖。
// 阶段二只有 MySQL；后续新增 Redis、Kafka 时应先遵守设计中的降级边界，再扩展该结构。
type ReadyChecks struct {
	MySQL string `json:"mysql"`
}

// ReadyResponse 是就绪探针在成功和失败时共用的响应结构。
// HTTP 状态码表达“是否就绪”，Status 和 Checks 则帮助开发者定位具体失败依赖。
type ReadyResponse struct {
	Status string      `json:"status"`
	Checks ReadyChecks `json:"checks"`
}

// live 是“存活探针”接口的处理函数。
//
// 存活探针只回答一个问题：当前 HTTP 进程是否还能够接收并处理请求。
// 它不访问数据库、缓存等外部服务，因此即使那些服务暂时不可用，这里仍然可以返回成功。
// 监控系统或容器平台可以据此判断是否需要重启当前进程。
//
// @Summary 检查 API 进程是否存活
// @Tags health
// @Produce json
// @Success 200 {object} LiveResponse
// @Router /health/live [get]
func live(c *gin.Context) {
	// c.JSON 会同时设置 200 状态码、application/json 响应头并写出 JSON 响应体。
	c.JSON(http.StatusOK, LiveResponse{Status: "ok"})
}

// RegisterReadyRoute 注册检查 MySQL 的就绪探针。
func RegisterReadyRoute(routes *gin.RouterGroup, pinger DatabasePinger) {
	routes.GET("/health/ready", ready(pinger))
}

// ready 回答“API 当前是否具备处理真实文章请求的条件”。
// MySQL 不可用时 API 进程仍然存活，因此 live 保持 200，而 ready 返回 503。
//
// @Summary 检查 API 是否可以访问必要的 MySQL
// @Tags health
// @Produce json
// @Success 200 {object} ReadyResponse
// @Failure 503 {object} ReadyResponse
// @Router /health/ready [get]
func ready(pinger DatabasePinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readyTimeout)
		defer cancel()

		if err := pinger.PingContext(ctx); err != nil {
			c.Set("error_code", "mysql_unavailable")
			c.JSON(http.StatusServiceUnavailable, ReadyResponse{
				Status: "unavailable",
				Checks: ReadyChecks{MySQL: "down"},
			})
			return
		}

		c.JSON(http.StatusOK, ReadyResponse{
			Status: "ok",
			Checks: ReadyChecks{MySQL: "up"},
		})
	}
}
