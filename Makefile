BIN      := gvisor-exec
PKG      := ./cmd/gvisor-exec
GOFLAGS  ?= -trimpath
LDFLAGS  ?= -s -w

.PHONY: all build test unit integration vet fmt clean smoke examples help

all: build

build: ## Build the gvisor-exec binary
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG)

test: ## Run all tests
	go test ./...

unit: ## Run unit tests only (skip integration tests that need runsc)
	go test -short ./...

integration: ## Run integration tests (requires runsc on PATH)
	go test -v -run Integration -timeout 5m ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Run gofmt on all Go files
	gofmt -s -w .

smoke: build ## End-to-end smoke test using the built binary
	./$(BIN) -- /bin/uname -a
	./$(BIN) -- /bin/sh -c 'id; echo $$$$; ls /tmp'
	./$(BIN) -- /bin/sh -c 'exit 42'; test $$? -eq 42 && echo 'exit-code propagation: ok'

examples: build ## Run the scripted examples in examples/
	./examples/run-all.sh

clean: ## Remove build artifacts
	rm -f $(BIN)
	rm -rf /tmp/gvisor-exec-*

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
