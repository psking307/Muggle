# Tiny Blog Lab 设计与分阶段实施方案

> 文档版本：2.0  
> 最近更新：2026-08-25  
> 仓库：`github.com/psking307/Muggle`  
> Go Module：`github.com/psking307/Muggle/backend`  
> 项目定位：用一个范围受控的个人博客，循序渐进练习 Go、Gin、GORM、MySQL、React、Redis、Kafka、Docker、Nginx 和 Kubernetes。

---

## 0. 文档使用说明

本方案由当前 Tiny Blog Lab 方案和旧版 HelloWorld 设计合并而来，但不是对旧项目业务的完整迁移。

合并原则：

1. 保留旧设计中成熟、边界明确的工程技术栈：Viper、Zap、validator、golang-migrate、JWT、Swagger/OpenAPI、testify、Ant Design、Tailwind CSS、Axios、Zustand 和 ByteMD。
2. 保留当前方案“小业务闭环、分阶段引入基础设施”的实施方法。
3. 不迁入旧设计中的普通用户注册、作者申请、三角色权限、复杂文章所有权和多管理员审核流程。
4. 每项依赖只在首次有真实调用者的阶段安装，不在项目开始时一次性创建全部目录、配置和抽象。
5. 每个阶段都必须以前一阶段的可运行产物为基础，完成验收后才能进入下一阶段。

本文中的目录是目标结构，不要求在阶段 0 一次性创建所有空目录。实现时只创建当前阶段实际使用的文件。

---

## 1. 关键决定

### 1.1 产品范围保持最小

v1.0 只服务两类使用者：

- 访客：阅读已发布文章和最终一致的浏览量。
- 管理员：登录、创建草稿、编辑、发布和取消发布文章。

v1.0 明确不做：

- 普通用户注册、作者申请和多角色权限。
- 评论、回复、点赞、收藏、关注和通知。
- 分类、标签、搜索和推荐。
- 图片上传、对象存储和富媒体管理。
- 多管理员协作、审计后台和动态 RBAC。
- 文章物理删除、版本历史和定时发布。
- 微服务、Service Mesh 和公网生产部署。

新想法先记录到 `docs/backlog.md`，不插入当前阶段。

### 1.2 项目形态

项目采用“模块化单体 + 独立异步 Worker”：

- `api`：Gin HTTP API。
- `worker`：Kafka 消费者，与 API 共用配置、领域模型和 GORM Repository。
- `admin`：一次性离线管理员创建命令。
- `web`：React 单页应用，生产构建后由 Nginx 提供。

v1.0 仍然只有一个业务系统，不拆微服务。Worker 独立只是因为 Kafka 消费需要独立生命周期和扩缩容方式。

### 1.3 数据事实来源

- MySQL 是管理员、Refresh Session、文章、浏览统计和已处理事件的唯一事实来源。
- Redis 只保存可以删除并重建的公开文章缓存。
- Kafka 只保存待异步处理的文章浏览事件，不承担文章读写主流程。
- Access JWT 是短期凭证，不是账号状态的事实来源。

### 1.4 基础设施引入顺序

```text
无外部依赖的 Gin/React 骨架
        ↓
MySQL：公开文章读取
        ↓
MySQL：管理员与 Refresh Session
        ↓
管理端文章写入
        ↓
Redis：公开读取缓存
        ↓
Kafka：异步浏览量
        ↓
完整 Compose 与故障实验
        ↓
本地 Kubernetes
```

这个顺序保证每项技术引入时都有明确用途和可验证故障行为。

---

## 2. 业务闭环

### 2.1 访客流程

1. 打开文章列表。
2. 只看到 `published` 状态的文章。
3. 通过稳定 Slug 打开文章详情。
4. API 返回 Markdown 内容和当前浏览量。
5. API 尽力发送浏览事件，文章响应不等待 Worker 更新计数。
6. Worker 稍后更新 MySQL，后续查询看到新的浏览量。

### 2.2 管理员流程

1. 使用离线命令创建初始管理员。
2. 管理员使用用户名和密码登录。
3. API 返回短期 Access JWT，并通过 HttpOnly Cookie 保存 Refresh Token。
4. 管理员创建草稿并使用 ByteMD 编辑 Markdown。
5. 管理员发布文章，文章立即出现在公开列表。
6. 管理员取消发布，文章立即从公开页面隐藏。
7. Access Token 过期时，前端通过 Refresh Session 轮转获得新 Token。
8. 退出时撤销 Refresh Session 并清除 Cookie。

### 2.3 文章生命周期

```text
draft ──发布──> published
  ^                |
  └────取消发布────┘
```

规则：

- 新文章只能以 `draft` 创建。
- 只有 `published` 文章能被公开读取。
- 首次发布写入 `published_at`；取消发布后保留该时间，再次发布不重置。
- Slug 在第一次发布后保持稳定；标题修改不会自动修改 Slug。
- 更新请求必须携带 `version`，使用乐观锁防止静默覆盖。
- v1.0 不提供删除接口。需要下线文章时取消发布。

---

## 3. 总体架构

### 3.1 开发环境

```text
Browser
   |
   v
Vite :5173  ── /api 代理 ──> Gin API :8080
                                      |
                         +------------+------------+
                         |            |            |
                       MySQL        Redis        Kafka
                         ^                          |
                         |                          v
                         +---------------------- Worker
```

开发阶段：

- Gin API 和 Worker 在 WSL 中直接运行，便于断点、快速编译和查看日志。
- React 在 WSL 中通过 Vite 运行，获得热更新。
- MySQL、Redis 和 Kafka 按当前阶段通过 Docker Compose 启动。
- Windows DataGrip 通过 `localhost` 连接 MySQL 映射端口。

### 3.2 完整 Compose 环境

```text
Browser :8080
   |
   v
Nginx + React
   |
   +── 静态文件和 SPA fallback
   |
   └── /api/* ──> Gin API
                     |
            +--------+--------+
            |        |        |
          MySQL    Redis    Kafka ──> Worker ──> MySQL
```

完整模式只对外暴露 Nginx。数据库端口只在显式 debug profile 中映射给本机。

### 3.3 后端调用方向

```text
Router / Middleware
        ↓
Handler：绑定参数、调用 Service、映射 HTTP 响应
        ↓
Service：业务规则、授权、状态流转和事务边界
        ↓
Repository：GORM 查询和持久化
        ↓
MySQL
```

硬约束：

- Gin 只出现在 HTTP 层。
- Service 接收标准 `context.Context`，不接收 `*gin.Context`。
- Handler 不直接执行 GORM 查询。
- Repository 不返回 HTTP 状态码。
- GORM Model 不直接作为 HTTP Request/Response DTO。
- 事务边界由 Service 或明确的应用用例决定。
- Redis、Kafka 等技术适配放在 `platform` 或对应业务模块的适配文件中。

### 3.4 API 进程启动顺序

```text
main.go
  → Viper 读取并校验当前阶段配置
  → 初始化 Zap Logger
  → 初始化当前阶段已启用的依赖
  → 创建 Repository 和 Service
  → 创建 Gin Engine
  → 按顺序注册 Middleware
  → 注册路由
  → 创建 http.Server
  → 监听端口
```

停止顺序：

1. 收到 SIGINT/SIGTERM。
2. 停止接收新请求。
3. 在配置的超时内等待在途请求完成。
4. 取消进程级 Context。
5. 关闭 Kafka Client、Redis Client 和数据库连接。
6. 同步 Zap 日志并退出。

---

## 4. 技术栈与使用边界

具体版本由 `go.mod`、`package.json`、锁文件和镜像标签固定，本文不硬编码易过期的小版本号。

### 4.1 后端

| 领域 | 技术 | 首次引入 | 使用边界 |
|---|---|---:|---|
| 语言 | Go | 阶段 0 | API、Worker 和离线命令 |
| Web | Gin | 阶段 1 | Router、Middleware、参数绑定、响应 |
| 配置 | Viper | 阶段 1 | 启动时读取文件和环境变量；Service 不直接使用 |
| 日志 | Zap | 阶段 1 | 结构化应用日志和访问日志 |
| 校验 | validator | 阶段 1/2 | 配置结构和请求 DTO 校验，不承载业务状态规则 |
| 测试 | testing + testify | 阶段 1 | 断言、Mock 辅助、Suite；不过度封装 |
| ORM | GORM | 阶段 2 | Repository 数据访问和事务 |
| 数据库 | MySQL 8 | 阶段 2 | 唯一业务事实来源 |
| 迁移 | golang-migrate | 阶段 2 | 版本化 SQL；不使用 AutoMigrate 管理结构 |
| 密码 | bcrypt | 阶段 3 | 管理员密码哈希 |
| Access Token | JWT | 阶段 3 | 15 分钟左右的短期访问凭证 |
| API 文档 | Swagger/OpenAPI | 阶段 2 起 | 公开和管理 API 契约 |
| Redis Client | go-redis/v9 | 阶段 5 | Cache-Aside，不保存唯一业务状态 |
| Kafka Client | franz-go | 阶段 6 | 浏览事件生产与消费 |

### 4.2 前端

| 领域 | 技术 | 首次引入 | 使用边界 |
|---|---|---:|---|
| 应用 | React + TypeScript + Vite | 阶段 1 | 单个公开站点和管理后台 |
| 路由 | React Router | 阶段 1 | 公开路由和管理路由 |
| UI | Ant Design | 阶段 1 | 表单、按钮、反馈、表格等交互组件 |
| 样式 | Tailwind CSS | 阶段 1 | 页面布局、间距、响应式和少量视觉样式 |
| HTTP | Axios | 阶段 1 | 统一 base URL、超时、认证和错误处理 |
| 状态 | Zustand | 阶段 3 | Access Token 和管理员摘要等少量全局状态 |
| Markdown | ByteMD | 阶段 4 | Markdown 编辑与预览 |
| 测试 | Vitest + Testing Library | 阶段 1 | 页面状态、组件和认证行为 |

Ant Design 与 Tailwind 的边界：

- Ant Design 负责交互组件及其状态。
- Tailwind 负责页面级布局和响应式。
- 不用 Tailwind 大量覆盖 Ant Design 内部类名。
- 全局主题通过 Ant Design Theme Token 统一，不同时维护多套颜色系统。

### 4.3 部署与工程

| 技术 | 首次引入 | 用途 |
|---|---:|---|
| Make | 阶段 1 | 统一开发、测试、迁移和生成命令 |
| Docker Compose | 阶段 2 | 按阶段启动 MySQL、Redis 和 Kafka |
| GitHub Actions | 阶段 7 | 测试、静态检查、构建和迁移质量门禁 |
| Docker | 阶段 8 | 构建 API、Worker 和 Web 镜像 |
| Nginx | 阶段 8 | React 静态文件、SPA fallback 和 API 代理 |
| kind + Kubernetes | 阶段 10 | 本地部署、自愈、扩缩容和配置练习 |

### 4.4 不采用或后置

- 不使用 Casbin、动态 RBAC 和多租户权限系统。
- 不同时引入另一套 ORM、日志库、配置库、UI 框架或全局状态库。
- 不使用 GORM `AutoMigrate` 代替 SQL migration。
- 不把 Access Token 放进 LocalStorage。
- 不在 v1.0 引入 Elasticsearch、MinIO、RabbitMQ、GraphQL 和微服务。
- 不为了“目录看起来完整”提前创建没有调用者的接口和实现。

---

## 5. 目录设计

```text
Muggle/
├── backend/
│   ├── cmd/
│   │   ├── api/
│   │   │   └── main.go
│   │   ├── admin/
│   │   │   └── main.go              # 阶段 3：离线创建管理员
│   │   └── worker/
│   │       └── main.go              # 阶段 6：Kafka Worker
│   ├── configs/
│   │   └── default.yaml             # 非敏感默认配置
│   ├── docs/                         # OpenAPI 生成结果
│   ├── internal/
│   │   ├── app/
│   │   │   └── bootstrap.go         # 依赖组装和生命周期
│   │   ├── config/
│   │   │   └── config.go
│   │   ├── httpapi/
│   │   │   ├── router.go
│   │   │   ├── response.go
│   │   │   └── middleware/
│   │   ├── platform/
│   │   │   ├── database/
│   │   │   ├── logger/
│   │   │   ├── rediscache/          # 阶段 5 再创建
│   │   │   └── kafka/               # 阶段 6 再创建
│   │   ├── admin/
│   │   │   ├── model.go
│   │   │   ├── dto.go
│   │   │   ├── repository.go
│   │   │   ├── repository_gorm.go
│   │   │   ├── service.go
│   │   │   ├── handler.go
│   │   │   └── routes.go
│   │   ├── post/
│   │   │   ├── model.go
│   │   │   ├── dto.go
│   │   │   ├── repository.go
│   │   │   ├── repository_gorm.go
│   │   │   ├── service.go
│   │   │   ├── handler.go
│   │   │   └── routes.go
│   │   └── view/                     # 阶段 6 再创建
│   │       ├── event.go
│   │       ├── producer.go
│   │       └── consumer.go
│   ├── migrations/
│   ├── Dockerfile                    # 阶段 8 再创建
│   ├── Makefile
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── app/                      # Router、Provider、主题
│   │   ├── api/                      # Axios 实例和拦截器
│   │   ├── features/
│   │   │   ├── health/
│   │   │   ├── auth/
│   │   │   └── posts/
│   │   ├── pages/
│   │   ├── components/               # 真正跨功能复用的组件
│   │   ├── layouts/
│   │   └── styles/
│   ├── nginx.conf                    # 阶段 8 再创建
│   ├── Dockerfile                    # 阶段 8 再创建
│   └── package.json
├── deploy/
│   ├── compose.yaml
│   ├── kind.yaml                     # 阶段 10 再创建
│   └── k8s/                          # 阶段 10 再创建
├── docs/
│   ├── api.md
│   ├── architecture.md
│   ├── backlog.md
│   └── failure-tests.md
├── .github/workflows/                # 阶段 7 再创建
├── .env.example
├── .gitignore
├── Makefile                          # 调用前后端子项目命令
└── README.md
```

目录约束：

- 阶段 1 只创建 `api`、配置、日志、HTTP 和 health 前端所需目录。
- `platform` 只放技术适配，不放业务规则。
- `httpapi` 只放跨业务 HTTP 能力，不变成 `utils` 大杂烩。
- 测试文件优先与被测包放在一起。
- 只有阶段 6 才创建 `cmd/worker` 和 Kafka 目录。
- 只有阶段 10 才创建 Kubernetes 清单。

---

## 6. 数据设计

### 6.1 通用约定

- 表名和列名使用 `snake_case`。
- 主键使用 `BIGINT UNSIGNED AUTO_INCREMENT`。
- 字符集使用 `utf8mb4`。
- 数据库保存 UTC 时间，API 输出 RFC 3339。
- 唯一性、非空、外键和必要状态约束由 migration 明确定义。
- GORM Tag 描述映射，不代替数据库约束。
- 每次查询使用 `db.WithContext(ctx)`。
- 生产结构只由 golang-migrate 管理。
- 敏感 Token 只保存不可逆哈希。

### 6.2 `posts`（阶段 2）

| 字段 | 类型建议 | 规则 |
|---|---|---|
| `id` | `BIGINT UNSIGNED` | 自增主键 |
| `slug` | `VARCHAR(160)` | 唯一、非空 |
| `title` | `VARCHAR(200)` | 非空 |
| `summary` | `VARCHAR(500)` | 非空，默认空字符串 |
| `content_md` | `LONGTEXT` | Markdown 原文、非空 |
| `status` | `VARCHAR(20)` | `draft` 或 `published` |
| `published_at` | `DATETIME(3)` | 可空，首次发布写入 |
| `version` | `BIGINT UNSIGNED` | 默认 1，乐观锁 |
| `created_at` | `DATETIME(3)` | 非空 |
| `updated_at` | `DATETIME(3)` | 非空 |

建议索引：

- 唯一索引：`slug`。
- 公开列表：`(status, published_at, id)`。
- 管理列表：`(status, updated_at, id)`。

乐观锁更新：

```sql
UPDATE posts
SET title = ?, summary = ?, content_md = ?, version = version + 1, updated_at = ?
WHERE id = ? AND version = ?;
```

更新零行时，Service 需要区分文章不存在和版本冲突。

### 6.3 `admins`（阶段 3）

| 字段 | 类型建议 | 规则 |
|---|---|---|
| `id` | `BIGINT UNSIGNED` | 自增主键 |
| `username` | `VARCHAR(64)` | 唯一、非空 |
| `password_hash` | `VARCHAR(255)` | bcrypt 哈希、非空 |
| `status` | `VARCHAR(16)` | `active` 或 `disabled` |
| `created_at` | `DATETIME(3)` | 非空 |
| `updated_at` | `DATETIME(3)` | 非空 |

规则：

- v1.0 不提供网页创建管理员。
- 初始管理员通过 `cmd/admin` 离线创建。
- 禁用管理员时必须拒绝新的登录和 Refresh。

### 6.4 `refresh_sessions`（阶段 3）

| 字段 | 类型建议 | 规则 |
|---|---|---|
| `id` | `BIGINT UNSIGNED` | 自增主键 |
| `admin_id` | `BIGINT UNSIGNED` | 外键、索引、非空 |
| `token_hash` | `CHAR(64)` | Refresh Token SHA-256、唯一 |
| `expires_at` | `DATETIME(3)` | 索引、非空 |
| `revoked_at` | `DATETIME(3)` | 可空 |
| `created_at` | `DATETIME(3)` | 非空 |

Refresh 轮转必须在一个事务中完成：

1. 检查旧 Session 未撤销、未过期且管理员仍为 `active`。
2. 条件撤销旧 Session。
3. 创建新 Refresh Session。
4. 事务成功后才向客户端设置新 Cookie。

### 6.5 `post_stats`（阶段 6）

| 字段 | 类型建议 | 规则 |
|---|---|---|
| `post_id` | `BIGINT UNSIGNED` | 主键、外键 |
| `view_count` | `BIGINT UNSIGNED` | 默认 0 |
| `updated_at` | `DATETIME(3)` | 非空 |

### 6.6 `processed_events`（阶段 6）

| 字段 | 类型建议 | 规则 |
|---|---|---|
| `event_id` | `CHAR(36)` | 主键 |
| `event_type` | `VARCHAR(80)` | 非空 |
| `processed_at` | `DATETIME(3)` | 非空 |

Worker 在一个 GORM 事务中：

1. 插入 `processed_events`。
2. 如果事件已经存在，按“已处理”结束，不重复增加计数。
3. 如果是首次处理，使用 Upsert 创建或更新 `post_stats`：不存在时写入 1，存在时执行 `view_count = view_count + 1`。
4. 文章详情使用 `LEFT JOIN` 和 `COALESCE(view_count, 0)`，因此阶段 6 之前创建的文章不需要预先补统计行。
5. 任一步失败全部回滚。
6. MySQL 事务提交后才提交 Kafka offset。

---

## 7. Redis 与 Kafka 设计

### 7.1 Redis Key

| Key | 内容 | TTL |
|---|---|---|
| `post:slug:{slug}` | 已发布文章内容 DTO，不含浏览量 | 5 分钟 |
| `posts:list:v{version}:{page}:{size}` | 已发布文章列表 | 1 分钟 |
| `posts:list:version` | 列表缓存版本 | 不过期 |

缓存规则：

- 公开文章内容使用 Cache-Aside。
- 第一次读取 MySQL 后写缓存，第二次命中缓存。
- 详情缓存不保存 `view_count`，浏览量从 `post_stats` 读取，避免计数更新后长期显示旧值。
- 文章修改后删除旧 Slug 和新 Slug 的详情缓存。
- 发布或取消发布后增加列表版本号。
- 不使用 Redis `KEYS` 做通配删除。
- Redis 超时或故障时记录降级日志并回源 MySQL。
- Redis 清空不会影响登录状态、文章事实数据和浏览量事实数据。

### 7.2 Kafka 事件

Topic：`blog.post-viewed.v1`  
Partition：3  
本地 replication factor：1  
Message key：文章 ID 字符串  
Consumer group：`blog-view-counter-v1`

事件格式：

```json
{
  "event_id": "4d6314d3-f834-4e26-84a1-afe7f940b5dd",
  "event_type": "post.viewed.v1",
  "post_id": 12,
  "occurred_at": "2026-08-25T12:00:00Z"
}
```

规则：

- API 只有成功读取公开文章后才产生事件。
- 事件发送失败只写结构化日志，不改变文章响应。
- `event_id` 每次浏览唯一。
- 事件类型带版本号，不原地改变旧契约。
- 同一文章使用同一 Message key，以保持分区内顺序。
- Worker 消费必须幂等。
- Worker 成功提交数据库事务后才提交 offset。

v1.0 明确限制：API 采用尽力发送，Kafka 不可用期间可能少算浏览量。这是学习阶段有意保留的权衡，不把浏览量升级为文章主事务，也暂不引入 Transactional Outbox。

---

## 8. API 契约

统一前缀：`/api/v1`。

### 8.1 健康检查

| 方法 | 路径 | 状态 | 说明 |
|---|---|---:|---|
| GET | `/api/v1/health/live` | 200 | 只证明 API 进程可响应，不访问依赖 |
| GET | `/api/v1/health/ready` | 200/503 | 短超时检查 MySQL |

Redis 和 Kafka 故障只在 readiness 详情中标记为 degraded，不让公开文章 API 整体下线。Worker readiness 则必须同时检查 MySQL 和 Kafka。

### 8.2 公开文章

| 方法 | 路径 | 成功状态 | 说明 |
|---|---|---:|---|
| GET | `/api/v1/posts?page=1&page_size=10` | 200 | 已发布文章列表 |
| GET | `/api/v1/posts/:slug` | 200/404 | 已发布文章详情，阶段 6 起产生浏览事件 |

### 8.3 管理员认证

| 方法 | 路径 | 成功状态 | 说明 |
|---|---|---:|---|
| POST | `/api/v1/admin/session` | 200 | 登录，返回 Access Token 并设置 Refresh Cookie |
| POST | `/api/v1/admin/session/refresh` | 200 | 轮转 Refresh Session，返回新 Access Token |
| DELETE | `/api/v1/admin/session` | 204 | 撤销当前 Session 并清 Cookie |
| GET | `/api/v1/admin/me` | 200 | 返回当前管理员摘要 |

除登录和 Refresh 外，管理 API 使用：

```http
Authorization: Bearer <access-token>
```

### 8.4 管理文章

| 方法 | 路径 | 成功状态 | 说明 |
|---|---|---:|---|
| GET | `/api/v1/admin/posts` | 200 | 查看草稿和已发布文章 |
| GET | `/api/v1/admin/posts/:id` | 200/404 | 编辑页读取完整内容 |
| POST | `/api/v1/admin/posts` | 201 | 创建草稿 |
| PUT | `/api/v1/admin/posts/:id` | 200/409 | 修改文章并校验 version |
| POST | `/api/v1/admin/posts/:id/publish` | 200/409 | 发布 |
| POST | `/api/v1/admin/posts/:id/unpublish` | 200/409 | 取消发布 |

### 8.5 响应格式

普通成功响应：

```json
{
  "data": {
    "id": 12,
    "title": "Gin Engine 是什么"
  }
}
```

列表响应：

```json
{
  "data": [],
  "meta": {
    "page": 1,
    "page_size": 10,
    "total": 0
  }
}
```

错误响应：

```json
{
  "error": {
    "code": "POST_VERSION_CONFLICT",
    "message": "文章已被其他操作修改，请刷新后重试",
    "request_id": "01J..."
  }
}
```

错误映射：

| 业务错误 | HTTP 状态 |
|---|---:|
| 参数或格式错误 | 400 |
| 未认证或 Token 失效 | 401 |
| 资源不存在 | 404 |
| Slug 或版本冲突 | 409 |
| 必要依赖暂时不可用 | 503 |
| 未分类内部错误 | 500 |

### 8.6 Swagger/OpenAPI

- 阶段 2 从第一个真实文章 API 开始维护文档。
- Handler 注释、生成文件和实际响应必须一致。
- 开发环境可以暴露 Swagger UI。
- 完整 Compose 和 Kubernetes 默认不向公网暴露 Swagger UI。
- 阶段 7 在 CI 中检查生成文件是否过期。

---

## 9. 配置、日志与安全

### 9.1 配置来源和优先级

```text
Viper 默认值
   < configs/default.yaml
   < 进程环境变量 / Docker Secret / Kubernetes Secret
```

本地 `.env` 的定位：

- `.env.example` 可以提交，只列配置名和安全示例。
- `.env` 不提交。
- `.env` 由 Make、Shell 或 Docker Compose 注入进程环境；不要误以为 Go 或 Viper 会无条件自动读取任意 `.env`。
- Viper 负责把配置文件与已经存在的环境变量合并进强类型 `Config`。

阶段 1 最小配置：

```dotenv
APP_ENV=development
HTTP_ADDR=:8080
LOG_LEVEL=debug
HTTP_READ_HEADER_TIMEOUT=5s
HTTP_READ_TIMEOUT=10s
HTTP_WRITE_TIMEOUT=15s
HTTP_IDLE_TIMEOUT=60s
HTTP_SHUTDOWN_TIMEOUT=10s
```

阶段 2 增加：

```dotenv
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_DATABASE=tiny_blog
MYSQL_USER=blog
MYSQL_PASSWORD=change-me
MYSQL_MAX_OPEN_CONNS=20
MYSQL_MAX_IDLE_CONNS=10
MYSQL_CONN_MAX_LIFETIME=30m
```

阶段 3 增加：

```dotenv
JWT_SECRET=replace-with-a-long-random-secret
ACCESS_TOKEN_TTL=15m
REFRESH_SESSION_TTL=168h
REFRESH_COOKIE_NAME=muggle_refresh
REFRESH_COOKIE_SECURE=false
REFRESH_COOKIE_SAME_SITE=lax
PUBLIC_ORIGIN=http://localhost:5173
```

阶段 5、6 分别增加 Redis 与 Kafka 配置。程序只校验当前二进制实际启用的配置，阶段 1 不因缺少 MySQL、Redis 或 Kafka 配置而启动失败。

配置规则：

- 启动时一次性读取并校验，不做运行时热更新。
- `main`/`bootstrap` 将强类型配置注入依赖。
- Handler、Service 和 Repository 不直接调用 `os.Getenv` 或 Viper。
- 生产环境检测到空 Secret、示例 Secret 或危险通配来源时拒绝启动。
- 容器内 Host 使用服务名 `mysql`、`redis`、`kafka`，不能使用 `localhost`。

### 9.2 Zap 日志

开发环境使用易读 Console Encoder；Compose/Kubernetes 使用 JSON Encoder。

访问日志至少包含：

- `timestamp`
- `level`
- `service`
- `environment`
- `request_id`
- `method`
- `route`
- `status`
- `latency_ms`
- `admin_id`（认证成功后且允许记录时）
- `error_code`

禁止记录：

- 密码和密码哈希。
- Access Token、Refresh Token 和 Cookie。
- 完整数据库 DSN。
- 完整文章正文。
- Kafka 消息中的未来敏感字段。

### 9.3 Middleware 顺序

```text
Request ID
  → Access Log
  → Recovery
  → Security Headers / CORS
  → Request Size Limit
  → Authentication（管理路由）
  → Handler
```

使用 `gin.New()` 创建空 Engine，再显式注册 Middleware；不使用 `gin.Default()` 隐式加入 Logger 和 Recovery。

### 9.4 认证安全

- 密码使用 bcrypt 哈希，不保存明文。
- Access JWT 生命周期短，只放在前端内存中的 Zustand Store。
- Refresh Token 使用 32 字节以上安全随机数，只把 SHA-256 哈希保存到 MySQL。
- Refresh Cookie 设置 HttpOnly、SameSite=Lax；生产环境启用 Secure。
- Refresh 每次成功都轮转 Token，旧 Token 立即失效。
- Refresh 和退出检查可信 Origin。
- Axios 只允许一次并发 Refresh；其他 401 请求等待该结果，避免刷新风暴和死循环。
- 后端永远重新检查管理员状态，不只相信长期会话记录。

### 9.5 Markdown 安全

- MySQL 只保存 Markdown 原文 `content_md`。
- ByteMD 预览不代表服务端信任生成结果。
- 默认禁用 Markdown 原始 HTML。
- 链接协议使用允许列表。
- 渲染到浏览器的 HTML 经过安全清洗。
- 文章内容不能注入脚本、样式和事件属性。

---

## 10. 前端设计

### 10.1 页面范围

公开页面：

- `/`：文章列表。
- `/posts/:slug`：文章详情。
- 通用 404 和 API 错误状态。

管理页面：

- `/admin/login`：登录。
- `/admin/posts`：文章管理列表。
- `/admin/posts/new`：创建草稿。
- `/admin/posts/:id/edit`：编辑文章。

### 10.2 状态边界

- Axios 负责 HTTP 请求和统一错误转换。
- Zustand 只保存 Access Token、管理员摘要和认证恢复状态。
- 页面局部 loading、表单输入和弹窗状态留在组件内部。
- 不把所有服务端列表和文章内容复制进全局 Store。
- Access Token 不持久化；刷新页面时通过 Refresh Cookie 恢复登录。

### 10.3 登录恢复

1. React 启动时认证状态为 `checking`。
2. 请求 Refresh API。
3. 成功后把新 Access Token 和管理员摘要放进内存 Store。
4. 失败则切换为 `anonymous`，而不是无限重试。
5. 管理路由在 `checking` 时显示加载状态，在 `anonymous` 时跳转登录。

### 10.4 必须处理的页面状态

每个数据页面至少处理：

- 初次加载。
- 空数据。
- 参数错误。
- 401 登录失效。
- 404 资源不存在。
- 409 版本冲突。
- 500/503 服务错误。
- 网络不可达。
- 提交中防重复点击。

---

## 11. 测试与质量策略

### 11.1 测试分层

| 层级 | 技术 | 重点 |
|---|---|---|
| 纯单元测试 | testing + testify | 状态流转、错误映射、Token 和缓存键 |
| Service 测试 | fake/mock Repository | 业务编排、乐观锁、Refresh 轮转 |
| Handler 测试 | Gin + httptest | 参数、状态码、JSON 和认证 |
| Repository 集成测试 | GORM + 测试 MySQL | 唯一约束、分页、事务和幂等 |
| Redis 集成测试 | 测试 Redis | HIT/MISS、失效和降级 |
| Kafka 集成测试 | 测试 Kafka + MySQL | 重复事件、offset 和恢复 |
| 前端测试 | Vitest + Testing Library | 页面状态、登录恢复和路由守卫 |
| 端到端验收 | 浏览器 + 运行环境 | 登录、发布、阅读、浏览量、取消发布 |

### 11.2 阶段持续执行

每个阶段至少运行：

```bash
go fmt ./...
go vet ./...
go test ./...
npm run lint
npm run typecheck
npm run test
npm run build
```

不存在相应前端或集成测试时，阶段计划中必须明确何时补齐，不能用空脚本伪造通过。

### 11.3 功能完成定义

一个功能只有同时满足以下条件才算完成：

- 正常路径和主要失败路径均已实现。
- Handler、Service 和 Repository 边界没有被绕过。
- migration、DTO、OpenAPI 和前端类型同步更新。
- 关键规则有自动化测试。
- 日志可以通过 Request ID 定位错误且不泄露敏感数据。
- 全新数据库可以重复完成本阶段验收。

---

## 12. 分阶段实施计划

### 12.1 阶段依赖总览

```text
阶段 0  环境与仓库基线
   ↓
阶段 1  Gin + React 工程骨架
   ↓
阶段 2  MySQL + 公开文章读取
   ↓
阶段 3  管理员认证与会话
   ↓
阶段 4  管理端写作与发布闭环
   ↓
阶段 5  Redis Cache-Aside
   ↓
阶段 6  Kafka 异步浏览量
   ↓
阶段 7  API 契约、测试与 CI 收口
   ↓
阶段 8  Nginx + 完整 Docker Compose
   ↓
阶段 9  故障演练与工程韧性
   ↓
阶段 10 本地 Kubernetes
```

每个阶段只依赖前面已经交付的能力：

| 阶段 | 直接复用的前置产物 | 新增能力 |
|---|---|---|
| 1 | Go Module、仓库与工具 | 可运行 HTTP/React 骨架 |
| 2 | Router、配置、日志、Axios | MySQL 公开读取链路 |
| 3 | MySQL、错误响应、前端 Router | 管理员登录和 Refresh |
| 4 | 认证、GORM、文章公开模型 | 管理写入和 ByteMD |
| 5 | 已稳定的文章读写语义 | 可失效、可降级缓存 |
| 6 | 文章详情和 MySQL 事务 | Kafka 事件与 Worker |
| 7 | 完整业务闭环 | 契约、测试和 CI 门禁 |
| 8 | 稳定二进制和配置边界 | 完整容器运行环境 |
| 9 | 可重复 Compose 环境 | 依赖故障和恢复证据 |
| 10 | 健康检查、优雅关闭、镜像 | Kubernetes 部署实验 |

### 阶段 0：环境与仓库基线

预计：0.5～1 天。

**目标**

建立一致的 WSL2 开发环境和干净仓库，不实现业务。

**实现顺序**

1. 安装或更新 WSL2 Ubuntu。
2. Docker Desktop 启用 WSL2 backend 和 Ubuntu Integration。
3. 把代码放在 WSL Linux 文件系统，例如 `/home/<user>/code/Muggle`。
4. 安装 Go、Node.js、Git、Make 和 Docker CLI。
5. Clone 或初始化 `github.com/psking307/Muggle`。
6. 创建 `backend` 并执行：

   ```bash
   go mod init github.com/psking307/Muggle/backend
   ```

7. 创建 README、`.gitignore`、`.env.example` 和最小目录。
8. 不提前安装 Redis、Kafka、GORM、JWT、ByteMD 等后续依赖。

**验收**

```bash
go version
node --version
git --version
docker version
docker compose version
docker run --rm hello-world
```

- 所有命令在 WSL 成功。
- WSL 中的 Docker CLI 连接 Docker Desktop，而不是第二套 Docker Engine。
- `backend/go.mod` 的 module 是 `github.com/psking307/Muggle/backend`。
- 仓库没有 Secret 和无调用者的空目录。

Git tag：`v0.0-environment`。

### 阶段 1：Gin + React 工程骨架

预计：1～2 天。

本阶段不启动 MySQL、Redis 和 Kafka。

**目标**

建立配置、日志、HTTP 生命周期、测试和前端调用的最小闭环。

**实现顺序**

1. 引入 Gin、Viper、Zap、validator 和 testify。
2. 创建阶段 1 的 `Config`，只包含应用、日志和 HTTP 配置。
3. Viper 加载 `configs/default.yaml` 与环境变量，并映射为强类型 Config。
4. 使用 validator 或明确代码校验监听地址和超时。
5. 初始化 Zap：开发 Console、生产 JSON。
6. 创建 `httpapi.NewRouter()`，内部使用 `gin.New()`。
7. 注册 Request ID、访问日志和 Recovery Middleware。
8. 注册 `GET /api/v1/health/live`，返回 `{"status":"ok"}`。
9. 使用显式 `http.Server` 设置 ReadHeader、Read、Write、Idle Timeout。
10. 实现 SIGINT/SIGTERM 优雅关闭和日志同步。
11. 使用 `httptest` 测试 live Handler、404 和 Recovery。
12. 创建 React + TypeScript + Vite 项目。
13. 引入 React Router、Ant Design、Tailwind CSS 和 Axios。
14. 创建 Axios 实例，base URL 使用 `/api/v1`，设置合理超时。
15. Vite 把 `/api` 代理到 `http://localhost:8080`。
16. 首页调用 `/health/live`，分别显示正常、加载和 API 不可达状态。
17. 增加 `make dev-api`、`make dev-web`、`make test`、`make lint`。

**推荐提交顺序**

1. `chore: initialize backend module and minimal config`
2. `feat: add gin router and liveness endpoint`
3. `feat: add request middleware and graceful server`
4. `test: cover health route and recovery`
5. `feat: initialize react app and api status page`

**验收**

- `make dev-api` 可以启动 API。
- `curl http://localhost:8080/api/v1/health/live` 返回 200。
- 浏览器显示 API 正常；停止 API 后显示清晰错误。
- 日志包含 Request ID、路径、状态和耗时。
- Handler panic 被 Recovery 转成 500，进程不退出。
- Ctrl+C 后 API 在超时内优雅退出。
- Go 测试、vet、前端 lint、类型检查和构建通过。
- 代码中没有 MySQL、Redis、Kafka、JWT、Zustand 和 ByteMD 的未使用配置。

Git tag：`v0.1-skeleton`。

### 阶段 2：MySQL + GORM 公开文章读取

预计：2～3 天。

**进入条件**

- 阶段 1 验收通过。
- Config、Logger、Router 和前端 Axios 已可复用。

**目标**

完成第一条真实业务链路：MySQL 中的已发布文章能够通过 API 和 React 页面读取。

**实现顺序**

1. Compose 只加入 MySQL 8 和 named volume。
2. 开发 profile 映射 `127.0.0.1:3306:3306` 供 DataGrip 使用。
3. 增加 MySQL 配置及阶段性校验。
4. 引入 GORM MySQL Driver，配置连接池和 Ping 超时。
5. 引入 golang-migrate，创建 `posts` 初始 migration。
6. 创建两篇已发布文章和一篇草稿的显式 seed。
7. 实现 Post Model、公开 DTO 和 Repository 接口。
8. 实现 GORM Repository，所有查询传入 Context。
9. 实现公开列表和详情 Service。
10. 使用 validator 校验分页 DTO；业务状态规则仍由 Service 校验。
11. 实现公开 Gin Handler 和路由。
12. 增加 `/health/ready`，只检查 MySQL。
13. 从本阶段开始生成 Swagger/OpenAPI。
14. React 实现文章列表和详情页。
15. 处理 loading、空列表、404、非法分页、500 和网络错误。
16. 使用 testify 编写 Service 测试、Handler 测试和 Repository 集成测试。

**验收命令**

```bash
docker compose up -d mysql
make migrate-up
make seed
make dev-api
make dev-web
make test
```

**验收标准**

- DataGrip 可以看到 `posts` 表及三篇 seed 数据。
- 公开 API 只返回已发布文章。
- 草稿 Slug 返回 404，不泄露其存在性。
- 非法分页返回稳定的 400 错误。
- MySQL 停止时 ready 返回 503，live 仍返回 200。
- 重启 MySQL 容器后数据仍存在。
- GORM 不执行 AutoMigrate。
- Swagger 与实际 DTO、状态码一致。

Git tag：`v0.2-public-posts`。

### 阶段 3：管理员认证与会话

预计：2 天。

**进入条件**

- MySQL migration 和 Repository 模式已经稳定。
- 前端统一错误处理和代理可用。

**目标**

实现管理员安全登录、Access JWT、Refresh Session 轮转、登录恢复和退出。本阶段不编辑文章。

**实现顺序**

1. 创建 `admins` 和 `refresh_sessions` migrations。
2. 增加 JWT、Refresh Cookie 和 Public Origin 配置校验。
3. 创建 `cmd/admin` 离线命令，交互式输入用户名和密码。
4. 使用 bcrypt 生成密码哈希，不输出密码或哈希。
5. 实现管理员 Repository 和认证 Service。
6. 登录成功生成短期 Access JWT 和随机 Refresh Token。
7. MySQL 只保存 Refresh Token SHA-256。
8. 实现 Refresh Session 事务轮转和旧 Token 重放拒绝。
9. 实现登录、Refresh、退出、`/admin/me` Handler。
10. 增加 Bearer JWT 认证 Middleware。
11. 前端引入 Zustand，保存内存 Access Token 和管理员摘要。
12. Axios 请求拦截器附加 Access Token。
13. Axios 响应层实现单次并发 Refresh，失败后统一退出。
14. 实现登录页、管理路由守卫和刷新页面后的会话恢复。
15. 更新 Swagger、错误码和安全文档。
16. 测试错误密码、禁用账号、过期 JWT、Refresh 轮转、重放、退出和伪造 JWT。

**验收标准**

- 初始管理员只能通过离线命令创建。
- 错误密码返回 401，日志不包含密码。
- 登录返回 Access Token，并设置 HttpOnly Refresh Cookie。
- JavaScript 无法读取 Refresh Cookie。
- 刷新页面可以恢复登录，但 Access Token 不进入 LocalStorage。
- 同一旧 Refresh Token 只能成功轮转一次。
- 退出后原 Refresh Token 不再有效。
- 未认证请求所有管理 API 都得到 401。
- Redis 尚未启动，认证流程仍完整可用。

Git tag：`v0.3-admin-auth`。

### 阶段 4：管理端写作与发布闭环

预计：2～3 天。

**进入条件**

- 管理员登录和 Bearer 认证稳定。
- 公开文章读取已经可用。

**目标**

完成从登录、创建草稿、ByteMD 编辑、发布到公开阅读的业务闭环。

**实现顺序**

1. 实现管理员文章列表和详情 Repository 方法。
2. 实现创建草稿、更新、发布和取消发布 Service。
3. 创建草稿时校验标题、Slug、摘要和 Markdown 正文。
4. 使用显式字段更新，不使用不受控制的整对象 `Save`。
5. 使用 `WHERE id = ? AND version = ?` 实现乐观锁。
6. 限制允许的状态流转，Repository 不能任意写状态字符串。
7. 实现管理文章 Handler 和路由。
8. 前端引入 ByteMD，完成新建和编辑页面。
9. 对预览 HTML 进行安全清洗并禁用原始 HTML。
10. 使用 Ant Design 实现表单、校验反馈、表格和确认操作。
11. 处理 401、404、409、保存中、发布中和失败重试。
12. 补齐 Handler、Service、Repository 和前端测试。
13. 完成端到端手工验收：登录 → 草稿 → 编辑 → 发布 → 公开读取 → 取消发布。

**验收标准**

- 新文章只能以草稿创建。
- 草稿不出现在公开页面。
- 发布后立即公开可见，取消发布后立即隐藏。
- 标题修改不会自动改变 Slug。
- 两个页面编辑同一文章时，旧版本保存得到 409。
- 未认证请求不能创建、修改或发布文章。
- Markdown 中的脚本和危险 HTML 不能执行。
- 本阶段仍不依赖 Redis 和 Kafka。

Git tag：`v0.4-admin-posts`。

### 阶段 5：Redis Cache-Aside

预计：1～2 天。

**进入条件**

- 公开读取和管理写入的语义、错误和缓存失效时机已经稳定。

**目标**

为公开文章列表和内容增加可删除、可重建、故障可降级的缓存。

**实现顺序**

1. Compose 加入 Redis。
2. 增加 Redis 配置、短连接超时和 go-redis Client。
3. 定义 Post Cache 接口，不让 Service 直接依赖 Redis 类型。
4. 实现详情 Cache-Aside，缓存不包含浏览量。
5. 实现列表 Cache-Aside 和列表版本号。
6. 开发环境响应 `X-Cache: HIT`、`MISS` 或 `BYPASS`。
7. 修改文章后删除相关详情缓存。
8. 发布或取消发布后增加列表版本。
9. Redis 故障时记录降级日志并回源 MySQL。
10. 测试命中、过期、写缓存失败、删除失败和 Redis 停止场景。

**验收标准**

- 第一次详情请求为 MISS，第二次为 HIT。
- 文章修改后立即看到新内容。
- 发布状态变化后列表立即正确。
- 停止 Redis 后公开文章仍可从 MySQL 读取。
- 停止 Redis 不影响管理员登录和 Refresh Session。
- 清空 Redis 不丢失任何业务数据。

Git tag：`v0.5-redis-cache`。

### 阶段 6：Kafka 异步浏览量

预计：2～3 天。

**进入条件**

- 文章详情读取稳定。
- MySQL 事务、优雅关闭和配置模式可复用。

**目标**

完成“读取文章 → Kafka 事件 → Worker 幂等更新浏览量”的异步链路。

**实现顺序**

1. Compose 加入 Kafka 单节点 KRaft broker。
2. 创建 `blog.post-viewed.v1`，3 个 partition。
3. 创建 `post_stats` 和 `processed_events` migrations。
4. 增加 Kafka Topic、Broker 和 Consumer Group 配置。
5. 定义版本化 View Event DTO。
6. 使用 franz-go 实现 API Producer。
7. 公开文章详情成功后异步尽力发送事件。
8. 创建 `cmd/worker` 和 Worker 独立启动生命周期。
9. Worker 使用固定 Consumer Group 消费。
10. 使用 GORM Transaction 插入幂等表并原子增加计数。
11. 事务成功后才提交 offset。
12. Worker 收到 SIGTERM 后停止拉取新事件并完成当前处理。
13. Worker 提供仅供 Compose/Kubernetes 探测的内部 live/ready 端点；ready 检查 MySQL 和 Kafka。
14. 详情 API 通过 `LEFT JOIN` 返回当前 `view_count`，没有统计行时返回 0；详情内容缓存不缓存计数。
15. 前端显示最终一致的浏览量说明。
16. 测试重复事件、非法事件、Worker 重启、Kafka 停止和两个 Worker 分区分配。

**验收标准**

- 打开文章后浏览量稍后增加。
- 相同 `event_id` 发送两次只增加一次。
- Worker 停止后事件积压，恢复后继续处理。
- 两个 Worker 共享 partition，不重复统计。
- Kafka 停止时文章仍然可读。
- Kafka 停止期间允许少算浏览量，README 明确记录限制。
- MySQL 事务失败时不提交 Kafka offset。

Git tag：`v0.6-kafka-views`。

### 阶段 7：API 契约、测试与 CI 收口

预计：1～2 天。

**进入条件**

- 登录、文章管理、Redis 和 Kafka 主流程均已实现。

**目标**

停止新增功能，把已有系统收口为契约一致、可重复测试、可进入容器化的发布候选版本。

**实现顺序**

1. 统一成功响应、分页 Meta、错误码和状态码。
2. 审查所有 Context、外部调用超时和资源关闭。
3. 补齐 Swagger/OpenAPI 并检查生成文件一致性。
4. 补齐权限、Token、乐观锁、缓存失效和 Kafka 幂等测试。
5. 固化 CORS、可信代理、Cookie、安全响应头和请求体限制。
6. 前端补齐所有 loading、empty、401、404、409、500、503 和 offline 状态。
7. 建立 GitHub Actions：Go fmt/vet/test/build。
8. 建立前端 lint/typecheck/test/build。
9. CI 在空 MySQL 上执行 migration 和关键 Repository 集成测试。
10. 检查 Swagger 生成结果和代码是否同步。
11. 更新 README、API 文档、架构说明和开发命令。

**验收标准**

- CI 能阻止编译失败、测试失败、前端类型错误、migration 失败和过期 OpenAPI。
- 全新 MySQL 可以从零 migration、seed 并完成核心验收。
- 任意 500 可以通过 Request ID 在 Zap 日志中定位。
- 响应和日志不泄露 Secret、Token、Cookie、SQL 细节和正文。
- 本阶段没有新增业务功能和基础设施。

Git tag：`v0.7-quality`。

### 阶段 8：Nginx + 完整 Docker Compose

预计：1～2 天。

**进入条件**

- 阶段 7 发布候选版本通过全部质量门禁。
- 配置外部化、健康检查和优雅关闭已经稳定。

**目标**

把 API、Worker、Web、MySQL、Redis 和 Kafka 组成一套可重复启动的完整环境。

**实现顺序**

1. Go 使用多阶段 Dockerfile 构建 API、Worker 和 Admin 命令。
2. 最终镜像使用非 root 用户并只包含运行所需文件。
3. React Dockerfile 第一阶段执行生产构建。
4. 第二阶段使用 Nginx 提供静态文件。
5. Nginx `/api/` 反向代理到 `api:8080`。
6. 非文件前端路由 fallback 到 `index.html`。
7. Compose 加入 MySQL、Redis、Kafka、API、Worker、Web 和 migration job。
8. 为依赖增加 healthcheck，区分启动顺序和真正就绪。
9. migration 作为显式一次性任务，API 启动不自动改表。
10. 完整模式只对外暴露 Nginx `8080`。
11. MySQL 端口映射放入 debug profile。
12. 配置 named volume、内部网络和合理日志轮转。

**验收**

```bash
docker compose up --build
```

- `http://localhost:8080` 是唯一 Web 入口。
- 刷新 React 详情和管理路由不返回 Nginx 404。
- 容器使用 `mysql:3306`、`redis:6379`、`kafka:9092`。
- `docker compose down` 后重启，MySQL 数据仍存在。
- API 和 Worker 收到终止信号后优雅退出。
- Secret 不在镜像、Git 和普通日志中。
- DataGrip 调试只在显式 debug profile 开放端口。

Git tag：`v0.8-compose`。

### 阶段 9：故障演练与工程韧性

预计：1～2 天。本阶段不增加业务功能。

**进入条件**

- 完整 Compose 环境可以重复启动。

**目标**

验证每个依赖失败时系统是否按设计降级、恢复和记录信息。

**实现顺序**

1. 确认 API live 只检查进程。
2. 确认 API ready 只把 MySQL 视为公开业务必要依赖。
3. Redis、Kafka 状态显示 degraded，但不让 API 整体下线。
4. Worker ready 同时检查 MySQL 和 Kafka。
5. 检查所有外部调用的 Context Timeout。
6. 检查连接池、客户端关闭和 SIGTERM 行为。
7. 编写 `docs/failure-tests.md` 并逐项记录命令、结果和恢复步骤。
8. 补充 MySQL volume 备份和恢复练习。

**故障矩阵**

| 操作 | 预期结果 |
|---|---|
| 停 MySQL | API ready 失败；live 正常；文章与认证不可用 |
| 恢复 MySQL | API 无需重启即可恢复或给出明确重连行为 |
| 停 Redis | 公开文章回源 MySQL；登录和管理不受影响 |
| 停 Kafka | 文章可读；浏览量不再增长或少算 |
| 停 Worker | Kafka 事件积压；恢复后继续消费 |
| 重启 API | Nginx 短暂错误；API 恢复后正常 |
| 重复发送事件 | 浏览量只增加一次 |
| 发送非法事件 | Worker 记录错误且不崩溃，处理策略写入文档 |
| SIGTERM API | 停止新请求并等待在途请求 |
| SIGTERM Worker | 停止拉取并完成当前事件 |

**验收标准**

- 故障矩阵全部有实际结果，不只写预期。
- 每次 5xx 或降级都能通过 Request ID 或结构化字段定位。
- 恢复依赖后不需要清库重建。
- 备份可以恢复到新的 MySQL volume。
- 日志没有敏感信息。

Git tag：`v0.9-resilience`。

### 阶段 10：本地 Kubernetes

预计：3～5 天。

**进入条件**

- Compose 完整环境和故障矩阵稳定。
- 镜像、外部配置、健康检查、优雅关闭和持久化语义已经验证。

**目标**

不拆微服务，把同一系统原样部署到本地 kind，练习 Kubernetes 对象和应用生命周期。

**实现顺序**

1. 在 WSL Ubuntu 中安装 `kubectl` 和 `kind`。
2. 验证 `kubectl`、`kind` 和 Docker CLI 均在 WSL 中可用，且 Docker CLI 连接的是 Docker Desktop WSL2 backend。
3. 手写 `deploy/kind.yaml`，创建本地 kind 集群。
4. 创建 `blog-lab` Namespace。
5. 用 ConfigMap 保存非敏感应用和 Nginx 配置。
6. 用 Secret 保存 MySQL 密码和 JWT Secret；真实 Secret 不提交 Git。
7. 创建 MySQL、Kafka StatefulSet、Service 和 PVC。
8. Redis 作为可重建缓存使用 Deployment 和 Service，不挂载持久卷。
9. 创建 migration Job 和 Kafka topic 初始化 Job。
10. 创建 API、Worker、Web Deployment 和 Service。
11. 配置 API/Worker liveness、readiness 和 termination grace period。
12. 设置 requests/limits 和非 root Security Context。
13. 构建本地镜像并通过 `kind load docker-image` 加载。
14. 使用 NodePort 或端口映射暴露 Web。
15. 演练 Pod 删除、扩缩容、滚动更新、错误配置和持久化恢复。

**工具验收**

```bash
kubectl version --client
kind version
docker version
```

- 所有命令在 WSL 中成功。
- `kind` 使用 Docker Desktop 提供的 Docker Engine 创建集群，不在 WSL 中另行安装第二套 Docker Engine。

**资源清单**

| 类型 | 资源 |
|---|---|
| Namespace | `blog-lab` |
| ConfigMap | API、Worker、Nginx 非敏感配置 |
| Secret | MySQL 密码和 JWT Secret |
| StatefulSet | MySQL、Kafka |
| PVC | MySQL、Kafka 数据 |
| Service | MySQL、Redis、Kafka、API、Web |
| Job | migration、Kafka topic 初始化 |
| Deployment | API、Worker、Web、Redis |
| NodePort | Web 本地入口 |

**必须完成的实验**

1. 浏览器访问完整网站。
2. 删除 API Pod，观察 Deployment 自动补回。
3. 删除 Worker Pod，恢复后继续消费。
4. 删除 MySQL Pod，重新创建后数据仍存在。
5. API 从 1 个副本扩到 2 个。
6. Worker 从 1 个副本扩到 2 个，观察 Consumer Group 分区。
7. 修改镜像标签并完成滚动更新和回滚。
8. 故意配置错误 MySQL 地址，观察 readiness 和日志。
9. 使用 `kubectl port-forward service/mysql 3307:3306` 从 DataGrip 查看数据。
10. 恢复全部配置并通过端到端检查。

**边界说明**

- 本地单副本 MySQL、Redis 和 Kafka 不代表生产高可用方案。
- Kubernetes 阶段仍不拆微服务。
- 第一遍不使用 Helm，先理解 YAML 对象关系。
- Compose 和 Kubernetes 完整环境不要同时运行，避免端口和内存冲突。

Git tag：`v1.0-kubernetes`。

---

## 13. 日常开发命令

根 Makefile 最终提供：

```text
make dev-infra          # 启动当前阶段需要的 MySQL/Redis/Kafka
make dev-api            # 在 WSL 直接运行 Gin API
make dev-worker         # 阶段 6 起运行 Worker
make dev-web            # 启动 Vite
make admin-create       # 交互式创建管理员
make migrate-up         # 执行全部未应用 migration
make migrate-down       # 只回退一个版本
make seed               # 写入开发数据
make swagger            # 生成 OpenAPI
make fmt                # Go 与前端格式化
make lint               # vet、lint 和类型检查
make test               # 单元和 Handler/前端测试
make test-integration   # MySQL、Redis、Kafka 集成测试
make build              # 构建前后端
make compose-up         # 完整 Docker 环境
make compose-down
make k8s-up
make k8s-down
```

推荐普通开发模式：

```text
基础设施：Docker Compose
Gin API / Worker：WSL 直接运行
React：WSL 运行 Vite
DataGrip：Windows 连接 localhost
```

直到阶段 8 才要求全部应用在容器中运行。

---

## 14. 范围控制规则

1. 一次只实现当前阶段列出的任务。
2. 验收未完成，不进入下一阶段。
3. 每阶段结束更新 README、设计差异和 Git tag。
4. 新需求只进入 `docs/backlog.md`。
5. 一个模块必须能用一句话说明职责。
6. Gin 不能进入 Service，GORM 不能进入 Handler。
7. MySQL 永远是业务事实来源。
8. Redis 数据必须允许删除和重建。
9. Kafka 不承担文章读取和发布的同步主流程。
10. 不安装下一阶段才使用的依赖。
11. 不创建 `utils`、`common`、`base` 等大杂烩目录。
12. 不因为旧设计已有代码就复制多用户和复杂权限功能。
13. Compose 稳定并完成故障实验后才开始 Kubernetes。
14. 遇到问题按固定方向排查：

```text
Browser
  → Vite/Nginx
  → Gin Middleware/Handler
  → Service
  → GORM/Redis/Kafka Adapter
  → Worker
  → MySQL
```

每次考虑增加技术前先回答：

- 它解决当前哪个已经出现的问题？
- 这个阶段是否已有真实调用者？
- 不引入它的最简单方案是什么？
- 数据的唯一事实来源在哪里？
- 依赖不可用时系统怎样表现？
- 如何测试、监控、升级和删除它？

---

## 15. 工作量估计

| 阶段 | 预计专注时间 |
|---|---:|
| 0 环境与仓库 | 0.5～1 天 |
| 1 Gin/React 骨架 | 1～2 天 |
| 2 MySQL 公开文章 | 2～3 天 |
| 3 管理员认证 | 2 天 |
| 4 管理端写作 | 2～3 天 |
| 5 Redis 缓存 | 1～2 天 |
| 6 Kafka 浏览量 | 2～3 天 |
| 7 质量与 CI | 1～2 天 |
| 8 完整 Compose | 1～2 天 |
| 9 故障演练 | 1～2 天 |
| 10 Kubernetes | 3～5 天 |

如果每天投入约两小时，预计需要 7～10 周。时间不是验收标准，阶段完成条件才是。

---

## 16. v1.0 完成定义

项目完成不是以“技术都安装过”或“页面像博客”为标准，而是你可以独立解释并演示：

1. Viper 如何把默认配置和环境变量转换成强类型 Config。
2. Zap 如何通过 Request ID 定位一次请求。
3. Gin Engine、Middleware、Handler 和 `http.Server` 的关系。
4. Handler、Service、Repository 和 GORM 的边界。
5. golang-migrate 为什么与 GORM Model 分开管理数据库结构。
6. JWT Access Token 和 MySQL Refresh Session 为什么承担不同职责。
7. validator 的格式校验与 Service 业务规则有什么区别。
8. Axios、Zustand、Ant Design、Tailwind 和 ByteMD 各自负责什么。
9. MySQL 为什么是事实来源，Redis 为什么允许清空。
10. 一次文章访问如何产生 Kafka 事件。
11. Worker 如何通过幂等表避免重复统计。
12. Nginx 如何同时提供 React 文件和代理 API。
13. Docker Compose 如何组织完整本地环境并验证故障行为。
14. Kubernetes 如何用 Deployment、StatefulSet、Service、ConfigMap、Secret、Job 和 PVC 运行同一系统。

达到这些标准时，v1.0 已经完成。之后再从 Backlog 中选择一个独立扩展，例如 Transactional Outbox、Redis 限流、Prometheus/Grafana、图片对象存储或文章归档；不要同时展开。
