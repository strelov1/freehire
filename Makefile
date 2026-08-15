.PHONY: help run reindex build tidy sqlc mail-preview gen-contracts gen-cities cv-previews up down logs migrate psql cover cover-html cover-integration

# Prefer Docker; fall back to Podman when `docker` is missing. Override with
# `make DOCKER=podman up` (or COMPOSE=…) when both are installed.
DOCKER ?= $(shell command -v docker >/dev/null 2>&1 && echo docker || echo podman)
COMPOSE ?= $(DOCKER) compose

# Coverage profiles land under coverage/ (gitignored). Informational only — no % gate.
COVERAGE_DIR ?= coverage

COVER_UNIT ?= $(COVERAGE_DIR)/unit.out
COVER_INTEGRATION ?= $(COVERAGE_DIR)/integration.out

# Where `make mail-preview` writes. Committed, because Storybook frames these files
# and a test asserts they still match what the templates render.
MAIL_PREVIEW_DIR ?= design-system/static/email-previews

SQLC_VERSION ?= 1.31.1

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

run: ## Run the server locally (requires a running Postgres)
	go run ./cmd/server

reindex: ## Reindex jobs into Meilisearch (requires running Postgres + Meilisearch)
	go run ./cmd/reindex

build: ## Build the binary
	go build -o bin/hire ./cmd/server

tidy: ## Tidy up dependencies
	go mod tidy

sqlc: ## Generate code from SQL (via Docker/Podman, no local sqlc needed)
	$(DOCKER) run --rm -v "$(PWD):/src" -w /src docker.io/sqlc/sqlc:$(SQLC_VERSION) generate

mail-preview: ## Render every outgoing email and open the contact sheet in a browser
	@go run ./cmd/mail-preview
	@(open $(MAIL_PREVIEW_DIR)/index.html 2>/dev/null || xdg-open $(MAIL_PREVIEW_DIR)/index.html 2>/dev/null) || \
		echo "open $(MAIL_PREVIEW_DIR)/index.html"

gen-contracts: ## Regenerate web/src/lib/generated/contracts.ts from Go contracts
	go run ./cmd/gen-contracts

gen-cities: ## Regenerate internal/location/cities15000.tsv from the GeoNames dump
	go run ./cmd/gen-cities

cv-previews: ## Regenerate web/static/cv-previews/*.svg from the CV templates (needs typst)
	go run ./cmd/cv-previews

up: ## Start everything (app + postgres) via Docker or Podman Compose
	$(COMPOSE) up --build -d

down: ## Stop and remove containers
	$(COMPOSE) down

logs: ## Tail application logs
	$(COMPOSE) logs -f app

migrate: ## Apply migrations manually to a running DB (for an existing volume)
	@for f in migrations/*.sql; do \
		echo "applying $$f"; \
		$(COMPOSE) exec -T db psql -U hire -d hire -f /docker-entrypoint-initdb.d/$$(basename $$f); \
	done

psql: ## Open psql in the database
	$(COMPOSE) exec db psql -U hire -d hire

cover: ## Unit tests with coverage profile → coverage/unit.out
	@mkdir -p $(COVERAGE_DIR)
	go test ./... -coverprofile=$(COVER_UNIT) -covermode=atomic
	@go tool cover -func=$(COVER_UNIT) | tail -1

cover-html: ## HTML report from coverage/unit.out → coverage/unit.html
	@test -f $(COVER_UNIT) || $(MAKE) cover
	go tool cover -html=$(COVER_UNIT) -o $(COVERAGE_DIR)/unit.html
	@echo "wrote $(COVERAGE_DIR)/unit.html"

cover-integration: ## Integration-tagged tests with coverage (needs Docker for testcontainers)
	@mkdir -p $(COVERAGE_DIR)
	go test -tags=integration ./... -coverprofile=$(COVER_INTEGRATION) -covermode=atomic
	@go tool cover -func=$(COVER_INTEGRATION) | tail -1
	@echo "profile: $(COVER_INTEGRATION)"
