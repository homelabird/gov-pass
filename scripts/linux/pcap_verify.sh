#!/usr/bin/env sh
set -eu

IFACE="eth0"
OUT=""
OUT_SET=0
TARGET_URL="https://example.com"
EXTRA_WAIT=2
RUN_CUSTOM=0
TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

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
  echo "$name is required" >&2
  exit 1
}

usage() {
  echo "usage: $0 [--iface IFACE] [--out FILE] [--url URL] [--wait SEC] [-- command [arg...]]"
}

need_arg() {
  if [ "$#" -lt 2 ] || [ -z "$2" ]; then
    echo "$1 requires a value"
    usage
    exit 1
  fi
}

validate_iface_name() {
  case "$IFACE" in
    ""|-*|*/*|*\\*|*[!A-Za-z0-9_.:@-]*)
      echo "--iface contains unsupported characters: $IFACE"
      exit 1
      ;;
  esac
}

validate_nonnegative_uint() {
  name="$1"
  value="$2"
  case "$value" in
    ""|*[!0-9]*)
      echo "$name must be a non-negative decimal integer"
      exit 1
      ;;
  esac
}

validate_url() {
  case "$TARGET_URL" in
    http://*|https://*)
      ;;
    *)
      echo "--url must start with http:// or https://"
      exit 1
      ;;
  esac
}

while [ $# -gt 0 ]; do
  case "$1" in
    --iface)
      need_arg "$@"
      IFACE="$2"
      shift 2
      ;;
    --out)
      need_arg "$@"
      OUT="$2"
      OUT_SET=1
      shift 2
      ;;
    --url)
      need_arg "$@"
      TARGET_URL="$2"
      shift 2
      ;;
    --wait)
      need_arg "$@"
      EXTRA_WAIT="$2"
      shift 2
      ;;
    --help)
      usage
      exit 0
      ;;
    --cmd)
      echo "--cmd was removed; use '-- command arg...' to avoid shell string execution"
      usage
      exit 1
      ;;
    --)
      shift
      if [ "$#" -eq 0 ]; then
        echo "-- requires a command"
        usage
        exit 1
      fi
      RUN_CUSTOM=1
      break
      ;;
    *)
      echo "unknown arg: $1"
      usage
      exit 1
      ;;
  esac
done

validate_iface_name
validate_nonnegative_uint "--wait" "$EXTRA_WAIT"
if [ "$RUN_CUSTOM" -eq 0 ]; then
  validate_url
fi

ID_BIN="$(lookup_trusted_command id)"
if [ "$("$ID_BIN" -u)" -ne 0 ]; then
  echo "root required"
  exit 1
fi

TCPDUMP_BIN="$(lookup_trusted_command tcpdump)"
SLEEP_BIN="$(lookup_trusted_command sleep)"

if [ "$RUN_CUSTOM" -eq 0 ]; then
  CURL_BIN="$(lookup_trusted_command curl)"
fi

if [ "$OUT_SET" -eq 0 ]; then
  MKTEMP_BIN="$(lookup_trusted_command mktemp)"
  TMP_BASE="${TMPDIR:-/tmp}"
  OUT="$("$MKTEMP_BIN" "${TMP_BASE%/}/gov-pass.pcap.XXXXXX")"
else
  if [ -L "$OUT" ]; then
    echo "refusing to write pcap through symlink: $OUT"
    exit 1
  fi
  if [ -e "$OUT" ] && [ ! -f "$OUT" ]; then
    echo "refusing to overwrite non-regular output path: $OUT"
    exit 1
  fi
fi

PID=""
stop_capture() {
  if [ -n "$PID" ]; then
    kill -INT "$PID" >/dev/null 2>&1 || true
    wait "$PID" >/dev/null 2>&1 || true
    PID=""
  fi
}
trap stop_capture EXIT HUP INT TERM

"$TCPDUMP_BIN" -i "$IFACE" -s 0 -w "$OUT" 'tcp port 443' >/dev/null 2>&1 &
PID=$!
"$SLEEP_BIN" 1
if [ "$RUN_CUSTOM" -eq 1 ]; then
  "$@" >/dev/null 2>&1 || true
else
  "$CURL_BIN" -sk "$TARGET_URL" >/dev/null || true
fi
"$SLEEP_BIN" "$EXTRA_WAIT"
stop_capture
trap - EXIT HUP INT TERM

echo "pcap saved to $OUT"
if TSHARK_BIN="$(lookup_optional_trusted_command tshark)"; then
  HEAD_BIN="$(lookup_trusted_command head)"
  echo "first data segments:"
  "$TSHARK_BIN" -r "$OUT" -Y 'tcp.port==443 && tcp.len>0' -T fields -e frame.number -e tcp.seq -e tcp.len -e ip.src -e ip.dst | "$HEAD_BIN" -n 10
  echo "expect a small segment (e.g. tcp.len=5) at the start of ClientHello"
else
  echo "install tshark for quick inspection, or open the pcap in Wireshark"
fi
