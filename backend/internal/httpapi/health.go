// Package httpapi 负责定义 HTTP 路由以及各个接口的处理函数。
package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// live 是“存活探针”接口的处理函数。
//
// 存活探针只回答一个问题：当前 HTTP 进程是否还能够接收并处理请求。
// 它不访问数据库、缓存等外部服务，因此即使那些服务暂时不可用，这里仍然可以返回成功。
// 监控系统或容器平台可以据此判断是否需要重启当前进程。
func live(c *gin.Context) {
	// c.JSON 会同时设置 200 状态码、application/json 响应头并写出 JSON 响应体。
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
