# Justfile for Go development

# Format Go code
format:
    go fmt ./...

# Build the project
build:
    go build -o bin/server ./cmd/server/main.go

# Run the project
run:
    go run ./cmd/server/main.go

# Vet the code
vet:
    go vet ./...

# Clean build artifacts
clean:
    go clean
    rm -rf bin/

# Tidy modules
tidy:
    go mod tidy
