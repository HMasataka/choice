.PHONY: build test test-coverage lint run clean fmt vet tidy tools

# Variables
BINARY_NAME=sfu
BINARY_DIR=bin
CMD_DIR=cmd/sfu
COVERAGE_FILE=coverage.out

# Build
build:
	@mkdir -p $(BINARY_DIR)
	go build -o $(BINARY_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

# Run
run: build
	./$(BINARY_DIR)/$(BINARY_NAME)

# Test
test:
	go test -v -race ./...

# Test with coverage
test-coverage:
	go test -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	go tool cover -html=$(COVERAGE_FILE) -o coverage.html

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# Format
fmt:
	go fmt ./...
	goimports -w .

# Vet
vet:
	go vet ./...

# Tidy
tidy:
	go mod tidy

# Clean
clean:
	rm -rf $(BINARY_DIR)
	rm -f $(COVERAGE_FILE) coverage.html

# Install development tools
tools:
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# All checks
check: fmt vet lint test
