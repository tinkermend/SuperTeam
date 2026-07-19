.PHONY: generate generate-sqlc generate-openapi test build clean migrate-up migrate-down migrate-status migrate-validate new-migration

# Throwaway dev DB used by migrate-validate to replay migrations. Standard CI has a docker
# daemon; override for local podman etc: make migrate-validate DEV_URL=postgres://...:5432/dev?sslmode=disable
DEV_URL ?= docker://postgres/16/dev

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
# Touches no real DB; safe to run in CI as a schema-drift/checksum gate.
# (atlas migrate lint is intentionally not wired in: v0.38+ gates it behind Atlas Pro login.)
migrate-validate:
	atlas migrate validate --dir file://internal/storage/migrations --dev-url "$(DEV_URL)"
