#!/usr/bin/env sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

NS="govpass"
CLIENT_NS=""
SERVER_NS=""
CLIENT_VETH="veth-gpc0"
SERVER_VETH="veth-gps0"
SERVER_IP="10.200.1.1/24"
CLIENT_IP="10.200.1.2/24"
QUEUE_NUM=100
MARK=1
SCRIPT_DIR="${0%/*}"
if [ "$SCRIPT_DIR" = "$0" ]; then
  SCRIPT_DIR="."
fi
ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.."; pwd)"
BIN="$ROOT/dist/splitter"

usage() {
  echo "usage: $0 [--queue-num N] [--mark N] [--ns NAME]"
}

need_arg() {
  if [ "$#" -lt 2 ] || [ -z "$2" ]; then
    echo "$1 requires a value"
    usage
    exit 1
  fi
}

validate_uint() {
  label="$1"
  value="$2"
  max="$3"
  case "$value" in
    ''|*[!0-9]*)
      echo "$label must be an unsigned integer"
      exit 1
      ;;
  esac
  if [ "$value" -gt "$max" ]; then
    echo "$label must be <= $max"
    exit 1
  fi
}

validate_netns_name() {
  value="$1"
  case "$value" in
    ''|-*|*/*|*\\*)
      echo "--ns contains an unsupported value: $value"
      exit 1
      ;;
  esac
  case "$value" in
    *[!A-Za-z0-9_.-]*)
      echo "--ns must use only A-Z, a-z, 0-9, '_', '.', or '-'"
      exit 1
      ;;
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
  echo "missing dependency: $name" >&2
  exit 1
}

netns_exists() {
  "$IP_BIN" netns list | "$AWK_BIN" '{print $1}' | "$GREP_BIN" -Fxq -- "$1"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --queue-num)
      need_arg "$@"
      QUEUE_NUM="$2"
      shift 2
      ;;
    --mark)
      need_arg "$@"
      MARK="$2"
      shift 2
      ;;
    --ns)
      need_arg "$@"
      NS="$2"
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

validate_uint "--queue-num" "$QUEUE_NUM" 65535
validate_uint "--mark" "$MARK" 4294967295
validate_netns_name "$NS"

CLIENT_NS="${NS}-client"
SERVER_NS="${NS}-server"

ID_BIN="$(lookup_trusted_command id)"
IP_BIN="$(lookup_trusted_command ip)"
AWK_BIN="$(lookup_trusted_command awk)"
GREP_BIN="$(lookup_trusted_command grep)"
OPENSSL_BIN="$(lookup_trusted_command openssl)"
CURL_BIN="$(lookup_trusted_command curl)"
MKTEMP_BIN="$(lookup_trusted_command mktemp)"
RM_BIN="$(lookup_trusted_command rm)"
SLEEP_BIN="$(lookup_trusted_command sleep)"
NFT_BIN="$(lookup_optional_trusted_command nft || true)"
IPTABLES_BIN="$(lookup_optional_trusted_command iptables || true)"
IP6TABLES_BIN="$(lookup_optional_trusted_command ip6tables || true)"

if [ "$("$ID_BIN" -u)" -ne 0 ]; then
  echo "root required"
  exit 1
fi

if [ -z "$NFT_BIN" ] && { [ -z "$IPTABLES_BIN" ] || [ -z "$IP6TABLES_BIN" ]; }; then
  echo "missing dependency: nft or iptables+ip6tables"
  exit 1
fi

if [ ! -x "$BIN" ]; then
  echo "splitter not found: $BIN"
  exit 1
fi

cleanup() {
  if [ -n "${SPLITTER_PID:-}" ]; then
    kill "$SPLITTER_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "${SERVER_PID:-}" ]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  if netns_exists "$CLIENT_NS"; then
    "$IP_BIN" netns exec "$CLIENT_NS" "$ROOT/scripts/linux/uninstall_nfqueue.sh" --queue-num "$QUEUE_NUM" --mark "$MARK" >/dev/null 2>&1 || true
    "$IP_BIN" netns del "$CLIENT_NS" >/dev/null 2>&1 || true
  fi
  if netns_exists "$SERVER_NS"; then
    "$IP_BIN" netns del "$SERVER_NS" >/dev/null 2>&1 || true
  fi
  if [ -n "${CERT_DIR:-}" ]; then
    "$RM_BIN" -rf "$CERT_DIR" || true
  fi
}
trap cleanup EXIT

"$IP_BIN" netns add "$CLIENT_NS"
"$IP_BIN" netns add "$SERVER_NS"
"$IP_BIN" link add "$CLIENT_VETH" type veth peer name "$SERVER_VETH"
"$IP_BIN" link set "$CLIENT_VETH" netns "$CLIENT_NS"
"$IP_BIN" link set "$SERVER_VETH" netns "$SERVER_NS"
"$IP_BIN" -n "$CLIENT_NS" addr add "$CLIENT_IP" dev "$CLIENT_VETH"
"$IP_BIN" -n "$CLIENT_NS" link set "$CLIENT_VETH" up
"$IP_BIN" -n "$CLIENT_NS" link set lo up
"$IP_BIN" -n "$SERVER_NS" addr add "$SERVER_IP" dev "$SERVER_VETH"
"$IP_BIN" -n "$SERVER_NS" link set "$SERVER_VETH" up
"$IP_BIN" -n "$SERVER_NS" link set lo up

CERT_DIR="$("$MKTEMP_BIN" -d)"
"$OPENSSL_BIN" req -x509 -newkey rsa:2048 -nodes \
  -keyout "$CERT_DIR/key.pem" -out "$CERT_DIR/cert.pem" \
  -subj "/CN=gov-pass-test" -days 1 >/dev/null 2>&1

"$IP_BIN" netns exec "$SERVER_NS" "$OPENSSL_BIN" s_server -quiet -WWW -accept 10.200.1.1:443 \
  -key "$CERT_DIR/key.pem" -cert "$CERT_DIR/cert.pem" >/dev/null 2>&1 &
SERVER_PID=$!

"$IP_BIN" netns exec "$CLIENT_NS" "$ROOT/scripts/linux/install_nfqueue.sh" --queue-num "$QUEUE_NUM" --mark "$MARK"
# Exercise the packaged helper scripts directly and keep runtime helpers off so
# the smoke test stays deterministic on dedicated CI runners.
"$IP_BIN" netns exec "$CLIENT_NS" "$BIN" \
  --queue-num "$QUEUE_NUM" \
  --mark "$MARK" \
  --auto-rules=false \
  --auto-offload=false \
  --auto-install-tools=false >/dev/null 2>&1 &
SPLITTER_PID=$!

success=0
for _ in 1 2 3 4 5; do
  if "$IP_BIN" netns exec "$CLIENT_NS" "$CURL_BIN" -sk --max-time 5 https://10.200.1.1/ >/dev/null; then
    success=1
    break
  fi
  "$SLEEP_BIN" 1
done

if [ "$success" -ne 1 ]; then
  echo "netns integration test: TLS handshake failed"
  exit 1
fi

echo "netns integration test: OK"
