# 架构说明

本文档解释 Muggle Tiny Blog 的整体架构与关键设计决策。项目定位是用一个范围受控的
个人博客，循序渐进地练习 Go、Gin、GORM、MySQL、React、Redis、Kafka、Docker 与
Kubernetes（详见 `Muggle-design.md`）。

## 整体形态

项目是「模块化单体 + 独立异步 Worker」：

- **api**：Gin HTTP API，处理公开文章读取与管理端读写。
- **worker**：Kafka 消费者，异步更新文章浏览量，与 API 共用配置、领域模型和 GORM。
- **admin**：一次性离线命令，创建初始管理员。
- **web**：React 单页应用。

不拆微服务：Worker 独立只是因为它需要独立的生命周期与扩缩容方式。

## 开发环境拓扑

```text
Browser
   |  (Vite 代理 /api -> :8080)
   v
Vite :5173  -->  Gin API :8080
                    |
           +--------+--------+
           |        |        |
         MySQL    Redis     Kafka
           ^                  |
           |                  v
           +-------------- Worker
```

- MySQL、Redis、Kafka 由 Docker Compose 启动（`make dev-infra`）。
- API 与 Worker 在 WSL 直接运行（`make dev-api` / `make dev-worker`），便于断点与看日志。
- React 通过 Vite 运行（`make dev-web`），`/api` 代理到 API。

## 后端分层与调用方向

```text
Router / Middleware
   ↓
Handler：绑定参数、调用 Service、映射 HTTP 响应
   ↓
Service：业务规则、授权、状态流转、事务边界、缓存失效、投递浏览事件
   ↓
Repository：GORM 查询和持久化
   ↓
MySQL
```

硬约束：

- Gin 只出现在 HTTP 层；Service 接收 `context.Context`，不接收 `*gin.Context`。
- Handler 不直接执行 GORM 查询；Repository 不返回 HTTP 状态码。
- GORM Model 不直接作为 HTTP Request/Response DTO。
- Redis、Kafka 等技术适配放在 `platform` 或对应业务模块的适配文件中。

## 数据事实来源

| 存储 | 定位 |
|---|---|
| MySQL | 管理员、Refresh Session、文章、浏览统计、已处理事件的唯一事实来源 |
| Redis | 只保存可删除、可重建的公开文章缓存 |
| Kafka | 只保存待异步处理的浏览事件，不承担文章读写主流程 |

## 异步浏览量链路（阶段六）

```text
访客打开文章详情
   → API 成功返回详情（缓存命中或回源）
   → Producer 尽力投递一条 post.viewed.v1 事件到 Kafka
        ↓
   Worker 消费事件
   → 在一个 MySQL 事务里：插入 processed_events（幂等去重）+ 累加 post_stats
   → 事务提交后，才提交 Kafka offset
```

- **幂等**：`processed_events.event_id` 主键唯一，重复投递不会重复计数。
- **至少一次 + 先落库再确认**：offset 只在数据库事务成功后提交；处理失败不提交，等重投。
- **最终一致**：浏览量是近似值，Kafka 不可用期间可能少算（有意保留的权衡）。
- **详情缓存不存计数**：浏览量实时回源 `post_stats`，避免缓存里长期显示旧值。

## 认证与会话

- Access JWT 短时效，只存前端内存（Zustand），不落 LocalStorage。
- Refresh Token 随机生成，MySQL 只存 SHA-256；每次轮转，旧 Token 立即失效。
- Refresh Cookie 为 HttpOnly + SameSite=Lax，生产环境必须 Secure。

## 安全加固（阶段七）

- 统一安全响应头（nosniff、frame deny、Referrer-Policy）。
- CORS 只放行配置的可信前端来源。
- 请求体大小限制（约 2MB）。
- 不信任任意反向代理的 `X-Forwarded-*` 头。

## 目录速览

```text
backend/cmd/{api,admin,worker}/   # 三个进程入口
backend/internal/{httpapi,admin,post,view,app,config}/
backend/internal/platform/{database,rediscache,kafka,logger,security}/
backend/migrations/               # 版本化 SQL（golang-migrate）
backend/docs/                     # 生成的 OpenAPI
frontend/src/{api,app,features,components,layouts,pages,stores}/
deploy/compose.yaml               # MySQL / Redis / Kafka / migrate
.github/workflows/                # CI 质量门禁
```
