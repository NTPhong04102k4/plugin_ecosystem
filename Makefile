BINARY := skillrunner
SHIM := sr
PKG := ./cmd/skillrunner
PREFIX := /usr/local/bin

# --------------------------------------------------------------------------
# Platform shim.
#
# cmd.exe ships no sh/grep/awk/rm/sudo and has its own echo rules, so every
# shell-shaped recipe below is written twice: the POSIX half is unchanged, the
# Windows half is cmd-native. GNU make sets OS=Windows_NT on Windows (including
# under Git Bash), so the shell is pinned here too — that way a stray sh.exe on
# PATH cannot make half of a Windows build run POSIX recipes.
#
# The help text is the one thing duplicated by hand: cmd cannot run the
# grep/awk introspection over the `## ` comments. Edit both when adding a target.
# --------------------------------------------------------------------------
ifeq ($(OS),Windows_NT)
SHELL := cmd.exe
.SHELLFLAGS := /C
WINDOWS := 1
EXE := .exe
BIN := bin\$(BINARY).exe
# cmd starts on a legacy code page (437/850 — cp1258 on VN installs), which
# renders UTF-8 output as mojibake: an em dash arrives as ΓÇö. 65001 is UTF-8,
# and chcp sets it for the whole console window, not just this recipe.
UTF8 := chcp 65001 >nul
else
EXE :=
BIN := ./bin/$(BINARY)
UTF8 := true
endif

.DEFAULT_GOAL := help
.PHONY: help build all test clean install

help: ## Show this help
ifdef WINDOWS
	@$(UTF8)
	@echo skillrunner — available make targets:
	@echo.
	@echo   build      Build $(BIN) for this platform
	@echo   all        Cross-compile macOS arm64/amd64, Linux amd64, Windows amd64
	@echo   test       go vet ./... and go test ./...
	@echo   install    go install $(BINARY)$(EXE) + $(SHIM) shim into GOBIN, and record this dir as the pool
	@echo   clean      Remove the bin directory
	@echo.
	@echo Examples:
	@echo   make build                      build $(BIN)
	@echo   $(SHIM) home       show which skill.json resolves, and why
	@echo   $(BIN) detect      detect the project stack
	@echo   $(BIN) emit SKILL  print marching orders for Claude
	@echo   $(BIN) emit all    print orders for every skill, the catalog
	@echo   $(BIN) apply-base  copy the stack base configs into the project
	@echo.
	@echo There is no `make list` — that is a skillrunner subcommand: $(BIN) list
else
	@echo "skillrunner — available make targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "} {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build               # build bin/$(BINARY) for this platform"
	@echo "  make all                 # cross-compile macOS/Linux/Windows"
	@echo "  ./bin/$(BINARY) detect           # detect the project stack"
	@echo "  ./bin/$(BINARY) emit <skill>     # print marching orders for Claude"
	@echo "  ./bin/$(BINARY) emit all         # print orders for every skill (catalog)"
	@echo "  ./bin/$(BINARY) apply-base       # copy the stack's base configs (eslint/linter/...) into the project"
endif

build: ## Build bin/skillrunner for the current platform
	@$(UTF8)
	go build -o bin/$(BINARY)$(EXE) $(PKG)

all: ## Cross-compile for macOS (arm64/amd64), Linux, and Windows
ifdef WINDOWS
	@$(UTF8)
	set GOOS=darwin&& set GOARCH=arm64&& go build -o bin/$(BINARY)-darwin-arm64 $(PKG)
	set GOOS=darwin&& set GOARCH=amd64&& go build -o bin/$(BINARY)-darwin-amd64 $(PKG)
	set GOOS=linux&& set GOARCH=amd64&& go build -o bin/$(BINARY)-linux-amd64 $(PKG)
	set GOOS=windows&& set GOARCH=amd64&& go build -o bin/$(BINARY).exe $(PKG)
else
	GOOS=darwin  GOARCH=arm64 go build -o bin/$(BINARY)-darwin-arm64 $(PKG)
	GOOS=darwin  GOARCH=amd64 go build -o bin/$(BINARY)-darwin-amd64 $(PKG)
	GOOS=linux   GOARCH=amd64 go build -o bin/$(BINARY)-linux-amd64 $(PKG)
	GOOS=windows GOARCH=amd64 go build -o bin/$(BINARY).exe $(PKG)
endif

test: ## Run go vet and go test
	@$(UTF8)
	go vet ./...
	go test ./...

install: build ## Install skillrunner + the sr shim, and record this dir as the skill pool
# Installing is three steps, not one. The binary alone is not a working setup:
# without the pointer file it cannot find this skill.json from any other project,
# and without the shims `sr` does not exist. Doing all three here is what kills
# the old "re-copy sr.exe by hand after every install" footgun.
#
# `home --set` writes into $HOME and must NEVER run under sudo — that would
# record the pool in root's home, where no normal invocation will ever look.
ifdef WINDOWS
	go install $(PKG)
	$(BINARY) home --set "$(CURDIR)"
	$(BINARY) home --shims
	@echo.
	@echo Installed $(BINARY)$(EXE) + $(SHIM) into GOBIN — usually %USERPROFILE%\go\bin, already on PATH.
	@echo Open a new terminal, then: $(SHIM) home
else
	@if [ -w "$(PREFIX)" ]; then \
		cp bin/$(BINARY) "$(PREFIX)/$(BINARY)"; \
		"$(PREFIX)/$(BINARY)" home --shims; \
	else \
		echo "→ $(PREFIX) not writable; using sudo (you may be prompted for your password)"; \
		sudo cp bin/$(BINARY) "$(PREFIX)/$(BINARY)"; \
		sudo "$(PREFIX)/$(BINARY)" home --shims; \
	fi
	@"$(PREFIX)/$(BINARY)" home --set "$(CURDIR)"
	@echo "Installed $(BINARY) + $(SHIM) to $(PREFIX)/"
endif

clean: ## Remove the bin/ directory
ifdef WINDOWS
	@if exist bin rmdir /s /q bin
else
	rm -rf bin
endif
