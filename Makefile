.PHONY: all build build-web build-go run clean test lint lint-go lint-web

# Default target - builds both web and go (web first)
all: build

# Build everything - web must complete before go
# Use 'make -j2 build' for parallel execution (web and go build concurrently,
# but go waits for web to finish due to dependency)
build: build-web build-go

# Build web assets (must complete first)
build-web:
	@echo "Building web assets..."
	cd web && npm run build

# Build Go binary (depends on web build)
build-go:
	@echo "Building Go binary..."
	go build -o bin/oda .

# Build and run (web → go → run)
run: build
	@echo "Starting oda..."
	./bin/oda

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	cd web && rm -rf dist/

# Run tests
test:
	@echo "Running Go tests..."
	go test -race ./...
	@echo "Running web lint..."
	cd web && npm run lint

# Run linters (can run in parallel with: make -j2 lint)
lint: lint-go lint-web

lint-go:
	@echo "Running Go linter..."
	golangci-lint run ./...

lint-web:
	@echo "Running web linter..."
	cd web && npm run lint

# Development mode - run both dev servers
dev:
	@echo "Starting development servers..."
	cd web && npm run dev &
	go run .

# Install dependencies
install:
	@echo "Installing Go dependencies..."
	go mod download
	@echo "Installing web dependencies..."
	cd web && npm install
