BIN     := nota
PKG     := github.com/dignajar/nota
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/internal/cli.Version=$(VERSION)
PREFIX  ?= $(HOME)/.local
TARGETS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: build test vet install uninstall cross clean release

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BIN) .

test:
	go test -race ./...

vet:
	gofmt -l . | (! grep .) || (echo "run gofmt -w ." && exit 1)
	go vet ./...

install: build
	install -d $(PREFIX)/bin
	install -m 0755 bin/$(BIN) $(PREFIX)/bin/$(BIN)
	@echo "Installed $(PREFIX)/bin/$(BIN). Next: nota install-service"

uninstall:
	-$(PREFIX)/bin/$(BIN) uninstall-service
	rm -f $(PREFIX)/bin/$(BIN)

cross:
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/$(BIN)-$$os-$$arch . || exit 1; \
	done

# Trigger the Release workflow on main. BUMP=auto|patch|minor|major
release:
	gh workflow run release.yml --ref main -f bump=$(or $(BUMP),auto)
	@echo "Started. Follow it with: gh run watch"

clean:
	rm -rf bin dist
