BIN       := bin/agentlog
VERSION   ?=
LDFLAGS   := -s -w $(if $(VERSION),-X main.version=$(VERSION))

.PHONY: build check install

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/agentlog

check:
	@out="$$(gofmt -l cmd internal web)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	go test ./...
	@if [ -f plugin/scripts/agentlog ]; then sh -n plugin/scripts/agentlog; fi

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/agentlog
