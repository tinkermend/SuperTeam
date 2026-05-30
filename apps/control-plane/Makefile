.PHONY: generate generate-sqlc generate-openapi test build clean migrate-up migrate-down migrate-status

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
	rm -f coverage.out
	@echo "Clean complete"

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
