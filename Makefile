.PHONY: build build-tray test vet clean install uninstall

PREFIX   ?= /opt/gov-pass
DISTDIR  ?= dist

GO       ?= go
GOFLAGS  ?=

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# ── build ────────────────────────────────────────────────────────────────
build:
	$(GO) build $(GOFLAGS) -o $(DISTDIR)/splitter ./cmd/splitter

build-tray:
	$(GO) build $(GOFLAGS) -o $(DISTDIR)/gov-pass-tray ./cmd/gov-pass-tray

# ── quality ──────────────────────────────────────────────────────────────
test:
	$(GO) test ./internal/... ./cmd/splitter/...

vet:
	$(GO) vet ./internal/... ./cmd/splitter/...

# ── install / uninstall (Linux) ──────────────────────────────────────────
install: build
	install -d $(DESTDIR)$(PREFIX)/dist
	install -m 0755 $(DISTDIR)/splitter $(DESTDIR)$(PREFIX)/dist/splitter
	install -D -m 0644 scripts/linux/gov-pass.service $(DESTDIR)/etc/systemd/system/gov-pass.service
	@echo "Installed to $(DESTDIR)$(PREFIX)"
	@echo "Run: sudo systemctl daemon-reload && sudo systemctl enable --now gov-pass"

uninstall:
	systemctl disable --now gov-pass 2>/dev/null || true
	rm -f $(DESTDIR)/etc/systemd/system/gov-pass.service
	rm -rf $(DESTDIR)$(PREFIX)
	systemctl daemon-reload 2>/dev/null || true

# ── clean ────────────────────────────────────────────────────────────────
clean:
	rm -rf $(DISTDIR)
