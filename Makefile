.PHONY: help run reindex build tidy sqlc gen-contracts gen-cities up down logs migrate psql

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

run: ## Run the server locally (requires a running Postgres)
	go run ./cmd/server

reindex: ## Reindex jobs into Meilisearch (requires running Postgres + Meilisearch)
	go run ./cmd/reindex

build: ## Build the binary
	go build -o bin/hire ./cmd/server

tidy: ## Tidy up dependencies
	go mod tidy

sqlc: ## Generate code from SQL (via Docker, no local sqlc needed)
	docker run --rm -v "$(PWD):/src" -w /src sqlc/sqlc generate

gen-contracts: ## Regenerate web/src/lib/generated/contracts.ts from Go contracts
	go run ./cmd/gen-contracts

gen-cities: ## Regenerate internal/location/cities15000.tsv from the GeoNames dump
	go run ./cmd/gen-cities

up: ## Start everything in Docker (app + postgres)
	docker compose up --build -d

down: ## Stop and remove containers
	docker compose down

logs: ## Tail application logs
	docker compose logs -f app

migrate: ## Apply migrations manually to a running DB (for an existing volume)
	@for f in migrations/*.sql; do \
		echo "applying $$f"; \
		docker compose exec -T db psql -U hire -d hire -f /docker-entrypoint-initdb.d/$$(basename $$f); \
	done

psql: ## Open psql in the database
	docker compose exec db psql -U hire -d hire
