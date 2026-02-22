#!/usr/bin/env sh
set -eu

NDK=""
API=28
LIBMNL_SRC=""
NFQ_SRC=""
OUT=""
TOOLCHAIN=""

usage() {
  echo "usage: $0 --ndk PATH --api N --libmnl-src DIR --nfq-src DIR --out DIR [--toolchain DIR]"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --ndk)
      NDK="$2"
      shift 2
      ;;
    --api)
      API="$2"
      shift 2
      ;;
    --libmnl-src)
      LIBMNL_SRC="$2"
      shift 2
      ;;
    --nfq-src)
      NFQ_SRC="$2"
      shift 2
      ;;
    --out)
      OUT="$2"
      shift 2
      ;;
    --toolchain)
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

HOST_TAG=""
case "$(uname -s)" in
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

mkdir -p "$OUT"

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

  env CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
    ./configure --host=aarch64-linux-android --prefix="$PREFIX" \
    --enable-shared --disable-static

  make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
  make install
}

echo "building libmnl..."
build_one "$LIBMNL_SRC" "libmnl"

echo "building libnetfilter_queue..."
cd "$NFQ_SRC"
if [ -x "./autogen.sh" ]; then
  ./autogen.sh
fi
PKG_CONFIG_PATH="$OUT/lib/pkgconfig" \
  env CC="$CC" AR="$AR" RANLIB="$RANLIB" STRIP="$STRIP" \
  ./configure --host=aarch64-linux-android --prefix="$OUT" \
  --enable-shared --disable-static --with-libmnl-prefix="$OUT"
make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
make install

echo "deps installed to $OUT"
