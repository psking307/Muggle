.PHONY: dev-infra dev-api dev-web admin-create migrate-up migrate-down seed swagger fmt lint test test-integration build compose-down

COMPOSE := docker compose --env-file .env -f deploy/compose.yaml
GO ?= /usr/local/go/bin/go
SWAG ?= $(shell $(GO) env GOPATH)/bin/swag

dev-infra:
	$(COMPOSE) --profile dev up -d mysql redis

dev-api:
	@set -a; [ ! -f .env ] || . ./.env; set +a; $(MAKE) -C backend dev

dev-web:
	npm --prefix frontend run dev

migrate-up:
	$(COMPOSE) --profile tools run --rm migrate up

migrate-down:
	$(COMPOSE) --profile tools run --rm migrate down 1

seed:
	# 容器内的 mysql 客户端默认 locale 是 latin1，会双重编码 UTF-8 内容；
	# 必须显式指定 utf8mb4，否则中文 seed 会变成乱码。
	$(COMPOSE) --profile dev exec -T mysql sh -c 'mysql --default-character-set=utf8mb4 -u"$$MYSQL_USER" -p"$$MYSQL_PASSWORD" "$$MYSQL_DATABASE"' < backend/seeds/dev.sql

# 离线创建初始管理员（阶段三）。交互式输入用户名和密码，密码不回显。
admin-create:
	@set -a; [ ! -f .env ] || . ./.env; set +a; cd backend && $(GO) run ./cmd/admin

# swag 会在生成过程中调用 go list；把项目指定的 Go 所在目录临时加入 PATH，
# 可兼容 Go 未安装到系统默认 PATH、但通过 GO=/path/to/go 显式指定的开发环境。
# 只扫描实际包含接口或 DTO 的 Go 包，避免对 backend/internal 空目录执行 go list。
swagger:
	cd backend && PATH="$(dir $(GO)):$$PATH" "$(SWAG)" init -g main.go --dir cmd/api,internal/httpapi,internal/httpapi/middleware,internal/httpapi/response,internal/post,internal/admin --parseInternal --output docs

fmt:
	$(MAKE) -C backend fmt

lint:
	$(MAKE) -C backend lint
	npm --prefix frontend run lint
	npm --prefix frontend run typecheck

test:
	$(MAKE) -C backend test
	npm --prefix frontend run test

test-integration:
	@set -a; . ./.env; set +a; cd backend && MYSQL_TEST_DSN="$${MYSQL_USER}:$${MYSQL_PASSWORD}@tcp($${MYSQL_HOST}:$${MYSQL_PORT})/$${MYSQL_DATABASE}?charset=utf8mb4&parseTime=true&loc=UTC" REDIS_TEST_ADDR="$${REDIS_ADDR}" $(GO) test -tags=integration ./internal/post ./internal/admin -v

build:
	$(MAKE) -C backend build
	npm --prefix frontend run build

compose-down:
	$(COMPOSE) --profile dev --profile tools down
