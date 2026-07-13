BINARY := skillrunner
PKG := ./cmd/skillrunner
PREFIX := /usr/local/bin

.DEFAULT_GOAL := help
.PHONY: help build all test clean install

help: ## Show this help
	@echo "skillrunner — available make targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "} {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build            # build bin/$(BINARY) for this platform"
	@echo "  make all              # cross-compile macOS/Linux/Windows"
	@echo "  ./bin/$(BINARY) detect        # detect the project stack"
	@echo "  ./bin/$(BINARY) emit <skill>  # print marching orders for Claude"

build: ## Build bin/skillrunner for the current platform
	go build -o bin/$(BINARY) $(PKG)

all: ## Cross-compile for macOS (arm64/amd64), Linux, and Windows
	GOOS=darwin  GOARCH=arm64 go build -o bin/$(BINARY)-darwin-arm64 $(PKG)
	GOOS=darwin  GOARCH=amd64 go build -o bin/$(BINARY)-darwin-amd64 $(PKG)
	GOOS=linux   GOARCH=amd64 go build -o bin/$(BINARY)-linux-amd64 $(PKG)
	GOOS=windows GOARCH=amd64 go build -o bin/$(BINARY).exe $(PKG)

test: ## Run go vet and go test
	go vet ./...
	go test ./...

install: build ## Install bin/skillrunner into /usr/local/bin (uses sudo if needed; override PREFIX=)
	@if [ -w "$(PREFIX)" ]; then \
		cp bin/$(BINARY) "$(PREFIX)/$(BINARY)"; \
	else \
		echo "→ $(PREFIX) not writable; using sudo (you may be prompted for your password)"; \
		sudo cp bin/$(BINARY) "$(PREFIX)/$(BINARY)"; \
	fi
	@echo "Installed $(BINARY) to $(PREFIX)/$(BINARY)"

clean: ## Remove the bin/ directory
	rm -rf bin
