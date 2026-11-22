BIN       := bin/agentlog
VERSION   ?=
LDFLAGS   := -s -w $(if $(VERSION),-X main.version=$(VERSION))

# `make dist` builds release binaries. plugin.json is the single source of the version.
PLUGIN_VERSION := $(shell sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' plugin/.claude-plugin/plugin.json 2>/dev/null | head -1)
DIST_VERSION   := $(or $(VERSION),$(PLUGIN_VERSION),dev)
DIST_LDFLAGS   := -s -w -X main.version=$(DIST_VERSION)
PLATFORMS      := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64
DIST_DIR       := plugin/scripts/dist

# Playwright is run through node: npm/npx may be unavailable or misconfigured on some machines.
PLAYWRIGHT ?= node e2e/node_modules/@playwright/test/cli.js

.PHONY: build check install dist dist-local validate e2e demo

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/agentlog

check:
	@out="$$(gofmt -l cmd internal web)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	go test ./...
	@if [ -f plugin/scripts/agentlog ]; then sh -n plugin/scripts/agentlog; fi
	@scripts/check-plugin.sh

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/agentlog

# All five platforms, for the release (binaries are gitignored on master).
dist:
	@mkdir -p $(DIST_DIR)
	@for p in $(PLATFORMS); do os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
	  echo "build $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(DIST_LDFLAGS)" \
	    -o $(DIST_DIR)/agentlog-$$os-$$arch$$ext ./cmd/agentlog || exit 1; done

# Only this machine's platform, for development.
dist-local:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(DIST_LDFLAGS)" \
	  -o $(DIST_DIR)/agentlog-$$(go env GOOS)-$$(go env GOARCH) ./cmd/agentlog

# Needs the `claude` CLI. Validates the plugin (strict) and the marketplace file.
validate:
	@scripts/check-plugin.sh
	claude plugin validate --strict ./plugin
	claude plugin validate .

e2e: build
	$(PLAYWRIGHT) test --config e2e/playwright.config.ts

# A realistic demo repo to try the UI: bin/agentlog ui /tmp/agentlog-demo
demo: build
	scripts/make-demo-repo.sh /tmp/agentlog-demo
	@echo "demo repo ready. Try: $(BIN) ui /tmp/agentlog-demo"
