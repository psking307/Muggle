package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 返回处理跨域请求的中间件（阶段七）。
//
// 正常情况下前端通过 Vite/Nginx 代理走同源访问，不触发跨域；但为了把安全边界
// “固化”下来，这里显式声明：只有配置的可信前端来源（PUBLIC_ORIGIN）才被允许跨域
// 访问 API，其它来源一律不返回任何 CORS 头（浏览器会因此拦截跨域响应）。
//
// 注意：这里不设置 Access-Control-Allow-Credentials。管理员认证依赖 HttpOnly 的
// Refresh Cookie（SameSite=Lax），本就只应在同源场景使用，跨域携带 Cookie 反而
// 会扩大风险面；公开读取接口无需 Cookie，因此也不需要凭证跨域。
func CORS(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// 只有当 Origin 与可信来源精确匹配时才回显 CORS 头，避免用通配符放行任意来源。
		if origin != "" && origin == allowedOrigin {
			c.Header("Access-Control-Allow-Origin", origin)
			// Vary: Origin 告诉缓存：不同 Origin 的响应可能不同，不能共用缓存。
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			// 预检结果缓存 10 分钟，减少重复的 OPTIONS 请求。
			c.Header("Access-Control-Max-Age", "600")
		}

		// 预检请求到此结束：它只是为了询问“是否允许跨域”，不需要进入业务处理。
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
