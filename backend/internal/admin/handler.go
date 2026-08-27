package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/psking307/Muggle/backend/internal/httpapi/middleware"
	"github.com/psking307/Muggle/backend/internal/httpapi/response"
	"go.uber.org/zap"
)

// CookieConfig 描述 Handler 写 Refresh Cookie 时使用的固定参数。
// 由 bootstrap 从强类型配置（config.AuthConfig）转换而来，
// Handler 自身不直接读取配置文件或环境变量。
type CookieConfig struct {
	// Name 是 Cookie 名称，例如 muggle_refresh。
	Name string
	// Secure 为 true 时 Cookie 只在 HTTPS 下发送（生产环境必须开启）。
	Secure bool
	// SameSite 控制跨站请求是否携带 Cookie，缓解 CSRF。
	SameSite http.SameSite
	// MaxAge 是 Cookie 的有效秒数，与 Refresh Session 的 TTL 一致。
	MaxAge int
	// Path 限制 Cookie 只随 /api/v1/admin 下的请求发送，减少不必要的暴露面。
	Path string
}

// Handler 是管理员认证接口的 HTTP 处理层。
// 它只负责“读 HTTP、调 Service、写 HTTP”，不直接接触数据库或密码哈希。
type Handler struct {
	service      *Service
	cookie       CookieConfig
	publicOrigin string
	log          *zap.Logger
}

// NewHandler 组装管理员认证 Handler。
func NewHandler(
	service *Service,
	cookie CookieConfig,
	publicOrigin string,
	log *zap.Logger,
) *Handler {
	return &Handler{
		service:      service,
		cookie:       cookie,
		publicOrigin: publicOrigin,
		log:          log,
	}
}

// Login godoc
// @Summary 管理员登录
// @Description 验证用户名密码，返回 Access Token 并设置 HttpOnly Refresh Cookie。
// @Tags admin-auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "用户名和密码"
// @Success 200 {object} SessionDataResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/session [post]
func (h *Handler) Login(c *gin.Context) {
	var request LoginRequest

	// 请求体必须是合法 JSON；字段格式校验交给 ValidateCredentials。
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_json", "请求体不是合法的 JSON")
		return
	}
	if err := ValidateCredentials(request.Username, request.Password); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_credentials_format", err.Error())
		return
	}

	result, err := h.service.Login(
		c.Request.Context(),
		request.Username,
		request.Password,
	)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		// 密码错误与用户名不存在统一文案，不泄露账号是否存在。
		// 日志只记录“登录失败”，绝不记录密码内容。
		response.Error(c, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	case errors.Is(err, ErrAdminDisabled):
		response.Error(c, http.StatusUnauthorized, "admin_disabled", "该管理员账号已被禁用")
		return
	case err != nil:
		h.log.Error("failed to login",
			zap.String("username", request.Username),
			zap.Error(err),
		)
		response.Error(c, http.StatusInternalServerError, "internal_error", "服务器暂时无法完成登录")
		return
	}

	h.setRefreshCookie(c, result.RefreshToken)
	c.JSON(http.StatusOK, SessionDataResponse{
		Data: SessionResponse{
			AccessToken: result.AccessToken,
			Admin:       result.Admin,
		},
	})
}

// Refresh godoc
// @Summary 轮转 Refresh Session
// @Description 用 HttpOnly Cookie 中的 Refresh Token 换取新的 Access Token 和 Refresh Cookie；旧 Token 立即失效。
// @Tags admin-auth
// @Produce json
// @Success 200 {object} SessionDataResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/session/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	// 设计文档 9.4：Refresh 必须校验可信 Origin，缓解 CSRF。
	if !h.isTrustedOrigin(c) {
		response.Error(c, http.StatusUnauthorized, "untrusted_origin", "请求来源不受信任")
		return
	}

	rawToken, err := c.Cookie(h.cookie.Name)
	if err != nil || rawToken == "" {
		response.Error(c, http.StatusUnauthorized, "missing_refresh_cookie", "缺少 Refresh Cookie")
		return
	}

	result, err := h.service.Refresh(c.Request.Context(), rawToken)
	switch {
	case errors.Is(err, ErrInvalidSession):
		// 旧 Token 重放、已撤销、已过期或伪造，统一返回 401。
		response.Error(c, http.StatusUnauthorized, "invalid_session", "Refresh 会话无效，请重新登录")
		return
	case errors.Is(err, ErrAdminDisabled):
		response.Error(c, http.StatusUnauthorized, "admin_disabled", "该管理员账号已被禁用")
		return
	case err != nil:
		h.log.Error("failed to refresh session", zap.Error(err))
		response.Error(c, http.StatusInternalServerError, "internal_error", "服务器暂时无法刷新会话")
		return
	}

	// 事务已经提交，此时写入新 Cookie 才是安全的：
	// 旧 Token 已撤销，客户端接下来只能用新 Token。
	h.setRefreshCookie(c, result.RefreshToken)
	c.JSON(http.StatusOK, SessionDataResponse{
		Data: SessionResponse{
			AccessToken: result.AccessToken,
			Admin:       result.Admin,
		},
	})
}

// Logout godoc
// @Summary 退出登录
// @Description 撤销当前 Refresh Session 并清除 Refresh Cookie。
// @Tags admin-auth
// @Success 204 "No Content"
// @Failure 401 {object} response.ErrorResponse
// @Router /admin/session [delete]
func (h *Handler) Logout(c *gin.Context) {
	// 与 Refresh 相同：退出也校验可信 Origin，防止被跨站诱导退出。
	if !h.isTrustedOrigin(c) {
		response.Error(c, http.StatusUnauthorized, "untrusted_origin", "请求来源不受信任")
		return
	}

	rawToken, err := c.Cookie(h.cookie.Name)
	if err == nil && rawToken != "" {
		// 撤销失败不阻塞退出：Cookie 无论如何都会被清除，
		// 这里只记录日志便于排查（例如数据库暂时不可用）。
		if revokeErr := h.service.Logout(c.Request.Context(), rawToken); revokeErr != nil {
			h.log.Warn("failed to revoke refresh session during logout", zap.Error(revokeErr))
		}
	}

	h.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

// Me godoc
// @Summary 获取当前管理员摘要
// @Tags admin-auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} MeDataResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /admin/me [get]
func (h *Handler) Me(c *gin.Context) {
	// 认证中间件已经把 admin_id 放进 Context。
	adminID, ok := middleware.AdminID(c)
	if !ok {
		// 正常情况下到达这里必然有值；保留防御性分支防止未来路由注册遗漏中间件。
		response.Error(c, http.StatusUnauthorized, "missing_admin", "缺少管理员身份")
		return
	}

	summary, err := h.service.Me(c.Request.Context(), adminID)
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrAdminDisabled):
		// Token 有效但账号已不可用：让前端退出并回到登录页。
		response.Error(c, http.StatusUnauthorized, "admin_unavailable", "管理员账号不可用，请重新登录")
		return
	case err != nil:
		h.log.Error("failed to get current admin", zap.Error(err))
		response.Error(c, http.StatusInternalServerError, "internal_error", "服务器暂时无法读取管理员信息")
		return
	}

	c.JSON(http.StatusOK, MeDataResponse{Data: *summary})
}

// isTrustedOrigin 判断请求的 Origin 请求头是否与配置的可信前端来源完全一致。
func (h *Handler) isTrustedOrigin(c *gin.Context) bool {
	return c.GetHeader("Origin") == h.publicOrigin
}

// setRefreshCookie 把明文 Refresh Token 写入 HttpOnly Cookie。
//
// HttpOnly 让浏览器脚本无法读取 Token（document.cookie 不可见）；
// SameSite 与 Path 进一步缩小 Cookie 的发送范围。
func (h *Handler) setRefreshCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.cookie.Name,
		Value:    token,
		Path:     h.cookie.Path,
		MaxAge:   h.cookie.MaxAge,
		HttpOnly: true,
		Secure:   h.cookie.Secure,
		SameSite: h.cookie.SameSite,
	})
}

// clearRefreshCookie 通过 MaxAge=-1 让浏览器立即删除 Refresh Cookie。
// 退出登录后即使数据库撤销失败，客户端也不再持有可用 Token。
func (h *Handler) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.cookie.Name,
		Value:    "",
		Path:     h.cookie.Path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookie.Secure,
		SameSite: h.cookie.SameSite,
	})
}
