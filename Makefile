.PHONY: all build clean test lint install uninstall release snapshot help

BINARY_NAME=ttv
INSTALL_PATH=/usr/local/bin
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags="-s -w -X main.version=$(VERSION)"

all: build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/ttv

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -rf dist/

## test: Run tests
test:
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

## lint: Run linters
lint:
	@echo "Running linters..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install: https://golangci-lint.run/usage/install/" && exit 1)
	@golangci-lint run

## install: Install the binary to system
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@sudo cp $(BINARY_NAME) $(INSTALL_PATH)/
	@echo "Installed successfully"

## uninstall: Remove the binary from system
uninstall:
	@echo "Uninstalling $(BINARY_NAME) from $(INSTALL_PATH)..."
	@sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Uninstalled successfully"

## release: Create a new release for the tag at HEAD (requires goreleaser)
release:
	@which goreleaser > /dev/null || (echo "goreleaser not found. Install: https://goreleaser.com/install/" && exit 1)
	@echo "Creating release..."
	@scripts/release-notes.sh "$$(git describe --tags --exact-match)" > release-notes.md
	@goreleaser release --clean --release-notes release-notes.md

## snapshot: Create a snapshot release (local testing)
snapshot:
	@which goreleaser > /dev/null || (echo "goreleaser not found. Install: https://goreleaser.com/install/" && exit 1)
	@echo "Creating snapshot..."
	@goreleaser release --snapshot --clean

## run: Build and run with sample data
run: build
	@./$(BINARY_NAME) --help

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

## check: Run tests and linters
check: test lint

## version: Update the fallback version in cmd/ttv/main.go (usage: make version VERSION=x.y.z)
version:
	@echo "Updating version to $(VERSION)..."
	@sed -i.bak 's/^var version = ".*"/var version = "$(VERSION)"/' cmd/ttv/main.go && rm -f cmd/ttv/main.go.bak

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
