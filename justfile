# Tide Editor - justfile

# Default recipe: build the binary
default: build

# Module path from go.mod
module := "github.com/bethropolis/tide"

# Binary name
bin_name := "tide"

# Install prefix (change to /usr/local for system-wide)
prefix := env("PREFIX", env("HOME", "/home") + "/.local")

# Build the tide binary
build:
    go build -o ./bin/{{ bin_name }} ./cmd/tide

# Install to $PREFIX/bin
install: build
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{ prefix }}/bin
    cp ./bin/{{ bin_name }} {{ prefix }}/bin/
    echo "Installed {{ bin_name }} to {{ prefix }}/bin/"

# Uninstall from $PREFIX/bin
uninstall:
    #!/usr/bin/env bash
    set -euo pipefail
    rm -f {{ prefix }}/bin/{{ bin_name }}
    echo "Removed {{ prefix }}/bin/{{ bin_name }}"

# Run all tests
test:
    go test ./...

# Run tests with verbose output
test-verbose:
    go test -v ./...

# Run tests with race detection
test-race:
    go test -race ./...

# Run vet checks
vet:
    go vet ./...

# Run all checks (build + vet + test)
check: build vet test

# Clean build artifacts
clean:
    rm -rf ./bin/

# Format all Go source files
fmt:
    gofmt -w -s .

# Check formatting without modifying files
fmt-check:
    test -z "$(gofmt -l -s .)"

# Tidy go.mod and go.sum
tidy:
    go mod tidy

# Run the editor (use ARGS='file.txt' to open a file)
run *args: build
    ./bin/{{ bin_name }} {{ args }}

# Development mode: build and run with example file
dev: build
    ./bin/{{ bin_name }} README.md

# Generate code coverage report
coverage:
    #!/usr/bin/env bash
    set -euo pipefail
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    echo "Coverage report written to coverage.html"

# Show help (default)
help:
    @just --list

# Lint with golangci-lint (if installed)
lint:
    golangci-lint run ./...
