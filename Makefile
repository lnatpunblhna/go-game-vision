# Go Game Vision Makefile

# Variable definitions
BINARY_NAME=go-game-vision
MAIN_PATH=./main.go
BUILD_DIR=./build
EXAMPLES_DIR=./examples
TESTS_DIR=./tests

# Go related variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
BUILD_FLAGS=-ldflags="-s -w"
WINDOWS_FLAGS=GOOS=windows GOARCH=amd64
DARWIN_FLAGS=GOOS=darwin GOARCH=amd64

.PHONY: all build clean test deps help run examples

# Default target
all: clean deps test build

# Build project
build:
	@echo "Building project..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Cross-platform build
build-windows:
	@echo "Building Windows version..."
	@mkdir -p $(BUILD_DIR)
	$(WINDOWS_FLAGS) $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows.exe $(MAIN_PATH)
	@echo "Windows build complete: $(BUILD_DIR)/$(BINARY_NAME)-windows.exe"

build-darwin:
	@echo "Building macOS version..."
	@mkdir -p $(BUILD_DIR)
	$(DARWIN_FLAGS) $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin $(MAIN_PATH)
	@echo "macOS build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin"

build-all: build-windows build-darwin
	@echo "All platform builds complete"

# Clean build files
clean:
	@echo "Cleaning build files..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# Install dependencies
deps:
	@echo "Installing Go dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download
	@echo "Dependencies installation complete"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v $(TESTS_DIR)/...
	@echo "Tests complete"

# Run tests and generate coverage report
test-coverage:
	@echo "Running tests and generating coverage report..."
	$(GOTEST) -v -coverprofile=coverage.out $(TESTS_DIR)/...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run main program
run:
	@echo "Running main program..."
	$(GOCMD) run $(MAIN_PATH) help

# Run examples
examples:
	@echo "Running basic usage examples..."
	$(GOCMD) run $(EXAMPLES_DIR)/basic_usage.go

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...
	@echo "Code formatting complete"

# Code check
vet:
	@echo "Running code check..."
	$(GOCMD) vet ./...
	@echo "Code check complete"

# Install tools
install-tools:
	@echo "Installing development tools..."
	$(GOGET) -u golang.org/x/tools/cmd/goimports
	$(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Tools installation complete"

# Code quality check
lint:
	@echo "Running code quality check..."
	golangci-lint run
	@echo "Code quality check complete"

# Complete code check process
check: fmt vet lint test
	@echo "All checks complete"

# Install to system
install: build
	@echo "Installing to system..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "Installation complete"

# Uninstall
uninstall:
	@echo "Uninstalling from system..."
	@rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Uninstallation complete"

# Create release package
release: clean build-all
	@echo "Creating release package..."
	@mkdir -p $(BUILD_DIR)/release
	@cp $(BUILD_DIR)/$(BINARY_NAME)-windows.exe $(BUILD_DIR)/release/
	@cp $(BUILD_DIR)/$(BINARY_NAME)-darwin $(BUILD_DIR)/release/
	@cp README.md $(BUILD_DIR)/release/
	@cp -r examples $(BUILD_DIR)/release/
	@cd $(BUILD_DIR) && tar -czf go-game-vision-release.tar.gz release/
	@echo "Release package created: $(BUILD_DIR)/go-game-vision-release.tar.gz"

# Show help information
help:
	@echo "Go Game Vision - Available Make commands:"
	@echo ""
	@echo "Build related:"
	@echo "  build          - Build executable for current platform"
	@echo "  build-windows  - Build Windows version"
	@echo "  build-darwin   - Build macOS version"
	@echo "  build-all      - Build all platform versions"
	@echo ""
	@echo "Development related:"
	@echo "  deps           - Install Go dependencies"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests and generate coverage report"
	@echo "  fmt            - Format code"
	@echo "  vet            - Run code check"
	@echo "  lint           - Run code quality check"
	@echo "  check          - Run all checks"
	@echo ""
	@echo "Runtime related:"
	@echo "  run            - Run main program"
	@echo "  examples       - Run example code"
	@echo ""
	@echo "Tools related:"
	@echo "  install-tools  - Install development tools"
	@echo "  install        - Install to system"
	@echo "  uninstall      - Uninstall from system"
	@echo ""
	@echo "Others:"
	@echo "  clean          - Clean build files"
	@echo "  release        - Create release package"
	@echo "  help           - Show this help information"
