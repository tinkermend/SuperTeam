.PHONY: generate generate-sqlc generate-openapi test build clean migrate-up migrate-down migrate-status migrate-validate new-migration

# Throwaway dev DB for migrate-validate (atlas will wipe this database only).
# Default: local container superteam-migrate-validate-pg on :55432, dedicated empty DB.
# CI with real Docker may override: make migrate-validate DEV_URL=docker://postgres/16/dev
ATLAS_VALIDATE_ADMIN_URL ?= postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable
DEV_URL ?= postgres://postgres:postgres@127.0.0.1:55432/atlas_migrate_validate?sslmode=disable

# Generate code from sqlc and OpenAPI specs
generate: generate-sqlc generate-openapi

generate-sqlc:
	@echo "Generating sqlc code..."
	sqlc generate
	@echo "sqlc generation complete"

generate-openapi:
	@echo "Generating code from OpenAPI specs..."
	@mkdir -p internal/auth
	oapi-codegen -package auth -generate types,chi-server \
		-o internal/auth/generated.go \
		../../contracts/control-plane/auth.yaml
	@mkdir -p internal/authzcenter
	oapi-codegen -package authzcenter -generate types,chi-server \
		-o internal/authzcenter/generated.go \
		../../contracts/control-plane/authz.yaml
	@echo "OpenAPI code generation complete"

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	@echo "Tests complete"

# Build the control-plane binary
build:
	@echo "Building control-plane..."
	go build -o bin/control-plane ./cmd/control-plane
	@echo "Build complete"

# Clean generated files and build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f internal/auth/generated.go
	rm -f internal/authzcenter/generated.go
	rm -f coverage.out
	@echo "Clean complete"

# Create a new empty migration with a UTC-timestamp version (updates atlas.sum).
# Timestamp versions avoid the sequential-number collisions that concurrent
# branches hit with hand-picked NNN_ prefixes; legacy 001-088 files sort first
# lexicographically, so mixing is safe. Usage: make new-migration NAME=add_foo_table
new-migration:
	@test -n "$(NAME)" || (echo "ERROR: NAME not set. Usage: make new-migration NAME=snake_case_description" && exit 1)
	atlas migrate new $(NAME) --dir file://internal/storage/migrations

# Database migrations
migrate-up:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL not set" && exit 1)
	atlas migrate apply --dir file://internal/storage/migrations --url "$(DATABASE_URL)" --revisions-schema atlas_schema_revisions

migrate-down:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL not set" && exit 1)
	@test -n "$(STEPS)" || (echo "ERROR: STEPS not set. Usage: make migrate-down STEPS=1" && exit 1)
	atlas migrate down $(STEPS) --dir file://internal/storage/migrations --url "$(DATABASE_URL)" --revisions-schema atlas_schema_revisions

migrate-status:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL not set" && exit 1)
	atlas migrate status --dir file://internal/storage/migrations --url "$(DATABASE_URL)" --revisions-schema atlas_schema_revisions

# Verify atlas.sum integrity and that every migration replays cleanly on a throwaway dev DB.
# Default DEV_URL → superteam-migrate-validate-pg :55432 / atlas_migrate_validate (auto-created).
# Override for CI docker daemon: make migrate-validate DEV_URL=docker://postgres/16/dev
# (atlas migrate lint is intentionally not wired in: v0.38+ gates it behind Atlas Pro login.)
migrate-validate:
	@if echo "$(DEV_URL)" | grep -qE '^postgres://'; then \
		if ! psql "$(ATLAS_VALIDATE_ADMIN_URL)" -tAc "SELECT 1 FROM pg_database WHERE datname='atlas_migrate_validate'" | grep -q 1; then \
			psql "$(ATLAS_VALIDATE_ADMIN_URL)" -v ON_ERROR_STOP=1 -c "CREATE DATABASE atlas_migrate_validate"; \
		fi; \
	fi
	atlas migrate validate --dir file://internal/storage/migrations --dev-url "$(DEV_URL)"
