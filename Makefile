# Variables
BINARY_NAME=sprezz-server
COVERAGE_FILE=coverage.out

# Include the environment file and export its variables to the shell session
-include .env
export

.PHONY: all tidy sqlc-gen sqlc-check fmt lint test cover clean run build

# Default target runs code generation and verification to guarantee a pristine repository state
all: tidy sqlc-gen fmt lint test

## tidy: Run go mod tidy to add missing and prune unused modules
tidy:
	@echo "=> Optimizing go.mod and go.sum..."
	go mod tidy

## sqlc-gen: Compile SQL schema definitions and annotations into type-safe Go source code
sqlc-gen:
	@echo "=> Compiling query layer using sqlc code generator..."
	@if command -v sqlc > /dev/null; then \
		sqlc generate; \
	else \
		echo "ERROR: sqlc command not found. Install it via 'brew install sqlc' or visit sqlc.dev"; \
		exit 1; \
	fi

## sqlc-check: Validate that generated database files are perfectly synced with queries on disk
sqlc-check:
	@echo "=> Verifying sqlc generation up-to-date state..."
	@if command -v sqlc > /dev/null; then \
		sqlc diff; \
	else \
		echo "ERROR: sqlc command not found. Validation skipped."; \
		exit 1; \
	fi

## fmt: Automatically format all code files according to standard styles
fmt:
	@echo "=> Formatting source tree code..."
	go fmt ./...

## lint: Execute the golangci-lint engine for architectural validations
lint:
	@echo "=> Running golangci-lint audit tree checks..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "WARNING: golangci-lint is not installed. Run 'brew install golangci-lint' or visit golangci-lint.run"; \
		exit 1; \
	fi

## test: Run the entire unit and integration testing harness suite
test:
	@echo "=> Running all package test specifications..."
	go test -v -race ./...

## cover: Generate line-by-line profiling data and launch interactive HTML visual report
cover:
	@echo "=> Capturing cross-package test coverage statistics..."
	go test -coverprofile=$(COVERAGE_FILE) -coverpkg=./... ./...
	@echo "=> Detailed statement summary matrix:"
	go tool cover -func=$(COVERAGE_FILE)
	@echo "=> Opening interactive coverage visualization in your web browser..."
	go tool cover -html=$(COVERAGE_FILE)

## build: Compile the core program binary into a transport target
build: tidy sqlc-gen
	@echo "=> Building system production binary..."
	go build -o $(BINARY_NAME) cmd/server/main.go

## run: Build and launch the multi-tenant application container engine immediately
run: build
	@echo "=> Bootstrapping Sprezz server runtime..."
	./$(BINARY_NAME)

## clean: Evict transient profiling outputs and temporary compiled targets
clean:
	@echo "=> Evicting build targets and coverage profiles..."
	rm -f $(BINARY_NAME)
	rm -f $(COVERAGE_FILE)
