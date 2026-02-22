#!/usr/bin/env sh
set -eu

TEMPLATE=""
SPLITTER=""
LIB_DIR=""
OUT=""
VERSION=""
VERSION_CODE=""

usage() {
  echo "usage: $0 --template DIR --splitter PATH --lib-dir DIR --out PATH --version VER --version-code N"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --template)
      TEMPLATE="$2"
      shift 2
      ;;
    --splitter)
      SPLITTER="$2"
      shift 2
      ;;
    --lib-dir)
      LIB_DIR="$2"
      shift 2
      ;;
    --out)
      OUT="$2"
      shift 2
      ;;
    --version)
      VERSION="$2"
      shift 2
      ;;
    --version-code)
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

if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required to build the Magisk module"
  exit 1
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

cp -R "$TEMPLATE"/. "$TMP_DIR"/
cp "$SPLITTER" "$TMP_DIR/splitter"

mkdir -p "$TMP_DIR/lib"
cp "$LIB_DIR"/*.so "$TMP_DIR/lib/" 2>/dev/null || true

PROP_FILE="$TMP_DIR/module.prop"
if [ -f "$PROP_FILE" ]; then
  awk -v ver="$VERSION" -v vcode="$VERSION_CODE" '
    /^version=/ {print "version=" ver; next}
    /^versionCode=/ {print "versionCode=" vcode; next}
    {print}
  ' "$PROP_FILE" > "$PROP_FILE.tmp"
  mv "$PROP_FILE.tmp" "$PROP_FILE"
fi

chmod 0755 "$TMP_DIR/splitter"
for script in service.sh post-fs-data.sh uninstall.sh iptables_add.sh iptables_del.sh; do
  if [ -f "$TMP_DIR/$script" ]; then
    chmod 0755 "$TMP_DIR/$script"
  fi
done

OUT_DIR="$(dirname "$OUT")"
mkdir -p "$OUT_DIR"
OUT_ABS="$(cd "$OUT_DIR" && pwd)/$(basename "$OUT")"

cd "$TMP_DIR"
zip -r "$OUT_ABS" .
echo "built $OUT_ABS"
