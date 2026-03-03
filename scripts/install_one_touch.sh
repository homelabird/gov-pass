#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "go is required — install from https://go.dev/dl/"
  exit 1
fi

OS="$(uname -s)"
INSTALL_TUI="${INSTALL_TUI:-0}"

if [ "$OS" != "Linux" ] && [ "$OS" != "FreeBSD" ]; then
  echo "unsupported OS: $OS"
  exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root (e.g. sudo ./scripts/install_one_touch.sh)"
  exit 1
fi

cd "$ROOT_DIR"
go build -o dist/splitter ./cmd/splitter

if [ "$OS" = "Linux" ]; then
  make install
  if [ "$INSTALL_TUI" = "1" ]; then
    make install-tui
  fi
  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable --now gov-pass
  fi
  echo "Installed on Linux: /opt/gov-pass/dist/splitter"
  if [ "$INSTALL_TUI" = "1" ]; then
    echo "Installed on Linux: /opt/gov-pass/dist/gov-pass-tui (TUI controller)"
    echo "Linux runs as terminal TUI controller (nmtui-like via whiptail when available)."
  fi
  exit 0
fi

install -d /usr/local/sbin
install -m 0755 dist/splitter /usr/local/sbin/splitter
if [ "$INSTALL_TUI" = "1" ]; then
  go build -o dist/gov-pass-tui ./cmd/gov-pass-tui
  install -m 0755 dist/gov-pass-tui /usr/local/sbin/gov-pass-tui
fi

echo "Installed on FreeBSD: /usr/local/sbin/splitter"
if [ "$INSTALL_TUI" = "1" ]; then
  echo "Installed on FreeBSD: /usr/local/sbin/gov-pass-tui (TUI controller)"
fi
echo "Configure pf divert rules before starting (see docs/pf/ and docs/DESIGN_BSD.md)."
echo "Start command: /usr/local/sbin/splitter"
