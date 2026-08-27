# API 契约

本文档描述 Muggle Tiny Blog 的 HTTP API。机器可读的完整契约由 `backend/docs/` 下的
Swagger/OpenAPI 文件生成（`make swagger`），本文档是供人快速查阅的补充说明。
若两者出现不一致，以代码和生成的 OpenAPI 为准。

## 约定

- 所有业务接口使用统一前缀 `/api/v1`。
- 时间一律使用 UTC，输出 RFC 3339（例如 `2026-08-25T12:00:00Z`）。
- 成功响应统一包裹在 `data` 字段里；列表响应额外带 `meta` 分页信息。
- 错误响应统一为 `{ "error": { "code", "message", "request_id" } }`，`request_id`
  对应响应头 `X-Request-ID`，可用于在日志里定位该次请求。

### 成功响应

```json
{ "data": { "id": 12, "title": "Gin Engine 是什么" } }
```

### 列表响应

```json
{
  "data": [],
  "meta": { "page": 1, "page_size": 10, "total": 0 }
}
```

### 错误响应

```json
{
  "error": {
    "code": "post_version_conflict",
    "message": "文章已被其他操作修改，请刷新后重试",
    "request_id": "01J..."
  }
}
```

## 健康检查

| 方法 | 路径 | 成功 | 说明 |
|---|---|---:|---|
| GET | `/api/v1/health/live` | 200 | 只证明进程可响应，不访问任何依赖 |
| GET | `/api/v1/health/ready` | 200/503 | 检查 MySQL（必要）；Redis/Kafka 故障只标记 degraded，不拖垮整体就绪 |

`ready` 正常响应：

```json
{
  "status": "ok",
  "checks": { "mysql": "up", "redis": "up", "kafka": "up" }
}
```

## 公开文章

| 方法 | 路径 | 成功 | 说明 |
|---|---|---:|---|
| GET | `/api/v1/posts?page=1&page_size=10` | 200 | 已发布文章列表 |
| GET | `/api/v1/posts/:slug` | 200/404 | 已发布文章详情；成功后产生一条浏览事件 |

- `page` 必须 `>= 1`，`page_size` 必须在 `1..100`。
- 详情响应头 `X-Cache: HIT / MISS / BYPASS` 表示缓存命中/未命中/降级。
- 详情包含 `view_count`（累计浏览量），由 `LEFT JOIN post_stats` 实时返回，
  是“最终一致”的近似值。
- 草稿与不存在的 slug 统一返回 404，不泄露草稿存在性。

## 管理员认证

| 方法 | 路径 | 成功 | 说明 |
|---|---|---:|---|
| POST | `/api/v1/admin/session` | 200 | 登录，返回 Access Token 并设置 HttpOnly Refresh Cookie |
| POST | `/api/v1/admin/session/refresh` | 200 | 轮转 Refresh Session，返回新 Access Token |
| DELETE | `/api/v1/admin/session` | 204 | 撤销当前会话并清 Cookie |
| GET | `/api/v1/admin/me` | 200 | 返回当前管理员摘要 |

- 除登录和 Refresh 外，管理接口使用 `Authorization: Bearer <access-token>`。
- Refresh 与退出校验 `Origin` 必须等于 `PUBLIC_ORIGIN`。
- Access Token 短时效（默认 15 分钟）；Refresh Token 只存 SHA-256 哈希到 MySQL。

## 管理文章

| 方法 | 路径 | 成功 | 说明 |
|---|---|---:|---|
| GET | `/api/v1/admin/posts` | 200 | 查看草稿和已发布文章 |
| GET | `/api/v1/admin/posts/:id` | 200/404 | 编辑页读取完整内容 |
| POST | `/api/v1/admin/posts` | 201 | 创建草稿 |
| PUT | `/api/v1/admin/posts/:id` | 200/409 | 修改文章并校验 version |
| POST | `/api/v1/admin/posts/:id/publish` | 200/409 | 发布 |
| POST | `/api/v1/admin/posts/:id/unpublish` | 200/409 | 取消发布 |

- 新文章只能以 `draft` 创建。
- 更新、发布、取消发布都要求携带 `version`（乐观锁），不匹配返回 409。
- 已发布文章的 slug 锁定；修改会返回 `slug_immutable`。

## 错误码与状态码映射

| 业务错误 | HTTP | code |
|---|---:|---|
| 参数或格式错误 | 400 | `invalid_pagination`、`invalid_slug`、`invalid_body` 等 |
| 未认证或 Token 失效 | 401 | `missing_token`、`invalid_token`、`token_expired`、`invalid_credentials`、`invalid_session` |
| 资源不存在 | 404 | `post_not_found`、`not_found` |
| slug 或版本冲突 | 409 | `slug_taken`、`slug_immutable`、`post_version_conflict`、`invalid_status_transition` |
| 请求体过大 | 413 | `request_too_large` |
| 必要依赖不可用 | 503 | `mysql_unavailable`（仅 ready） |
| 未分类内部错误 | 500 | `internal_error` |
