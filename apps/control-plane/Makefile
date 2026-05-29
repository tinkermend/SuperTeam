.PHONY: generate test build clean

# Generate code from OpenAPI specs
generate:
	@echo "Generating code from OpenAPI specs..."
	@mkdir -p internal/auth
	oapi-codegen -package auth -generate types,chi-server \
		-o internal/auth/generated.go \
		../../contracts/control-plane/auth.yaml
	@echo "Code generation complete"

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
