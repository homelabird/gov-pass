#!/usr/bin/env sh
set -eu

TARGET="https://example.com"
CONC=20
REQUESTS=200

usage() {
  echo "usage: $0 [--target URL] [--concurrency N] [--requests N]"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --target)
      TARGET="$2"
      shift 2
      ;;
    --concurrency)
      CONC="$2"
      shift 2
      ;;
    --requests)
      REQUESTS="$2"
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

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required"
  exit 1
fi

if command -v nstat >/dev/null 2>&1; then
  echo "nstat snapshot (before):"
  nstat -az | head -n 20
fi

if command -v wrk >/dev/null 2>&1; then
  echo "using wrk"
  wrk -t2 -c "$CONC" -d 10s "$TARGET"
elif command -v hey >/dev/null 2>&1; then
  echo "using hey"
  hey -n "$REQUESTS" -c "$CONC" "$TARGET"
else
  echo "using curl loop"
  seq 1 "$REQUESTS" | xargs -I{} -P "$CONC" sh -c "curl -sk --max-time 5 \"$TARGET\" >/dev/null"
fi

if command -v nstat >/dev/null 2>&1; then
  echo "nstat snapshot (after):"
  nstat -az | head -n 20
fi

if command -v ss >/dev/null 2>&1; then
  echo "ss summary:"
  ss -s
fi
