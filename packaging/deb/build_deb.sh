#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
DEFAULT_VERSION="$(git -C "$ROOT_DIR" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || true)"
VERSION="${1:-${GOV_PASS_VERSION:-${DEFAULT_VERSION:-0.0.0}}}"
ARCH="${GOV_PASS_ARCH:-amd64}"
PKG_NAME="gov-pass"
STAGE_DIR="$ROOT_DIR/dist/${PKG_NAME}_${VERSION}_${ARCH}"
OUT_DEB="$ROOT_DIR/dist/${PKG_NAME}_${VERSION}_${ARCH}.deb"

for cmd in go dpkg-deb; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: $cmd is required but not found. Please install it to continue."
    exit 1
  fi
done

cd "$ROOT_DIR"
export CGO_ENABLED=1
# Version precedence: arg > GOV_PASS_VERSION > latest git tag > 0.0.0.
go build -o dist/splitter ./cmd/splitter

rm -rf "$STAGE_DIR"
mkdir -p \
  "$STAGE_DIR/DEBIAN" \
  "$STAGE_DIR/usr/libexec/gov-pass" \
  "$STAGE_DIR/lib/systemd/system" \
  "$STAGE_DIR/etc/default"

install -m 0755 dist/splitter "$STAGE_DIR/usr/libexec/gov-pass/splitter"
install -m 0755 scripts/linux/install_nfqueue.sh "$STAGE_DIR/usr/libexec/gov-pass/install_nfqueue.sh"
install -m 0755 scripts/linux/uninstall_nfqueue.sh "$STAGE_DIR/usr/libexec/gov-pass/uninstall_nfqueue.sh"
install -m 0644 packaging/deb/gov-pass.service "$STAGE_DIR/lib/systemd/system/gov-pass.service"
install -m 0644 packaging/deb/gov-pass.default "$STAGE_DIR/etc/default/gov-pass"

cat > "$STAGE_DIR/DEBIAN/control" <<CTRL
Package: $PKG_NAME
Version: $VERSION
Section: net
Priority: optional
Architecture: $ARCH
Maintainer: homelabird maintainers <250380627+homelabird@users.noreply.github.com>
Depends: systemd, ethtool, iproute2
Recommends: nftables | iptables
Description: Split-only TLS ClientHello splitter for outbound TCP/443
 gov-pass intercepts outbound TCP/443 via NFQUEUE and performs
 split-only first-ClientHello processing on Linux.
CTRL

cat > "$STAGE_DIR/DEBIAN/postinst" <<'POSTINST'
#!/usr/bin/env bash
set -e
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi
POSTINST
chmod 0755 "$STAGE_DIR/DEBIAN/postinst"

cat > "$STAGE_DIR/DEBIAN/postrm" <<'POSTRM'
#!/usr/bin/env bash
set -e
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi
POSTRM
chmod 0755 "$STAGE_DIR/DEBIAN/postrm"

rm -f "$OUT_DEB"
dpkg-deb --build "$STAGE_DIR" "$OUT_DEB"
echo "Built $OUT_DEB"
