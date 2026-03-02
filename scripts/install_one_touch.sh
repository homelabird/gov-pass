#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "go is required (1.21+) — install from https://go.dev/dl/"
  exit 1
fi

OS="$(uname -s)"

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
  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable --now gov-pass
  fi
  echo "Installed on Linux: /opt/gov-pass/dist/splitter"
  exit 0
fi

install -d /usr/local/sbin
install -m 0755 dist/splitter /usr/local/sbin/gov-pass-splitter

echo "Installed on FreeBSD: /usr/local/sbin/gov-pass-splitter"
echo "Configure pf divert rules before starting (see docs/pf/ and docs/DESIGN_BSD.md)."
echo "Start command: /usr/local/sbin/gov-pass-splitter"
