#!/usr/bin/env sh
set -eu

TRUSTED_PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

NDK=""
API=28
DEPS=""
OUT="dist/android/arm64/splitter"
TOOLCHAIN=""
ORIGINAL_DIR="$(pwd -P)"

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir="." ;;
esac
SCRIPT_DIR="$(cd "$script_dir" && pwd -P)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd -P)"

usage() {
  echo "usage: $0 --ndk PATH --api N --deps DIR [--out PATH] [--toolchain DIR]"
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

validate_android_api() {
  case "$API" in
    ""|*[!0-9]*)
      echo "--api must be a decimal Android API level" >&2
      exit 1
      ;;
  esac
  api="$API"
  while [ "${api#0}" != "$api" ]; do
    api="${api#0}"
  done
  if [ -z "$api" ]; then
    api=0
  fi
  if [ "${#api}" -lt 2 ] || { [ "${#api}" -eq 2 ] && [ "$api" \< "21" ]; } ||
    [ "${#api}" -gt 3 ] || { [ "${#api}" -eq 3 ] && [ "$api" \> "100" ]; }; then
    echo "--api out of supported range: $API" >&2
    exit 1
  fi
  API="$api"
}

absolute_path_arg() {
  value="$1"
  case "$value" in
    /*) printf '%s\n' "$value" ;;
    *) printf '%s\n' "$ORIGINAL_DIR/$value" ;;
  esac
}

lookup_optional_trusted_command() {
  name="$1"
  candidate="$(PATH="$TRUSTED_PATH" command -v "$name" 2>/dev/null || true)"
  if [ -z "$candidate" ]; then
    return 1
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

lookup_trusted_command() {
  name="$1"
  if candidate="$(lookup_optional_trusted_command "$name")"; then
    printf '%s\n' "$candidate"
    return 0
  fi
  echo "required command not found in trusted directories: $name" >&2
  exit 1
}

validate_explicit_tool_path() {
  name="$1"
  value="$2"
  case "$value" in
    /*) ;;
    *)
      echo "$name override must be an absolute path: $value" >&2
      exit 1
      ;;
  esac
  if [ -L "$value" ]; then
    echo "refusing symlinked $name override: $value" >&2
    exit 1
  fi
  if [ ! -x "$value" ] || [ -d "$value" ]; then
    echo "$name override is not executable: $value" >&2
    exit 1
  fi
  printf '%s\n' "$value"
}

lookup_go_command() {
  if [ -n "${GOV_PASS_GO_BIN:-}" ]; then
    validate_explicit_tool_path "go" "$GOV_PASS_GO_BIN"
    return 0
  fi
  lookup_trusted_command go
}

while [ $# -gt 0 ]; do
  case "$1" in
    --ndk)
      require_value "$@"
      NDK="$2"
      shift 2
      ;;
    --api)
      require_value "$@"
      API="$2"
      shift 2
      ;;
    --deps)
      require_value "$@"
      DEPS="$2"
      shift 2
      ;;
    --out)
      require_value "$@"
      OUT="$2"
      shift 2
      ;;
    --toolchain)
      require_value "$@"
      TOOLCHAIN="$2"
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

if [ -z "$NDK" ] || [ -z "$DEPS" ]; then
  usage
  exit 1
fi

validate_android_api
validate_path_arg "--ndk" "$NDK"
validate_path_arg "--deps" "$DEPS"
validate_path_arg "--out" "$OUT"
if [ -n "$TOOLCHAIN" ]; then
  validate_path_arg "--toolchain" "$TOOLCHAIN"
fi

NDK="$(absolute_path_arg "$NDK")"
DEPS="$(absolute_path_arg "$DEPS")"
OUT="$(absolute_path_arg "$OUT")"
if [ -n "$TOOLCHAIN" ]; then
  TOOLCHAIN="$(absolute_path_arg "$TOOLCHAIN")"
fi

UNAME_BIN="$(lookup_trusted_command uname)"
MKDIR_BIN="$(lookup_trusted_command mkdir)"
GO_BIN="$(lookup_go_command)"

HOST_TAG=""
case "$("$UNAME_BIN" -s)" in
  Linux) HOST_TAG="linux-x86_64" ;;
  Darwin) HOST_TAG="darwin-x86_64" ;;
  *) echo "unsupported host"; exit 1 ;;
esac

if [ -z "$TOOLCHAIN" ]; then
  TOOLCHAIN="$NDK/toolchains/llvm/prebuilt/$HOST_TAG"
fi

if [ ! -d "$TOOLCHAIN" ]; then
  echo "toolchain not found: $TOOLCHAIN"
  exit 1
fi

CC="$TOOLCHAIN/bin/aarch64-linux-android${API}-clang"
if [ ! -x "$CC" ]; then
  echo "required compiler not executable: $CC" >&2
  exit 1
fi
if [ ! -d "$DEPS/include" ] || [ ! -d "$DEPS/lib" ]; then
  echo "deps must contain include and lib directories: $DEPS" >&2
  exit 1
fi
export CGO_ENABLED=1
export GOOS=android
export GOARCH=arm64
export CC
export CGO_CFLAGS="-I$DEPS/include"
export CGO_LDFLAGS="-L$DEPS/lib -lmnl -lnetfilter_queue"

case "$OUT" in
  */*) OUT_DIR="${OUT%/*}" ;;
  *) OUT_DIR="." ;;
esac

"$MKDIR_BIN" -p "$OUT_DIR"
cd "$REPO_ROOT"
"$GO_BIN" build -o "$OUT" ./cmd/splitter
echo "built $OUT"
