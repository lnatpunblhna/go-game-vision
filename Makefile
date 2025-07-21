# Go Game Vision Makefile

# Detect operating system
ifeq ($(OS),Windows_NT)
    DETECTED_OS := Windows
    EXE_EXT := .exe
    RM := del /Q
    RMDIR := rmdir /S /Q
    MKDIR := mkdir
    COPY := copy
    XCOPY := xcopy /E /I /Y
    PATH_SEP := \\
    INSTALL_DIR := $(USERPROFILE)\\bin
    ARCHIVE_CMD := powershell Compress-Archive -Path
    ARCHIVE_EXT := .zip
else
    DETECTED_OS := $(shell uname -s)
    EXE_EXT :=
    RM := rm -f
    RMDIR := rm -rf
    MKDIR := mkdir -p
    COPY := cp
    XCOPY := cp -r
    PATH_SEP := /
    ifeq ($(DETECTED_OS),Darwin)
        INSTALL_DIR := /usr/local/bin
    else
        INSTALL_DIR := /usr/local/bin
    endif
    ARCHIVE_CMD := tar -czf
    ARCHIVE_EXT := .tar.gz
endif

# Variable definitions
BINARY_NAME=go-game-vision
MAIN_PATH=./main.go
BUILD_DIR=./build
TESTS_DIR=./tests

# Go related variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
BUILD_FLAGS=-ldflags "-s -w"
WINDOWS_FLAGS=GOOS=windows GOARCH=amd64
DARWIN_FLAGS=GOOS=darwin GOARCH=amd64
LINUX_FLAGS=GOOS=linux GOARCH=amd64

# Platform-specific binary names
BINARY_LOCAL=$(BINARY_NAME)$(EXE_EXT)
BINARY_WINDOWS=$(BINARY_NAME)-windows.exe
BINARY_DARWIN=$(BINARY_NAME)-darwin
BINARY_LINUX=$(BINARY_NAME)-linux

.PHONY: all build clean test deps help run build-windows build-darwin build-linux build-all release

# Default target
all: clean deps test build

# Build project for current platform (CGO disabled)
build:
	@echo "Building project for $(DETECTED_OS)"
ifeq ($(OS),Windows_NT)
	@if not exist $(BUILD_DIR) $(MKDIR) $(BUILD_DIR)
	@powershell -Command "$(GOBUILD) -ldflags \"-s -w\" -o $(BUILD_DIR)/$(BINARY_LOCAL) $(MAIN_PATH)"
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_LOCAL)"
else
	@$(MKDIR) $(BUILD_DIR)
	$(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_LOCAL) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_LOCAL)"
endif

# Cross-platform builds
build-windows:
	@echo "Building Windows version"
ifeq ($(OS),Windows_NT)
	@if not exist $(BUILD_DIR) $(MKDIR) $(BUILD_DIR)
	@powershell -Command "$$env:GOOS='windows'; $$env:GOARCH='amd64';$(GOBUILD) -ldflags \"-s -w\" -o $(BUILD_DIR)/$(BINARY_WINDOWS) $(MAIN_PATH)"
else
	@$(MKDIR) $(BUILD_DIR)
	$(WINDOWS_FLAGS) CGO_ENABLED=0 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_WINDOWS) $(MAIN_PATH)
endif
	@echo "Windows build complete: $(BUILD_DIR)/$(BINARY_WINDOWS)"

build-darwin:
	@echo "Building macOS version"
ifeq ($(OS),Windows_NT)
	@if not exist $(BUILD_DIR) $(MKDIR) $(BUILD_DIR)
	@powershell -Command "$$env:GOOS='darwin'; $$env:GOARCH='amd64';$(GOBUILD) -ldflags \"-s -w\" -o $(BUILD_DIR)/$(BINARY_DARWIN) $(MAIN_PATH)"
else
	@$(MKDIR) $(BUILD_DIR)
	$(DARWIN_FLAGS) CGO_ENABLED=0 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_DARWIN) $(MAIN_PATH)
endif
	@echo "macOS build complete: $(BUILD_DIR)/$(BINARY_DARWIN)"

build-linux:
	@echo "Building Linux version"
ifeq ($(OS),Windows_NT)
	@if not exist $(BUILD_DIR) $(MKDIR) $(BUILD_DIR)
	@powershell -Command "$$env:GOOS='linux'; $$env:GOARCH='amd64'; $(GOBUILD) -ldflags \"-s -w\" -o $(BUILD_DIR)/$(BINARY_LINUX) $(MAIN_PATH)"
else
	@$(MKDIR) $(BUILD_DIR)
	$(LINUX_FLAGS) CGO_ENABLED=0 $(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_LINUX) $(MAIN_PATH)
endif
	@echo "Linux build complete: $(BUILD_DIR)/$(BINARY_LINUX)"

build-all: build-windows build-darwin build-linux
	@echo "All platform builds complete"

# Clean build files
clean:
	@echo "Cleaning build files..."
	$(GOCLEAN)
ifeq ($(OS),Windows_NT)
	@if exist $(BUILD_DIR) $(RMDIR) $(BUILD_DIR) 2>nul || echo "Build directory already clean"
	@if exist coverage.out $(RM) coverage.out 2>nul || echo ""
	@if exist coverage.html $(RM) coverage.html 2>nul || echo ""
else
	@$(RMDIR) $(BUILD_DIR) 2>/dev/null || echo "Build directory already clean"
	@$(RM) coverage.out coverage.html 2>/dev/null || true
endif
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

# Run main program
run:
	@echo "Running main program..."
	$(GOCMD) run $(MAIN_PATH) help

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

# Complete code check process
check: fmt vet test
	@echo "All checks complete"

# Create release package
release: clean build-all
	@echo "Creating release package..."
ifeq ($(OS),Windows_NT)
	@if not exist $(BUILD_DIR)\release $(MKDIR) $(BUILD_DIR)\release
	@$(COPY) $(BUILD_DIR)\$(BINARY_WINDOWS) $(BUILD_DIR)\release\
	@$(COPY) $(BUILD_DIR)\$(BINARY_DARWIN) $(BUILD_DIR)\release\
	@$(COPY) $(BUILD_DIR)\$(BINARY_LINUX) $(BUILD_DIR)\release\
	@$(COPY) README.md $(BUILD_DIR)\release\
	@$(XCOPY) $(EXAMPLES_DIR) $(BUILD_DIR)\release\examples
	@cd $(BUILD_DIR) && $(ARCHIVE_CMD) go-game-vision-release$(ARCHIVE_EXT) -DestinationPath . -Force
else
	@$(MKDIR) $(BUILD_DIR)/release
	@$(COPY) $(BUILD_DIR)/$(BINARY_WINDOWS) $(BUILD_DIR)/release/
	@$(COPY) $(BUILD_DIR)/$(BINARY_DARWIN) $(BUILD_DIR)/release/
	@$(COPY) $(BUILD_DIR)/$(BINARY_LINUX) $(BUILD_DIR)/release/
	@$(COPY) README.md $(BUILD_DIR)/release/
	@$(XCOPY) $(EXAMPLES_DIR) $(BUILD_DIR)/release/examples
	@cd $(BUILD_DIR) && $(ARCHIVE_CMD) go-game-vision-release$(ARCHIVE_EXT) release/
endif
	@echo "Release package created: $(BUILD_DIR)/go-game-vision-release$(ARCHIVE_EXT)"

# Show help information
help:
	@echo "Go Game Vision - Available Make commands:"
	@echo ""
	@echo "Build related:"
	@echo "  build          - Build executable for current platform ($(DETECTED_OS))"
	@echo "  build-windows  - Build Windows version"
	@echo "  build-darwin   - Build macOS version"
	@echo "  build-linux    - Build Linux version"
	@echo "  build-all      - Build all platform versions"
	@echo ""
	@echo "Development related:"
	@echo "  deps           - Install Go dependencies"
	@echo "  test           - Run tests"
	@echo "  fmt            - Format code"
	@echo "  vet            - Run code check"
	@echo "  lint           - Run code quality check"
	@echo "  check          - Run all checks"
	@echo ""
	@echo "Runtime related:"
	@echo "  run            - Run main program"
	@echo ""
	@echo "Others:"
	@echo "  clean          - Clean build files"
	@echo "  release        - Create release package"
	@echo "  help           - Show this help information"