# Muggle Tiny Blog

Muggle 是一个按阶段实现的 Tiny Blog Lab。当前仓库完成了阶段 1：Gin + React 工程骨架。

## 阶段 1 已实现

- Viper 默认配置、环境变量覆盖、强类型解码和 validator 校验。
- Zap 开发 Console 日志与生产 JSON 日志。
- Gin Router、Request ID、访问日志和 panic Recovery。
- `GET /api/v1/health/live` 健康检查。
- 显式 `http.Server` 超时和 SIGINT/SIGTERM 优雅关闭。
- health、404、Recovery 和配置加载测试。
- React + TypeScript + Vite 前端。
- React Router、Ant Design、Tailwind CSS 和 Axios。
- API 正常、加载中和 API 不可达三种页面状态。
- 根目录 Makefile 统一开发、测试、检查和构建命令。

阶段 1 不启动或配置 MySQL、Redis、Kafka、JWT、Zustand 和 ByteMD。

## 环境要求

在 WSL Ubuntu 中安装：

- Go
- Node.js 和 npm
- Git
- Make

项目应位于 WSL Linux 文件系统中，例如：

```text
/home/hejinze/Muggle
```

## 首次准备

```bash
cd /home/hejinze/Muggle
cp .env.example .env
cd frontend
npm install
```

`.env` 不提交到 Git。程序配置优先级为：

```text
代码默认值 < backend/configs/default.yaml < 进程环境变量
```

根 Makefile 的 `dev-api` 会在 `.env` 存在时自动把它加载为进程环境变量。

## 开发命令

在仓库根目录运行：

```bash
make dev-api   # 启动 Gin API，默认监听 :8080
make dev-web   # 启动 Vite，默认监听 :5173
make test      # 运行 Go 单元测试和 Handler 测试
make lint      # 运行 go vet、ESLint 和 TypeScript 类型检查
make fmt       # 格式化 Go 代码
make build     # 构建后端二进制和前端静态文件
```

## 手动验收

打开第一个 WSL 终端：

```bash
cd /home/hejinze/Muggle
make dev-api
```

打开第二个 WSL 终端：

```bash
cd /home/hejinze/Muggle
make dev-web
```

检查 API：

```bash
curl http://localhost:8080/api/v1/health/live
```

期望返回：

```json
{"status":"ok"}
```

浏览器打开 `http://localhost:5173`，应显示“API 运行正常”。停止 API 后点击“重新检查”，页面应显示“API 当前不可达”。

API 日志包含 Request ID、方法、路由、状态码、耗时和错误代码。按 `Ctrl+C` 时，API 会在配置的关闭超时内优雅退出。

## 主要目录

```text
backend/
├── cmd/api/                 # API 程序入口
├── configs/                 # 非敏感默认配置
└── internal/
    ├── app/                 # 依赖组装和 HTTP 生命周期
    ├── config/              # 配置加载与校验
    ├── httpapi/             # Router、health 和 Middleware
    └── platform/logger/     # Zap 技术适配

frontend/src/
├── api/                     # Axios 实例
├── app/                     # React Provider 和 Router
├── features/health/         # API 状态功能
├── pages/                   # 页面
└── styles/                  # 全局样式和 Tailwind
```
