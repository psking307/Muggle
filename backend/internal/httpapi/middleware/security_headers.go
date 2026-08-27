package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders 返回为每个响应统一添加基础安全头的中间件（阶段七）。
//
// 这些响应头属于“纵深防御”：即使应用代码存在疏漏，也能降低浏览器端被利用的风险：
//   * X-Content-Type-Options: nosniff —— 禁止浏览器根据内容猜测 MIME 类型，
//     防止把服务端返回的内容误当成可执行脚本；
//   * X-Frame-Options: DENY —— 禁止页面被嵌入 iframe，降低点击劫持风险；
//   * Referrer-Policy: strict-origin-when-cross-origin —— 只在同源时发送完整
//     Referer，跨域时仅发送来源，避免泄露完整路径中的敏感信息。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		c.Next()
	}
}
