BIN       ?= vm-info
PKG       ?= github.com/dragonsecurity/vm-info

PREFIX    ?= /usr/local
BINDIR    ?= $(PREFIX)/bin
DESTDIR   ?=

DIST      ?= dist
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

VERSION_PKG ?= $(PKG)/cmd
LDFLAGS   ?= -s -w \
  -X $(VERSION_PKG).Version=$(VERSION) \
  -X $(VERSION_PKG).Commit=$(COMMIT) \
  -X $(VERSION_PKG).Date=$(DATE)
GOFLAGS   ?= -trimpath
CGO_ENABLED ?= 0

# Cross-compile matrix. Override on the command line, e.g.
#   make release PLATFORMS="linux/amd64 linux/arm64"
PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

GO        ?= go

.PHONY: all build install uninstall release clean test vet fmt tidy help

all: build

build:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN) .

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(BIN) $(DESTDIR)$(BINDIR)/$(BIN)
	@echo "installed $(DESTDIR)$(BINDIR)/$(BIN)"

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BIN)

# Cross-compile every entry in PLATFORMS into $(DIST)/.
release: | $(DIST)
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%%/*}; arch=$${p##*/}; \
		out=$(DIST)/$(BIN)-$(VERSION)-$$os-$$arch; \
		[ "$$os" = "windows" ] && out=$$out.exe; \
		echo ">> $$out"; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$$os GOARCH=$$arch \
			$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $$out .; \
	done

$(DIST):
	mkdir -p $(DIST)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -s -w .

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BIN)
	rm -rf $(DIST)

help:
	@echo "Targets:"
	@echo "  build      build $(BIN) for the host platform"
	@echo "  install    install to \$$(DESTDIR)\$$(BINDIR)  [PREFIX=$(PREFIX)]"
	@echo "  uninstall  remove the installed binary"
	@echo "  release    cross-compile to \$$(DIST)/  [PLATFORMS='$(PLATFORMS)']"
	@echo "  test vet fmt tidy clean"
