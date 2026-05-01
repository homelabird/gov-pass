.PHONY: build build-tui ensure-tui-deps test vet clean install install-tui uninstall

PREFIX   ?= /opt/gov-pass
DISTDIR  ?= dist

GO       ?= go
GOFLAGS  ?=
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# ── build ────────────────────────────────────────────────────────────────
build:
	$(GO) build $(GOFLAGS) -o $(DISTDIR)/splitter ./cmd/splitter

build-tui: ensure-tui-deps
	$(GO) build $(GOFLAGS) -o $(DISTDIR)/gov-pass-tui ./cmd/gov-pass-tui

# ── TUI dependencies (Linux) ───────────────────────────────────────────
ensure-tui-deps:
ifeq ($(shell uname -s),Linux)
	@echo "No additional Linux TUI build dependencies are required."
endif

# ── quality ──────────────────────────────────────────────────────────────
test:
	$(GO) test $(GOFLAGS) ./... -count=1

vet:
	$(GO) vet $(GOFLAGS) ./...

# ── install / uninstall (Linux, requires root) ──────────────────────────
install: build
	install -d $(DESTDIR)$(PREFIX)/dist
	install -m 0755 $(DISTDIR)/splitter $(DESTDIR)$(PREFIX)/dist/splitter
	install -D -m 0644 scripts/linux/gov-pass.service $(DESTDIR)/etc/systemd/system/gov-pass.service
	@echo "Installed to $(DESTDIR)$(PREFIX)"
	@echo "Run: sudo systemctl daemon-reload && sudo systemctl enable --now gov-pass"

install-tui: build-tui
	install -d $(DESTDIR)$(PREFIX)/dist
	install -m 0755 $(DISTDIR)/gov-pass-tui $(DESTDIR)$(PREFIX)/dist/gov-pass-tui
	@echo "TUI controller installed to $(DESTDIR)$(PREFIX)/dist/gov-pass-tui"

uninstall:
	prefix="$(DESTDIR)$(PREFIX)"; \
	case "$$prefix" in ""|"/"|"/usr"|"/usr/local"|"/opt"|"/etc"|"/bin"|"/sbin"|"/lib"|"/lib64") \
		echo "refusing to uninstall unsafe PREFIX: $$prefix"; exit 1 ;; \
	esac; \
	systemctl disable --now gov-pass 2>/dev/null || true; \
	rm -f "$(DESTDIR)/etc/systemd/system/gov-pass.service"; \
	rm -rf "$$prefix"; \
	systemctl daemon-reload 2>/dev/null || true

# ── clean ────────────────────────────────────────────────────────────────
clean:
	distdir="$(DISTDIR)"; \
	case "$$distdir" in ""|"/"|"."|".."|../*|*/../*|*/..|/*) echo "refusing to clean unsafe DISTDIR: $$distdir"; exit 1 ;; esac; \
	rm -rf "$$distdir"
