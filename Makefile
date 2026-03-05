BINARY   := rest-mcp
MODULE   := github.com/devstroop/rest-mcp
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: all build run clean test fmt lint dry-run

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY).exe ./cmd/rest-mcp

run: build
	./$(BINARY).exe

clean:
	rm -f $(BINARY).exe

test:
	go test ./... -v -count=1

fmt:
	go fmt ./...
	goimports -w .

lint:
	golangci-lint run ./...

# Print generated tools from example config without starting server
dry-run: build
	BASE_URL=https://httpbin.org ./$(BINARY).exe --config rest-mcp.example.toml --dry-run

tidy:
	go mod tidy
