package httpapi

import (
	"github.com/gin-gonic/gin"

	// 匿名导入会执行生成文档中的 init，把本项目的 API 描述注册给 Swagger。
	_ "github.com/psking307/Muggle/backend/docs"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// RegisterSwagger 在开发环境挂载可交互 API 文档。
func RegisterSwagger(router *gin.Engine) {
	router.GET(
		"/swagger/*any",
		ginSwagger.WrapHandler(swaggerFiles.Handler),
	)
}
