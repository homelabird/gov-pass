.PHONY: build build-tray ensure-tray-deps test vet clean install install-tray uninstall

PREFIX   ?= /opt/gov-pass
DISTDIR  ?= dist

GO       ?= go
GOFLAGS  ?=

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# ── build ────────────────────────────────────────────────────────────────
build:
	$(GO) build $(GOFLAGS) -o $(DISTDIR)/splitter ./cmd/splitter

build-tray: ensure-tray-deps
	$(GO) build $(GOFLAGS) -o $(DISTDIR)/gov-pass-tray ./cmd/gov-pass-tray

# ── tray GUI dependencies (Linux) ───────────────────────────────────────
ensure-tray-deps:
ifeq ($(shell uname -s),Linux)
	@if ! pkg-config --exists ayatana-appindicator3-0.1 gtk+-3.0 2>/dev/null; then \
		echo "Installing tray GUI build dependencies…"; \
		if command -v apt-get >/dev/null 2>&1; then \
			sudo apt-get update -qq && sudo apt-get install -y --no-install-recommends \
				libayatana-appindicator3-dev libgtk-3-dev pkg-config gcc; \
		elif command -v dnf >/dev/null 2>&1; then \
			sudo dnf install -y libayatana-appindicator-gtk3-devel gtk3-devel pkg-config gcc; \
		elif command -v yum >/dev/null 2>&1; then \
			sudo yum install -y libayatana-appindicator-gtk3-devel gtk3-devel pkgconfig gcc; \
		elif command -v pacman >/dev/null 2>&1; then \
			sudo pacman -Sy --noconfirm --needed libayatana-appindicator gtk3 pkgconf gcc; \
		elif command -v apk >/dev/null 2>&1; then \
			sudo apk add --no-cache libayatana-appindicator-dev gtk+3.0-dev pkgconf gcc musl-dev; \
		elif command -v zypper >/dev/null 2>&1; then \
			sudo zypper --non-interactive install typelib-1_0-AyatanaAppIndicator3-0_1 gtk3-devel pkg-config gcc; \
		else \
			echo "Error: unsupported package manager; install libayatana-appindicator and GTK3 development libraries manually" >&2; \
			exit 1; \
		fi; \
	else \
		echo "Tray GUI dependencies already installed."; \
	fi
endif

# ── quality ──────────────────────────────────────────────────────────────
test:
	$(GO) test ./internal/... ./cmd/splitter/...

vet:
	$(GO) vet ./internal/... ./cmd/splitter/...

# ── install / uninstall (Linux, requires root) ──────────────────────────
install: build
	install -d $(DESTDIR)$(PREFIX)/dist
	install -m 0755 $(DISTDIR)/splitter $(DESTDIR)$(PREFIX)/dist/splitter
	install -D -m 0644 scripts/linux/gov-pass.service $(DESTDIR)/etc/systemd/system/gov-pass.service
	@echo "Installed to $(DESTDIR)$(PREFIX)"
	@echo "Run: sudo systemctl daemon-reload && sudo systemctl enable --now gov-pass"

install-tray: build-tray
	install -d $(DESTDIR)$(PREFIX)/dist
	install -m 0755 $(DISTDIR)/gov-pass-tray $(DESTDIR)$(PREFIX)/dist/gov-pass-tray
	@echo "Tray installed to $(DESTDIR)$(PREFIX)/dist/gov-pass-tray"

uninstall:
	systemctl disable --now gov-pass 2>/dev/null || true
	rm -f $(DESTDIR)/etc/systemd/system/gov-pass.service
	rm -rf $(DESTDIR)$(PREFIX)
	systemctl daemon-reload 2>/dev/null || true

# ── clean ────────────────────────────────────────────────────────────────
clean:
	rm -rf $(DISTDIR)
