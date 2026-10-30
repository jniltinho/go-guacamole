# GoGuacamole — Go (Cobra / Echo v5) + Vue 3 (Vite+ / Tailwind v4) gateway for Apache guacd.
#
# `make build` builds the frontend into web/dist and then the Go binary embedding it.

# bash with pipefail: a failing command piped into grep (go test | grep) must fail the target (dash has no pipefail)
SHELL       := /bin/bash
.SHELLFLAGS := -eo pipefail -c

BINARY      := goguacamole
BIN_DIR     := bin
FRONTEND    := frontend
MODULE      := github.com/jniltinho/go-guacamole
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
	-X $(MODULE)/cmd.Version=$(VERSION) \
	-X $(MODULE)/cmd.GitCommit=$(GIT_COMMIT) \
	-X $(MODULE)/cmd.BuildDate=$(BUILD_DATE)

IMAGE       ?= ghcr.io/jniltinho/go-guacamole
PORT        ?= 8080
GUACD       ?= 127.0.0.1:4822
VP          := cd $(FRONTEND) && npx vp
COMPOSE     := docker compose
E2E_PASS    ?= Admin-E2E\#2026

.DEFAULT_GOAL := help
.PHONY: help deps build frontend backend run dev dev-backend dev-frontend test test-backend test-frontend \
	test-db lint fmt typecheck certs docker-build docker-allinone docker-binary docker-test docker-push up up-mariadb up-postgres up-lab up-tls down \
	logs ps migrate-up migrate-down e2e e2e-api e2e-browser screenshots clean

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## ---------------------------------------------------------------- build

deps: ## Download Go modules and install frontend dependencies
	go mod download
	cd $(FRONTEND) && npm ci --no-audit --no-fund

$(FRONTEND)/node_modules: $(FRONTEND)/package-lock.json
	cd $(FRONTEND) && npm ci --no-audit --no-fund
	@touch $@

frontend: $(FRONTEND)/node_modules ## Check, type-check and build the frontend into web/dist
	$(VP) check
	cd $(FRONTEND) && npm run typecheck
	$(VP) build
	@touch web/dist/.gitkeep

backend: ## Build the Go binary (expects web/dist)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) .

build: frontend ## Build frontend and Go binary (bin/goguacamole)
	$(MAKE) backend

## ---------------------------------------------------------------- run

run: build ## Build and run the server on $(PORT) (guacd at $(GUACD))
	GUACD_ADDR=$(GUACD) ./$(BIN_DIR)/$(BINARY) serve --addr :$(PORT)

dev: ## Print how to run backend + frontend in dev mode
	@echo "Terminal 1: make dev-backend   (Go API on :$(PORT))"
	@echo "Terminal 2: make dev-frontend  (Vite+ on :9000, proxies /api to :$(PORT))"

dev-backend: ## Run the Go server with go run (guacd at $(GUACD))
	GUACD_ADDR=$(GUACD) APP_LOG_LEVEL=debug go run . serve --addr :$(PORT)

dev-frontend: $(FRONTEND)/node_modules ## Run the Vite+ dev server on :9000 proxying /api to :$(PORT)
	cd $(FRONTEND) && GG_BACKEND=http://localhost:$(PORT) npx vp dev

## ---------------------------------------------------------------- quality

test: test-backend test-frontend ## Run Go and frontend unit tests

test-backend: ## Run Go tests (SQLite; set TEST_MYSQL_DSN / TEST_POSTGRES_DSN for the other drivers)
	go test ./... -count=1

test-frontend: $(FRONTEND)/node_modules ## Run frontend unit tests (Vitest via Vite+)
	$(VP) test run

test-db: ## Run the repository tests against throwaway MariaDB and PostgreSQL containers
	@docker rm -f gg-test-mariadb gg-test-pg >/dev/null 2>&1 || true
	docker run -d --rm --name gg-test-mariadb -e MARIADB_ROOT_PASSWORD=root -e MARIADB_DATABASE=gg -p 127.0.0.1:13306:3306 mariadb:11.4
	docker run -d --rm --name gg-test-pg -e POSTGRES_PASSWORD=pg -e POSTGRES_DB=gg -p 127.0.0.1:15432:5432 postgres:17-alpine
	@for i in $$(seq 1 60); do docker exec gg-test-mariadb healthcheck.sh --connect >/dev/null 2>&1 && docker exec gg-test-pg pg_isready -U postgres >/dev/null 2>&1 && break; sleep 2; done
	TEST_MYSQL_DSN='root:root@tcp(127.0.0.1:13306)/gg' TEST_POSTGRES_DSN='postgres://postgres:pg@127.0.0.1:15432/gg?sslmode=disable' \
		go test ./internal/store/... ./internal/access/... ./internal/importer/... -count=1 -v | grep -E '^(---|ok|FAIL|PASS)'
	docker rm -f gg-test-mariadb gg-test-pg >/dev/null

lint: ## go vet + gofmt check + vp check (oxlint/oxfmt)
	go vet ./...
	@test -z "$$(gofmt -l $$(git ls-files '*.go' 2>/dev/null || find . -name '*.go' -not -path './frontend/*'))" || (gofmt -l .; exit 1)
	$(VP) check

typecheck: $(FRONTEND)/node_modules ## Type-check the frontend (vue-tsc)
	cd $(FRONTEND) && npm run typecheck

fmt: ## Format Go and frontend code
	gofmt -w $$(find . -name '*.go' -not -path './frontend/*' -not -path './base/*')
	$(VP) fmt

## ---------------------------------------------------------------- docker

certs: ## Generate a local CA and server certificate in deploy/certs (for TLS tests)
	docker run --rm -v $(CURDIR)/deploy/certs:/certs -w /certs alpine/openssl:latest sh -c '\
		openssl req -x509 -newkey rsa:3072 -nodes -days 825 -keyout ca.key -out ca.crt -subj "/CN=GoGuacamole Local CA" && \
		openssl req -newkey rsa:2048 -nodes -keyout server.key -out server.csr -subj "/CN=localhost" && \
		printf "subjectAltName=DNS:localhost,DNS:app,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n" > ext.cnf && \
		openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 825 -out server.crt -extfile ext.cnf && \
		rm -f server.csr ext.cnf ca.srl && chmod 644 server.key'
	@ls -l deploy/certs

docker-build: ## Build the container image $(IMAGE):$(VERSION)
	docker build --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .
	@docker image ls $(IMAGE):$(VERSION) --format '{{.Repository}}:{{.Tag}}  {{.Size}}'

docker-allinone: ## Build the all-in-one image (guacd + goguacamole + Redis) $(IMAGE):$(VERSION)-allinone
	docker build --target allinone --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(IMAGE):$(VERSION)-allinone -t $(IMAGE):allinone .
	@docker image ls $(IMAGE):$(VERSION)-allinone --format '{{.Repository}}:{{.Tag}}  {{.Size}}'

docker-binary: ## Build bin/goguacamole inside Docker (lint + tests included; no local Go or Node.js)
	docker build --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) --target binary --output type=local,dest=$(BIN_DIR) .
	@ls -l $(BIN_DIR)/$(BINARY)

docker-test: ## Run frontend checks/tests and go vet/test inside Docker (same gates as the image build)
	docker build --target backend .

docker-push: ## Push $(IMAGE):$(VERSION) and :latest to GHCR (CI does this automatically)
	docker push $(IMAGE):$(VERSION)
	docker push $(IMAGE):latest

up: ## Start app + guacd (SQLite) on :8080
	TAG=latest $(COMPOSE) up -d --build
	@echo "→ http://localhost:8080"

up-mariadb: ## Start app + guacd + MariaDB
	DB_DRIVER=mysql DB_DSN='goguacamole:goguacamole@tcp(mariadb:3306)/goguacamole' $(COMPOSE) --profile mariadb up -d --build

up-postgres: ## Start app + guacd + PostgreSQL
	DB_DRIVER=postgres DB_DSN='postgres://goguacamole:goguacamole@postgres:5432/goguacamole?sslmode=disable' $(COMPOSE) --profile postgres up -d --build

up-lab: ## Start app + guacd + SSH/VNC/RDP lab targets
	$(COMPOSE) --profile lab up -d --build

up-tls: ## Start app with native HTTPS on :8443 (run `make certs` first)
	TLS_ENABLED=true TLS_CERT_FILE=/certs/server.crt TLS_KEY_FILE=/certs/server.key APP_ADDR=:8443 \
		APP_BASE_URL=https://localhost:8443 $(COMPOSE) up -d --build
	@echo "→ https://localhost:8443"

down: ## Stop every compose service (all profiles)
	$(COMPOSE) --profile mariadb --profile postgres --profile lab down

logs: ## Follow the app and guacd logs
	$(COMPOSE) logs -f app guacd

ps: ## Show compose services
	$(COMPOSE) --profile mariadb --profile postgres --profile lab ps

migrate-up: ## Apply pending migrations to the configured database
	go run . migrate up

migrate-down: ## Roll back the last migration on the configured database
	go run . migrate down

## ---------------------------------------------------------------- e2e

e2e: ## Full e2e: API suite on SQLite, MariaDB and PostgreSQL + browser suite, against the lab stack
	./deploy/e2e.sh all

e2e-api: ## API/tunnel e2e on SQLite only (quick)
	./deploy/e2e.sh sqlite

e2e-browser: ## Playwright browser suite against the lab stack (SQLite)
	./deploy/e2e.sh browser

screenshots: ## README screenshots (docs/screenshots) on a clean lab stack with demo data
	./deploy/e2e.sh screenshots

clean: ## Remove build outputs
	rm -rf $(BIN_DIR) dist
	find web/dist -mindepth 1 ! -name .gitkeep -delete 2>/dev/null || true
