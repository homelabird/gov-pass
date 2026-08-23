#!/usr/bin/env sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

TARGET="https://example.com"
CONC=20
REQUESTS=200

usage() {
  echo "usage: $0 [--target URL] [--concurrency N] [--requests N]"
}

need_arg() {
  if [ "$#" -lt 2 ] || [ -z "$2" ]; then
    echo "$1 requires a value"
    usage
    exit 1
  fi
}

validate_positive_uint() {
  label="$1"
  value="$2"
  case "$value" in
    ''|*[!0-9]*)
      echo "$label must be a positive integer"
      exit 1
      ;;
  esac
  if [ "$value" -lt 1 ]; then
    echo "$label must be >= 1"
    exit 1
  fi
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
    resolved="$(readlink -f -- "$candidate" 2>/dev/null || true)"
    case "$resolved" in
      /usr/local/sbin/*|/usr/local/bin/*|/usr/sbin/*|/usr/bin/*|/sbin/*|/bin/*)
        candidate="$resolved"
        ;;
      *)
        echo "refusing symlinked command outside trusted directories for $name: $candidate" >&2
        exit 1
        ;;
    esac
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
  echo "$name is required" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --target)
      need_arg "$@"
      TARGET="$2"
      shift 2
      ;;
    --concurrency)
      need_arg "$@"
      CONC="$2"
      shift 2
      ;;
    --requests)
      need_arg "$@"
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

case "$TARGET" in
  http://*|https://*) ;;
  *)
    echo "--target must start with http:// or https://"
    exit 1
    ;;
esac
validate_positive_uint "--concurrency" "$CONC"
validate_positive_uint "--requests" "$REQUESTS"

CURL_BIN="$(lookup_trusted_command curl)"
NSTAT_BIN="$(lookup_optional_trusted_command nstat || true)"
WRK_BIN="$(lookup_optional_trusted_command wrk || true)"
HEY_BIN="$(lookup_optional_trusted_command hey || true)"
SS_BIN="$(lookup_optional_trusted_command ss || true)"

if [ -n "$NSTAT_BIN" ]; then
  HEAD_BIN="$(lookup_trusted_command head)"
  echo "nstat snapshot (before):"
  "$NSTAT_BIN" -az | "$HEAD_BIN" -n 20
fi

if [ -n "$WRK_BIN" ]; then
  echo "using wrk"
  "$WRK_BIN" -t2 -c "$CONC" -d 10s "$TARGET"
elif [ -n "$HEY_BIN" ]; then
  echo "using hey"
  "$HEY_BIN" -n "$REQUESTS" -c "$CONC" "$TARGET"
else
  XARGS_BIN="$(lookup_trusted_command xargs)"
  echo "using curl loop"
  i=0
  while [ "$i" -lt "$REQUESTS" ]; do
    printf '%s\0' "$TARGET"
    i=$((i + 1))
  done | "$XARGS_BIN" -0 -n 1 -P "$CONC" "$CURL_BIN" -sk --max-time 5 -o /dev/null
fi

if [ -n "$NSTAT_BIN" ]; then
  HEAD_BIN="$(lookup_trusted_command head)"
  echo "nstat snapshot (after):"
  "$NSTAT_BIN" -az | "$HEAD_BIN" -n 20
fi

if [ -n "$SS_BIN" ]; then
  echo "ss summary:"
  "$SS_BIN" -s
fi
