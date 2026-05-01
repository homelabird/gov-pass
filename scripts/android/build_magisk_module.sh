#!/usr/bin/env sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

TEMPLATE=""
SPLITTER=""
LIB_DIR=""
OUT=""
VERSION=""
VERSION_CODE=""

usage() {
  echo "usage: $0 --template DIR --splitter PATH --lib-dir DIR --out PATH --version VER --version-code N"
}

require_value() {
  if [ "$#" -lt 2 ]; then
    echo "$1 requires a value" >&2
    usage
    exit 1
  fi
}

validate_path_arg() {
  name="$1"
  value="$2"
  case "$value" in
    ""|-*)
      echo "$name must be a non-option path" >&2
      exit 1
      ;;
  esac
}

validate_version_value() {
  case "$VERSION" in
    ""|.*|*/*|*\\*)
      echo "--version is invalid: $VERSION" >&2
      exit 1
      ;;
  esac
  case "$VERSION" in
    *[!A-Za-z0-9._+-]*)
      echo "--version contains unsupported characters: $VERSION" >&2
      exit 1
      ;;
  esac
  case "$VERSION_CODE" in
    ""|*[!0-9]*)
      echo "--version-code must be a decimal integer" >&2
      exit 1
      ;;
  esac
}

lookup_trusted_command() {
  name="$1"
  candidate="$(PATH="$TRUSTED_PATH" command -v "$name" 2>/dev/null || true)"
  if [ -z "$candidate" ]; then
    echo "$name is required" >&2
    exit 1
  fi
  case "$candidate" in
    */*) ;;
    *)
      echo "refusing non-path command lookup for $name: $candidate" >&2
      exit 1
      ;;
  esac
  if [ -L "$candidate" ]; then
    echo "refusing symlinked command for $name: $candidate" >&2
    exit 1
  fi
  if [ ! -x "$candidate" ] || [ -d "$candidate" ]; then
    echo "trusted command is not executable: $candidate" >&2
    exit 1
  fi
  printf '%s\n' "$candidate"
}

copy_shared_libs() {
  found=0
  for lib in "$LIB_DIR"/*.so; do
    if [ -L "$lib" ]; then
      echo "refusing symlinked shared library: $lib" >&2
      exit 1
    fi
    if [ ! -f "$lib" ]; then
      continue
    fi
    "$CP_BIN" "$lib" "$TMP_DIR/lib/"
    found=1
  done
  if [ "$found" -eq 0 ]; then
    echo "no shared libraries found in: $LIB_DIR" >&2
    exit 1
  fi
}

while [ $# -gt 0 ]; do
  case "$1" in
    --template)
      require_value "$@"
      TEMPLATE="$2"
      shift 2
      ;;
    --splitter)
      require_value "$@"
      SPLITTER="$2"
      shift 2
      ;;
    --lib-dir)
      require_value "$@"
      LIB_DIR="$2"
      shift 2
      ;;
    --out)
      require_value "$@"
      OUT="$2"
      shift 2
      ;;
    --version)
      require_value "$@"
      VERSION="$2"
      shift 2
      ;;
    --version-code)
      require_value "$@"
      VERSION_CODE="$2"
      shift 2
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo "unknown arg: $1"
      usage
      exit 1
      ;;
  esac
done

if [ -z "$TEMPLATE" ] || [ -z "$SPLITTER" ] || [ -z "$LIB_DIR" ] || [ -z "$OUT" ] || [ -z "$VERSION" ] || [ -z "$VERSION_CODE" ]; then
  usage
  exit 1
fi

validate_path_arg "--template" "$TEMPLATE"
validate_path_arg "--splitter" "$SPLITTER"
validate_path_arg "--lib-dir" "$LIB_DIR"
validate_path_arg "--out" "$OUT"
validate_version_value

if [ ! -d "$TEMPLATE" ]; then
  echo "template not found: $TEMPLATE"
  exit 1
fi

if [ ! -f "$SPLITTER" ]; then
  echo "splitter not found: $SPLITTER"
  exit 1
fi

if [ ! -d "$LIB_DIR" ]; then
  echo "lib dir not found: $LIB_DIR"
  exit 1
fi

ZIP_BIN="$(lookup_trusted_command zip)"
MKTEMP_BIN="$(lookup_trusted_command mktemp)"
RM_BIN="$(lookup_trusted_command rm)"
CP_BIN="$(lookup_trusted_command cp)"
FIND_BIN="$(lookup_trusted_command find)"
GREP_BIN="$(lookup_trusted_command grep)"
MKDIR_BIN="$(lookup_trusted_command mkdir)"
AWK_BIN="$(lookup_trusted_command awk)"
MV_BIN="$(lookup_trusted_command mv)"
CHMOD_BIN="$(lookup_trusted_command chmod)"

TMP_DIR="$("$MKTEMP_BIN" -d)"
cleanup() {
  "$RM_BIN" -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

"$CP_BIN" -R "$TEMPLATE"/. "$TMP_DIR"/
if "$FIND_BIN" "$TMP_DIR" -type l | "$GREP_BIN" -q .; then
  echo "refusing Magisk template containing symlinks" >&2
  exit 1
fi
"$RM_BIN" -f "$TMP_DIR/splitter"
"$CP_BIN" "$SPLITTER" "$TMP_DIR/splitter"

"$RM_BIN" -rf "$TMP_DIR/lib"
"$MKDIR_BIN" -p "$TMP_DIR/lib"
copy_shared_libs

PROP_FILE="$TMP_DIR/module.prop"
if [ -f "$PROP_FILE" ]; then
  "$AWK_BIN" -v ver="$VERSION" -v vcode="$VERSION_CODE" '
    /^version=/ {print "version=" ver; next}
    /^versionCode=/ {print "versionCode=" vcode; next}
    {print}
  ' "$PROP_FILE" > "$PROP_FILE.tmp"
  "$MV_BIN" "$PROP_FILE.tmp" "$PROP_FILE"
fi

"$CHMOD_BIN" 0755 "$TMP_DIR/splitter"
for script in service.sh post-fs-data.sh uninstall.sh iptables_add.sh iptables_del.sh; do
  if [ -f "$TMP_DIR/$script" ]; then
    "$CHMOD_BIN" 0755 "$TMP_DIR/$script"
  fi
done

case "$OUT" in
  */*)
    OUT_DIR="${OUT%/*}"
    OUT_BASE="${OUT##*/}"
    ;;
  *)
    OUT_DIR="."
    OUT_BASE="$OUT"
    ;;
esac
"$MKDIR_BIN" -p "$OUT_DIR"
OUT_ABS="$(cd "$OUT_DIR" && pwd)/$OUT_BASE"

cd "$TMP_DIR"
"$ZIP_BIN" -r "$OUT_ABS" .
echo "built $OUT_ABS"
