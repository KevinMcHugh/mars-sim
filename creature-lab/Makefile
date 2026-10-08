# Creature Lab: see docs/creature-lab.md. Run from this directory; the Go
# code lives in the mars-sim module (cmd/creature-lab, internal/creaturelab)
# because it imports internal/sim.
.PHONY: dev generate migrate rollback key invite build test help

SQLC   = go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
OAPI   = go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
DBMATE = go run github.com/amacneil/dbmate/v2@v2.28.0 --migrations-dir db/migrations --no-dump-schema

dev: migrate ## Migrate, then run the server on :8080 (needs DATABASE_URL)
	cd .. && go run ./cmd/creature-lab serve

generate: ## Regenerate internal/creaturelab/db (sqlc) and internal/creaturelab/api/gen (oapi-codegen) after any SQL, migration or spec change
	$(SQLC) generate
	$(OAPI) --config oapi-codegen.yaml api/openapi.yaml

migrate: ## Apply pending migrations (needs DATABASE_URL)
	$(DBMATE) up

rollback: ## Roll back the last migration (needs DATABASE_URL)
	$(DBMATE) rollback

key: ## Mint an api key (usage: make key NAME=kev)
	@if [ -z "$(NAME)" ]; then echo "usage: make key NAME=<label>"; exit 1; fi
	cd .. && go run ./cmd/creature-lab keys create --name "$(NAME)"

invite: ## Mint a single-use invite for a newcomer (usage: make invite NOTE=ana)
	cd .. && go run ./cmd/creature-lab invites create --note "$(NOTE)"

build: ## Build the binary to bin/creature-lab
	cd .. && go build -o creature-lab/bin/creature-lab ./cmd/creature-lab

test: ## Run the lab's tests; set CREATURE_LAB_TEST_DATABASE_URL to include the Postgres ones
	cd .. && go test -count=1 ./internal/creaturelab/... && go test -count=1 -run TestRollLabSpecies ./internal/sim

.DEFAULT_GOAL := help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'
