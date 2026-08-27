package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterWorkerHealth 注册 Worker 进程的内部健康检查端点（阶段六新增）。
//
// Worker 与 API 不同，它直接依赖 MySQL（写入浏览量）和 Kafka（消费事件），
// 二者缺一不可，因此 ready 会同时检查这两个依赖，任一失败都返回 503。
// 这些端点仅供 Compose/Kubernetes 的 liveness/readiness 探针调用，不承载业务。
func RegisterWorkerHealth(routes *gin.RouterGroup, mysql DatabasePinger, kafka KafkaPinger) {
	// live 与 API 的 live 共用同一个处理函数：只证明进程可响应，不访问依赖。
	routes.GET("/health/live", live)
	routes.GET("/health/ready", workerReady(mysql, kafka))
}

// workerReady 回答“Worker 当前是否具备处理浏览事件的条件”。
//
// 与 API 的 ready 语义不同：API 把 Redis/Kafka 视为可选依赖（失败只降级），
// 而 Worker 把 MySQL 与 Kafka 都视为必要依赖——任何一个不可用都无法完成
// “消费事件 → 累加浏览量”，因此直接判定未就绪。
func workerReady(mysql DatabasePinger, kafka KafkaPinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readyTimeout)
		defer cancel()

		checks := ReadyChecks{}
		ready := true

		// 检查必要依赖 MySQL。
		if mysql != nil {
			if err := mysql.PingContext(ctx); err != nil {
				checks.MySQL = "down"
				ready = false
			} else {
				checks.MySQL = "up"
			}
		}

		// 检查必要依赖 Kafka。
		if kafka != nil {
			if err := kafka.Ping(ctx); err != nil {
				checks.Kafka = "down"
				ready = false
			} else {
				checks.Kafka = "up"
			}
		}

		if !ready {
			c.JSON(http.StatusServiceUnavailable, ReadyResponse{
				Status: "unavailable",
				Checks: checks,
			})
			return
		}

		c.JSON(http.StatusOK, ReadyResponse{
			Status: "ok",
			Checks: checks,
		})
	}
}
