# Muggle Tiny Blog

Muggle 是一个按阶段实现的 Tiny Blog Lab。当前仓库已经完成阶段 4：在认证与公开读取之上，实现了从登录、创建草稿、ByteMD 编辑、发布到公开阅读、取消发布的完整写作闭环。

## 阶段 2 已实现

- Docker Compose 启动 MySQL 8，并用 named volume 保存数据。
- 强类型 MySQL 配置、启动校验、连接池与启动 Ping。
- 版本化 SQL migration 创建 `posts` 表；不使用 GORM `AutoMigrate`。
- 显式 seed：两篇已发布文章和一篇草稿。
- Post 的 Model、DTO、Repository、Service、Handler 分层。
- 公开文章列表、详情、分页校验和稳定错误响应。
- `/health/live` 只检查进程，`/health/ready` 检查 MySQL。
- Swagger/OpenAPI 文档与开发环境 Swagger UI。
- React 文章列表、详情和分页；覆盖加载、空列表、404、非法分页、500 与网络错误。
- Go 单元/Handler/Repository 集成测试和 Vitest 页面测试。

阶段 2 不包含管理员登录、文章写入、Redis、Kafka、Zustand 和 ByteMD；这些属于后续阶段。

## 阶段 3 已实现

- `admins` 与 `refresh_sessions` 版本化 migration：状态 CHECK 约束、外键和唯一索引。
- bcrypt 密码哈希；密码与哈希不进入日志和 API 响应。
- 离线建号命令 `make admin-create`（v1.0 不提供网页注册管理员）。
- 登录签发短期 Access JWT + 随机 Refresh Token；MySQL 只保存 Refresh Token 的 SHA-256。
- Refresh Session 事务轮转：旧 Token 立即失效，重放/过期/撤销统一返回 401。
- 登录、Refresh、退出、`/admin/me` 四个接口与 Bearer JWT 认证中间件。
- Refresh Cookie 带 HttpOnly、SameSite、Path；Access Token 只存在前端内存（Zustand）。
- Axios 请求拦截器附加 Token；响应层单次并发刷新，失败统一退出。
- 登录页、管理路由守卫、刷新页面后的会话恢复。
- 生产环境配置校验：拒绝弱 JWT 密钥、示例密钥、通配来源与非 Secure Cookie。
- 覆盖错误密码、禁用账号、过期/伪造 JWT、轮转、重放、退出的自动化测试。

## 阶段 4 已实现

- 管理端文章列表、详情、创建草稿、更新、发布、取消发布接口（全部需要 Bearer Token）。
- 新文章只能以 `draft` 创建；发布写入 `published_at`，取消发布保留该时间，再次发布不重置。
- 显式字段更新（不用整对象 Save）；`WHERE id = ? AND version = ?` 实现乐观锁，旧版本保存返回 409。
- 允许的状态流转仅 `draft <-> published`；已发布文章的 slug 锁定，草稿阶段可修改。
- 前端引入 ByteMD（GFM + 代码高亮 + 中文 locale）做 Markdown 编辑与预览。
- Markdown 安全：禁用原始 HTML、链接/图片协议白名单、渲染结果经 rehype-sanitize 清洗。
- 管理后台顶部导航布局：文章管理列表、新建、编辑页面，处理 401/404/409/保存中/发布中。
- 补齐 Service / Handler / Repository 集成测试与前端页面测试。

阶段 4 不包含 Redis、Kafka 与 Worker；这些属于后续阶段。

## 环境要求

在 WSL Ubuntu 中准备 Go、Node.js、npm、Git、Make，以及开启 WSL Integration 的 Docker Desktop。项目位于：

```text
/home/hejinze/Muggle
```

> Muggle 的容器 MySQL 映射到 `127.0.0.1:3306`，与容器内部端口一致。启动前请确保本机没有其他 MySQL 占用 3306。

## 首次准备

```bash
cd /home/hejinze/Muggle
cp .env.example .env
# 把 .env 中的示例密码改成本机开发密码；.env 不会提交到 Git。
cd frontend && npm install && cd ..
```

配置优先级是：

```text
代码默认值 < backend/configs/default.yaml < 进程环境变量
```

根 Makefile 会把仓库根目录的 `.env` 注入 Go 进程和 Docker Compose。

## 从零启动阶段 4

在仓库根目录依次运行：

```bash
make dev-infra     # 1. 启动 MySQL，等待容器变为 healthy
make migrate-up    # 2. 按版本创建数据库表（posts、admins、refresh_sessions）
make seed          # 3. 写入三篇开发文章，可安全重复执行
make admin-create  # 4. 交互式创建初始管理员（密码不回显）
make dev-api       # 5. 启动 Gin API：http://localhost:8080
```

另开一个 WSL 终端：

```bash
cd /home/hejinze/Muggle
make dev-web     # 启动网页：http://localhost:5173
```

可访问地址：

- 文章列表：`http://localhost:5173/`
- 管理登录：`http://localhost:5173/admin/login`（登录后进入 `/admin/posts` 文章管理）
- 健康页：`http://localhost:5173/health`
- Swagger UI：`http://localhost:8080/swagger/index.html`

## 常用命令

```bash
make dev-infra        # 启动 MySQL 开发容器
make dev-api          # 在 WSL 直接运行 Gin API
make dev-web          # 运行 Vite 前端开发服务器
make admin-create     # 交互式创建初始管理员
make migrate-up       # 执行尚未执行的 migration
make migrate-down     # 回退一个 migration 版本
make seed             # 写入开发 seed
make swagger          # 重新生成 OpenAPI 文件
make fmt              # 格式化 Go 代码
make lint             # go vet、ESLint、TypeScript 类型检查
make test             # 后端单元/Handler测试和前端页面测试
make test-integration # post/admin Repository 的 MySQL 集成测试
make build            # 构建 Go 二进制和 React 生产文件
make compose-down     # 停止容器，但保留 MySQL volume
```

## API 契约

所有业务 API 使用前缀 `/api/v1`：

```text
GET    /api/v1/health/live
GET    /api/v1/health/ready
GET    /api/v1/posts?page=1&page_size=10
GET    /api/v1/posts/:slug

POST   /api/v1/admin/session           # 登录：返回 Access Token，设置 Refresh Cookie
POST   /api/v1/admin/session/refresh   # 轮转会话：旧 Refresh Token 立即失效
DELETE /api/v1/admin/session           # 退出：撤销会话并清除 Cookie
GET    /api/v1/admin/me                # 当前管理员摘要（需要 Bearer Token）

GET    /api/v1/admin/posts             # 管理端文章列表（草稿 + 已发布）
GET    /api/v1/admin/posts/:id         # 编辑页完整内容
POST   /api/v1/admin/posts             # 创建草稿（201）
PUT    /api/v1/admin/posts/:id         # 更新文章（校验 version 乐观锁）
POST   /api/v1/admin/posts/:id/publish    # 发布
POST   /api/v1/admin/posts/:id/unpublish  # 取消发布
```

除登录和 Refresh 外，管理 API 使用 `Authorization: Bearer <access-token>`；Refresh 与退出接口还会校验 `Origin` 请求头必须等于 `PUBLIC_ORIGIN`。

列表响应包含 `data` 和分页 `meta`。错误响应固定包含 `code`、`message` 和用于查日志的 `request_id`。

## 手动验收

API 启动后执行：

```bash
curl -i http://localhost:8080/api/v1/health/live
curl -i http://localhost:8080/api/v1/health/ready
curl -i "http://localhost:8080/api/v1/posts?page=1&page_size=10"
curl -i http://localhost:8080/api/v1/posts/hello-muggle
curl -i http://localhost:8080/api/v1/posts/secret-draft
curl -i "http://localhost:8080/api/v1/posts?page=0"
```

预期结果：

- live、ready、已发布文章返回 200。
- 列表只包含两篇 `published` 文章。
- 草稿 `secret-draft` 返回 404，不能泄露它的存在。
- `page=0` 返回带 `request_id` 的稳定 400。

数据库故障与持久化实验：

1. 执行 `docker compose --env-file .env -f deploy/compose.yaml stop mysql`。
2. live 应继续返回 200，ready 应返回 503。
3. 执行 `docker compose --env-file .env -f deploy/compose.yaml start mysql`。
4. 等待 MySQL healthy 后，文章仍然存在，无需重新 seed。

管理员认证验收（登录后拿到的 Token 与 Cookie 只在本次实验使用）：

```bash
# 登录：200，响应含 access_token，Set-Cookie 带 HttpOnly
curl -i -c /tmp/cookies.txt -X POST http://localhost:8080/api/v1/admin/session \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"你的密码"}'

# 错误密码：401 invalid_credentials（文案不区分“用户不存在”）
# /admin/me 不带 Token：401；带 Bearer Token：200
curl -i http://localhost:8080/api/v1/admin/me -H "Authorization: Bearer <access_token>"

# Refresh：200 并下发新 Cookie；用旧 Cookie 再刷一次：401 invalid_session（重放被拒）
curl -i -X POST http://localhost:8080/api/v1/admin/session/refresh \
  -H 'Origin: http://localhost:5173' -b /tmp/cookies.txt

# 退出：204 且 Cookie 被清空；退出后再 Refresh：401
curl -i -X DELETE http://localhost:8080/api/v1/admin/session \
  -H 'Origin: http://localhost:5173' -b /tmp/cookies.txt
```

管理端写作闭环验收（登录后拿到的 Token 与 Cookie 只在本次实验使用）：

```bash
TOKEN='<登录返回的 access_token>'
# 创建草稿：201，返回 status=draft、version=1
curl -i -X POST http://localhost:8080/api/v1/admin/posts \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"阶段四验收","slug":"phase4-acceptance","summary":"","content_md":"# 你好"}'

# 用错误 version 更新：409 post_version_conflict（乐观锁）
curl -i -X PUT http://localhost:8080/api/v1/admin/posts/4 \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"改","slug":"phase4-acceptance","summary":"","content_md":"x","version":99}'

# 发布：200，status=published；公开列表/详情可见
curl -i -X POST http://localhost:8080/api/v1/admin/posts/4/publish \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"version":1}'
curl -i http://localhost:8080/api/v1/posts/phase4-acceptance

# 发布后改 slug：409 slug_immutable；取消发布：200
curl -i -X POST http://localhost:8080/api/v1/admin/posts/4/unpublish \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"version":2}'
```

DataGrip 使用 `localhost:3306`，数据库为 `tiny_blog`，账号密码取自本机 `.env`。

## 代码分层

```text
React 页面 / Zustand 认证状态
  → Axios /api/v1（请求附加 Bearer Token，401 单次并发刷新）
  → Gin Router / Middleware（Request ID → Access Log → Recovery → BearerAuth）
  → Handler：读取 HTTP 参数，映射 HTTP 响应与 Cookie
  → Service：认证规则、文章状态流转、乐观锁、slug 锁定
  → Repository：用 GORM 查询数据库（Refresh 轮转在事务中完成）
  → MySQL：文章、管理员与会话数据的唯一事实来源
```

主要目录：

```text
backend/cmd/admin/                   # 离线创建管理员的命令
backend/cmd/api/                     # API 进程入口
backend/docs/                        # 自动生成的 Swagger/OpenAPI
backend/internal/httpapi/            # Router、健康检查、统一响应、Middleware
backend/internal/admin/              # 管理员认证业务分层
backend/internal/post/               # 文章业务分层
backend/internal/platform/database/  # GORM/MySQL 技术适配
backend/internal/platform/security/  # bcrypt、JWT、Refresh Token 工具
backend/migrations/                  # 版本化数据库结构
backend/seeds/                       # 显式开发数据
frontend/src/api/                    # Axios 实例与错误描述
frontend/src/stores/                 # Zustand 认证状态与拦截器
frontend/src/features/auth/          # 认证 API 与类型
frontend/src/features/posts/         # 公开/管理文章 API 与类型
frontend/src/components/             # ByteMD Markdown 编辑器/预览器
frontend/src/layouts/                # 管理后台顶部导航布局
frontend/src/pages/                  # 公开页与管理页及页面测试
deploy/compose.yaml                  # MySQL 和 migration job
```

## 安全与工程约束

- `.env` 不提交；示例密码不能用于生产环境。
- SQL migration 是数据库结构的唯一管理入口。
- Handler 不直接查询 GORM，Repository 不返回 HTTP 状态码。
- 所有 Repository 查询都传入 `context.Context`。
- 草稿对公开请求统一表现为 404。
- Markdown 原文存于 MySQL；前端用 ByteMD 渲染，禁用原始 HTML、协议白名单 + 安全清洗，脚本不能执行。
- 密码只保存 bcrypt 哈希；Refresh Token 只保存 SHA-256，Access Token 不进 LocalStorage。
- Refresh Cookie 必须 HttpOnly；生产环境必须 Secure，且 JWT 密钥不得使用示例值。
- 登录失败文案不区分“用户名不存在”和“密码错误”；日志禁止记录密码、Token 和 Cookie。
- Refresh 每次轮转，旧 Token 立即失效；Refresh 与退出校验可信 Origin。
