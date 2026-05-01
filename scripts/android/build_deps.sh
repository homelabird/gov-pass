#!/usr/bin/env sh
set -eu

TRUSTED_PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

NDK=""
API=28
LIBMNL_SRC=""
NFQ_SRC=""
OUT=""
TOOLCHAIN=""
ORIGINAL_DIR="$(pwd -P)"

usage() {
  echo "usage: $0 --ndk PATH --api N --libmnl-src DIR --nfq-src DIR --out DIR [--toolchain DIR]"
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

parallel_jobs() {
  if [ -n "${GETCONF_BIN:-}" ] && jobs="$("$GETCONF_BIN" _NPROCESSORS_ONLN 2>/dev/null)" && [ -n "$jobs" ]; then
    case "$jobs" in
      *[!0-9]*|0) echo 4 ;;
      *) echo "$jobs" ;;
    esac
  else
    echo 4
  fi
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
    --libmnl-src)
      require_value "$@"
      LIBMNL_SRC="$2"
      shift 2
      ;;
    --nfq-src)
      require_value "$@"
      NFQ_SRC="$2"
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

if [ -z "$NDK" ] || [ -z "$LIBMNL_SRC" ] || [ -z "$NFQ_SRC" ] || [ -z "$OUT" ]; then
  usage
  exit 1
fi

validate_android_api
validate_path_arg "--ndk" "$NDK"
validate_path_arg "--libmnl-src" "$LIBMNL_SRC"
validate_path_arg "--nfq-src" "$NFQ_SRC"
validate_path_arg "--out" "$OUT"
if [ -n "$TOOLCHAIN" ]; then
  validate_path_arg "--toolchain" "$TOOLCHAIN"
fi

NDK="$(absolute_path_arg "$NDK")"
LIBMNL_SRC="$(absolute_path_arg "$LIBMNL_SRC")"
NFQ_SRC="$(absolute_path_arg "$NFQ_SRC")"
OUT="$(absolute_path_arg "$OUT")"
if [ -n "$TOOLCHAIN" ]; then
  TOOLCHAIN="$(absolute_path_arg "$TOOLCHAIN")"
fi

UNAME_BIN="$(lookup_trusted_command uname)"
MKDIR_BIN="$(lookup_trusted_command mkdir)"
MAKE_BIN="$(lookup_trusted_command make)"
GETCONF_BIN="$(lookup_optional_trusted_command getconf || true)"
ENV_BIN="$(lookup_trusted_command env)"

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
AR="$TOOLCHAIN/bin/llvm-ar"
RANLIB="$TOOLCHAIN/bin/llvm-ranlib"
STRIP="$TOOLCHAIN/bin/llvm-strip"

"$MKDIR_BIN" -p "$OUT"

for tool in "$CC" "$AR" "$RANLIB" "$STRIP"; do
  if [ ! -x "$tool" ]; then
    echo "required tool not executable: $tool" >&2
    exit 1
  fi
done

build_one() {
  SRC="$1"
  NAME="$2"
  PREFIX="$OUT"

  if [ ! -d "$SRC" ]; then
    echo "$NAME source not found: $SRC"
    exit 1
  fi

  cd "$SRC"
  if [ -x "./autogen.sh" ]; then
    ./autogen.sh
  fi

  "$ENV_BIN" CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
    ./configure --host=aarch64-linux-android --prefix="$PREFIX" \
    --enable-shared --disable-static

  "$MAKE_BIN" -j"$(parallel_jobs)"
  "$MAKE_BIN" install
}

echo "building libmnl..."
build_one "$LIBMNL_SRC" "libmnl"

echo "building libnetfilter_queue..."
cd "$NFQ_SRC"
if [ -x "./autogen.sh" ]; then
  ./autogen.sh
fi
PKG_CONFIG_PATH="$OUT/lib/pkgconfig" \
  "$ENV_BIN" CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
  ./configure --host=aarch64-linux-android --prefix="$OUT" \
  --enable-shared --disable-static --with-libmnl-prefix="$OUT"
"$MAKE_BIN" -j"$(parallel_jobs)"
"$MAKE_BIN" install

echo "deps installed to $OUT"
