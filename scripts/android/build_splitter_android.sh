#!/usr/bin/env sh
set -eu

NDK=""
API=28
DEPS=""
OUT="dist/android/arm64/splitter"
TOOLCHAIN=""

usage() {
  echo "usage: $0 --ndk PATH --api N --deps DIR [--out PATH] [--toolchain DIR]"
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
    --deps)
      DEPS="$2"
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

if [ -z "$NDK" ] || [ -z "$DEPS" ]; then
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
export CGO_ENABLED=1
export GOOS=android
export GOARCH=arm64
export CC
export CGO_CFLAGS="-I$DEPS/include"
export CGO_LDFLAGS="-L$DEPS/lib -lmnl -lnetfilter_queue"

mkdir -p "$(dirname "$OUT")"
go build -o "$OUT" ./cmd/splitter
echo "built $OUT"
