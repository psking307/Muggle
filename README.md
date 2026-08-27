# Muggle Tiny Blog

Muggle 是一个按阶段实现的 Tiny Blog Lab。当前仓库已经完成阶段 2：用 MySQL + GORM 打通“数据库中的已发布文章 → Gin API → React 页面”的第一条真实业务链路。

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

## 从零启动阶段 2

在仓库根目录依次运行：

```bash
make dev-infra   # 1. 启动 MySQL，等待容器变为 healthy
make migrate-up  # 2. 按版本创建数据库表
make seed        # 3. 写入三篇开发数据，可安全重复执行
make dev-api     # 4. 启动 Gin API：http://localhost:8080
```

另开一个 WSL 终端：

```bash
cd /home/hejinze/Muggle
make dev-web     # 启动网页：http://localhost:5173
```

可访问地址：

- 文章列表：`http://localhost:5173/`
- 健康页：`http://localhost:5173/health`
- Swagger UI：`http://localhost:8080/swagger/index.html`

## 常用命令

```bash
make dev-infra        # 启动阶段 2 的 MySQL
make dev-api          # 在 WSL 直接运行 Gin API
make dev-web          # 运行 Vite 前端开发服务器
make migrate-up       # 执行尚未执行的 migration
make migrate-down     # 回退一个 migration 版本
make seed             # 写入开发 seed
make swagger          # 重新生成 OpenAPI 文件
make fmt              # 格式化 Go 代码
make lint             # go vet、ESLint、TypeScript 类型检查
make test             # 后端单元/Handler测试和前端页面测试
make test-integration # Repository 的 MySQL 集成测试
make build            # 构建 Go 二进制和 React 生产文件
make compose-down     # 停止容器，但保留 MySQL volume
```

## API 契约

所有业务 API 使用前缀 `/api/v1`：

```text
GET /api/v1/health/live
GET /api/v1/health/ready
GET /api/v1/posts?page=1&page_size=10
GET /api/v1/posts/:slug
```

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

DataGrip 使用 `localhost:3306`，数据库为 `tiny_blog`，账号密码取自本机 `.env`。

## 代码分层

```text
React 页面
  → Axios /api/v1
  → Gin Router / Middleware
  → Handler：读取 HTTP 参数，映射 HTTP 响应
  → Service：执行“只有 published 可公开读取”等业务规则
  → Repository：用 GORM 查询数据库
  → MySQL：文章数据的唯一事实来源
```

主要目录：

```text
backend/docs/                       # 自动生成的 Swagger/OpenAPI
backend/internal/httpapi/           # Router、健康检查、统一响应、Middleware
backend/internal/platform/database/ # GORM/MySQL 技术适配
backend/internal/post/              # 文章业务分层
backend/migrations/                 # 版本化数据库结构
backend/seeds/                      # 显式开发数据
frontend/src/features/posts/        # 文章 API 类型和调用函数
frontend/src/pages/                 # 列表页、详情页、健康页及页面测试
deploy/compose.yaml                 # 阶段 2 MySQL 和 migration job
```

## 安全与工程约束

- `.env` 不提交；示例密码不能用于生产环境。
- SQL migration 是数据库结构的唯一管理入口。
- Handler 不直接查询 GORM，Repository 不返回 HTTP 状态码。
- 所有 Repository 查询都传入 `context.Context`。
- 草稿对公开请求统一表现为 404。
- 前端按纯文本显示 Markdown 原文，不注入不安全 HTML。
