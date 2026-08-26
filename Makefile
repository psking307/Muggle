.PHONY: dev-api dev-web fmt lint test build

dev-api:
	@set -a; [ ! -f .env ] || . ./.env; set +a; $(MAKE) -C backend dev

dev-web:
	npm --prefix frontend run dev

fmt:
	$(MAKE) -C backend fmt

lint:
	$(MAKE) -C backend lint
	npm --prefix frontend run lint
	npm --prefix frontend run typecheck

test:
	$(MAKE) -C backend test

build:
	$(MAKE) -C backend build
	npm --prefix frontend run build
