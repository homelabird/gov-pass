#!/usr/bin/env bash
set -euo pipefail

TRUSTED_PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

lookup_optional_trusted_command() {
  local name="$1"
  local old_ifs="$IFS"
  local dir candidate
  IFS=:
  for dir in $TRUSTED_PATH; do
    candidate="${dir}/${name}"
    if [[ -L "$candidate" ]]; then
      echo "Error: refusing symlinked trusted command for $name: $candidate" >&2
      IFS="$old_ifs"
      exit 1
    fi
    if [[ -f "$candidate" && -x "$candidate" ]]; then
      printf '%s\n' "$candidate"
      IFS="$old_ifs"
      return 0
    fi
  done
  IFS="$old_ifs"
  return 1
}

lookup_trusted_command() {
  local name="$1"
  local candidate
  if candidate="$(lookup_optional_trusted_command "$name")"; then
    printf '%s\n' "$candidate"
    return 0
  fi
  echo "Error: $name is required but was not found in trusted command directories." >&2
  exit 1
}

validate_explicit_tool_path() {
  local name="$1"
  local path="$2"
  if [[ -z "$path" || "$path" != /* ]]; then
    echo "Error: $name override must be an absolute path: $path" >&2
    exit 1
  fi
  if [[ -L "$path" ]]; then
    echo "Error: refusing symlinked $name override: $path" >&2
    exit 1
  fi
  if [[ ! -x "$path" || -d "$path" ]]; then
    echo "Error: $name override is not executable: $path" >&2
    exit 1
  fi
  printf '%s\n' "$path"
}

lookup_go_command() {
  if [[ -n "${GOV_PASS_GO_BIN:-}" ]]; then
    validate_explicit_tool_path go "$GOV_PASS_GO_BIN"
    return 0
  fi
  lookup_trusted_command go
}

SCRIPT_DIR="${BASH_SOURCE[0]%/*}"
if [[ "$SCRIPT_DIR" == "${BASH_SOURCE[0]}" ]]; then
  SCRIPT_DIR="."
fi
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
GIT_BIN="$(lookup_optional_trusted_command git || true)"
DEFAULT_VERSION=""
if [[ -n "$GIT_BIN" ]]; then
  DEFAULT_VERSION="$("$GIT_BIN" -C "$ROOT_DIR" describe --tags --abbrev=0 2>/dev/null || true)"
  DEFAULT_VERSION="${DEFAULT_VERSION#v}"
fi
VERSION="${1:-${GOV_PASS_VERSION:-${DEFAULT_VERSION:-0.0.0}}}"
ARCH="${GOV_PASS_ARCH:-amd64}"
PKG_NAME="gov-pass"
DIST_DIR="$ROOT_DIR/dist"
STAGE_DIR="$DIST_DIR/${PKG_NAME}_${VERSION}_${ARCH}"
OUT_DEB="$DIST_DIR/${PKG_NAME}_${VERSION}_${ARCH}.deb"

validate_package_component() {
  local label="$1"
  local value="$2"
  local pattern="$3"
  if [[ -z "$value" || "$value" == .* || "$value" == *"/"* || "$value" == *"\\"* ]]; then
    echo "Error: invalid $label: $value"
    exit 1
  fi
  if ! [[ "$value" =~ $pattern ]]; then
    echo "Error: invalid $label: $value"
    exit 1
  fi
}

validate_package_component "version" "$VERSION" '^[0-9][0-9A-Za-z.+:~_-]*$'
validate_package_component "architecture" "$ARCH" '^[0-9A-Za-z.+_-]+$'

validate_dist_dir() {
  if [[ -L "$DIST_DIR" ]]; then
    echo "Error: refusing to use symlink dist directory: $DIST_DIR"
    exit 1
  fi
  if [[ -e "$DIST_DIR" && ! -d "$DIST_DIR" ]]; then
    echo "Error: dist path is not a directory: $DIST_DIR"
    exit 1
  fi
}

validate_stage_path() {
  local path="$1"
  local expected_prefix="$DIST_DIR/${PKG_NAME}_"
  if [[ -z "$path" || "$path" == "/" || "$path" == "$ROOT_DIR" || "$path" == "$DIST_DIR" || "$path" == *"/.." || "$path" == *"/../"* ]]; then
    echo "Error: refusing to remove unsafe package stage directory: $path"
    exit 1
  fi
  if [[ "$path" != "$expected_prefix"* ]]; then
    echo "Error: package stage directory is outside expected dist prefix: $path"
    exit 1
  fi
}

validate_deb_output_path() {
  local path="$1"
  local expected_prefix="$DIST_DIR/${PKG_NAME}_"
  if [[ -z "$path" || "$path" == "/" || "$path" == "$ROOT_DIR" || "$path" == "$DIST_DIR" || "$path" == *"/.." || "$path" == *"/../"* ]]; then
    echo "Error: refusing to remove unsafe deb output path: $path"
    exit 1
  fi
  if [[ "$path" != "$expected_prefix"*".deb" ]]; then
    echo "Error: deb output path is outside expected dist prefix: $path"
    exit 1
  fi
}

validate_dist_dir
validate_stage_path "$STAGE_DIR"
validate_deb_output_path "$OUT_DEB"

GO_BIN="$(lookup_go_command)"
DPKG_DEB_BIN="$(lookup_trusted_command dpkg-deb)"
MKDIR_BIN="$(lookup_trusted_command mkdir)"
RM_BIN="$(lookup_trusted_command rm)"
INSTALL_BIN="$(lookup_trusted_command install)"
CHMOD_BIN="$(lookup_trusted_command chmod)"

cd "$ROOT_DIR"
"$MKDIR_BIN" -p "$DIST_DIR"
validate_dist_dir
export CGO_ENABLED=1
# Version precedence: arg > GOV_PASS_VERSION > latest git tag > 0.0.0.
"$GO_BIN" build -o "$DIST_DIR/splitter" ./cmd/splitter

"$RM_BIN" -rf -- "$STAGE_DIR"
"$MKDIR_BIN" -p \
  "$STAGE_DIR/DEBIAN" \
  "$STAGE_DIR/usr/libexec/gov-pass" \
  "$STAGE_DIR/usr/share/doc/gov-pass/examples" \
  "$STAGE_DIR/usr/share/doc/gov-pass/schema" \
  "$STAGE_DIR/lib/systemd/system" \
  "$STAGE_DIR/etc/default"

"$INSTALL_BIN" -m 0755 dist/splitter "$STAGE_DIR/usr/libexec/gov-pass/splitter"
"$INSTALL_BIN" -m 0755 scripts/linux/install_nfqueue.sh "$STAGE_DIR/usr/libexec/gov-pass/install_nfqueue.sh"
"$INSTALL_BIN" -m 0755 scripts/linux/uninstall_nfqueue.sh "$STAGE_DIR/usr/libexec/gov-pass/uninstall_nfqueue.sh"
"$INSTALL_BIN" -m 0644 packaging/deb/gov-pass.service "$STAGE_DIR/lib/systemd/system/gov-pass.service"
"$INSTALL_BIN" -m 0644 packaging/deb/gov-pass.default "$STAGE_DIR/etc/default/gov-pass"
"$INSTALL_BIN" -m 0644 README.md SECURITY.md docs/DESIGN.md docs/THIRD_PARTY_NOTICES.md "$STAGE_DIR/usr/share/doc/gov-pass/"
"$INSTALL_BIN" -m 0644 docs/examples/splitter.*.json "$STAGE_DIR/usr/share/doc/gov-pass/examples/"
"$INSTALL_BIN" -m 0644 docs/schema/* "$STAGE_DIR/usr/share/doc/gov-pass/schema/"

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
PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
export PATH
lookup_optional_trusted_command() {
  name="$1"
  old_ifs="$IFS"
  IFS=:
  for dir in $PATH; do
    candidate="${dir}/${name}"
    if [ -L "$candidate" ]; then
      echo "postinst: refusing symlinked trusted command for $name: $candidate" >&2
      IFS="$old_ifs"
      exit 1
    fi
    if [ -f "$candidate" ] && [ -x "$candidate" ]; then
      printf '%s\n' "$candidate"
      IFS="$old_ifs"
      return 0
    fi
  done
  IFS="$old_ifs"
  return 1
}
SYSTEMCTL_BIN="$(lookup_optional_trusted_command systemctl || true)"
if [ -n "$SYSTEMCTL_BIN" ]; then
  "$SYSTEMCTL_BIN" daemon-reload || true
fi
POSTINST
"$CHMOD_BIN" 0755 "$STAGE_DIR/DEBIAN/postinst"

cat > "$STAGE_DIR/DEBIAN/postrm" <<'POSTRM'
#!/usr/bin/env bash
set -e
PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
export PATH
lookup_optional_trusted_command() {
  name="$1"
  old_ifs="$IFS"
  IFS=:
  for dir in $PATH; do
    candidate="${dir}/${name}"
    if [ -L "$candidate" ]; then
      echo "postrm: refusing symlinked trusted command for $name: $candidate" >&2
      IFS="$old_ifs"
      exit 1
    fi
    if [ -f "$candidate" ] && [ -x "$candidate" ]; then
      printf '%s\n' "$candidate"
      IFS="$old_ifs"
      return 0
    fi
  done
  IFS="$old_ifs"
  return 1
}
SYSTEMCTL_BIN="$(lookup_optional_trusted_command systemctl || true)"
if [ -n "$SYSTEMCTL_BIN" ]; then
  "$SYSTEMCTL_BIN" daemon-reload || true
fi
POSTRM
"$CHMOD_BIN" 0755 "$STAGE_DIR/DEBIAN/postrm"

"$RM_BIN" -f -- "$OUT_DEB"
"$DPKG_DEB_BIN" --build "$STAGE_DIR" "$OUT_DEB"
echo "Built $OUT_DEB"
