#!/usr/bin/env sh
set -eu

NS="govpass"
CLIENT_NS=""
SERVER_NS=""
CLIENT_VETH="veth-gpc0"
SERVER_VETH="veth-gps0"
SERVER_IP="10.200.1.1/24"
CLIENT_IP="10.200.1.2/24"
QUEUE_NUM=100
MARK=1
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.."; pwd)"
BIN="$ROOT/dist/splitter"

usage() {
  echo "usage: $0 [--queue-num N] [--mark N] [--ns NAME]"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --queue-num)
      QUEUE_NUM="$2"
      shift 2
      ;;
    --mark)
      MARK="$2"
      shift 2
      ;;
    --ns)
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

CLIENT_NS="${NS}-client"
SERVER_NS="${NS}-server"

if [ "$(id -u)" -ne 0 ]; then
  echo "root required"
  exit 1
fi

for cmd in ip openssl curl; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "missing dependency: $cmd"
    exit 1
  fi
done
if ! command -v nft >/dev/null 2>&1 && \
   { ! command -v iptables >/dev/null 2>&1 || ! command -v ip6tables >/dev/null 2>&1; }; then
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
  if ip netns list | grep -q "^${CLIENT_NS}\b"; then
    ip netns exec "$CLIENT_NS" "$ROOT/scripts/linux/uninstall_nfqueue.sh" --queue-num "$QUEUE_NUM" --mark "$MARK" >/dev/null 2>&1 || true
    ip netns del "$CLIENT_NS" >/dev/null 2>&1 || true
  fi
  if ip netns list | grep -q "^${SERVER_NS}\b"; then
    ip netns del "$SERVER_NS" >/dev/null 2>&1 || true
  fi
  if [ -n "${CERT_DIR:-}" ]; then
    rm -rf "$CERT_DIR" || true
  fi
}
trap cleanup EXIT

ip netns add "$CLIENT_NS"
ip netns add "$SERVER_NS"
ip link add "$CLIENT_VETH" type veth peer name "$SERVER_VETH"
ip link set "$CLIENT_VETH" netns "$CLIENT_NS"
ip link set "$SERVER_VETH" netns "$SERVER_NS"
ip -n "$CLIENT_NS" addr add "$CLIENT_IP" dev "$CLIENT_VETH"
ip -n "$CLIENT_NS" link set "$CLIENT_VETH" up
ip -n "$CLIENT_NS" link set lo up
ip -n "$SERVER_NS" addr add "$SERVER_IP" dev "$SERVER_VETH"
ip -n "$SERVER_NS" link set "$SERVER_VETH" up
ip -n "$SERVER_NS" link set lo up

CERT_DIR="$(mktemp -d)"
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$CERT_DIR/key.pem" -out "$CERT_DIR/cert.pem" \
  -subj "/CN=gov-pass-test" -days 1 >/dev/null 2>&1

ip netns exec "$SERVER_NS" openssl s_server -quiet -WWW -accept 10.200.1.1:443 \
  -key "$CERT_DIR/key.pem" -cert "$CERT_DIR/cert.pem" >/dev/null 2>&1 &
SERVER_PID=$!

ip netns exec "$CLIENT_NS" "$ROOT/scripts/linux/install_nfqueue.sh" --queue-num "$QUEUE_NUM" --mark "$MARK"
# Exercise the packaged helper scripts directly and keep runtime helpers off so
# the smoke test stays deterministic on dedicated CI runners.
ip netns exec "$CLIENT_NS" "$BIN" \
  --queue-num "$QUEUE_NUM" \
  --mark "$MARK" \
  --auto-rules=false \
  --auto-offload=false \
  --auto-install-tools=false >/dev/null 2>&1 &
SPLITTER_PID=$!

success=0
for _ in 1 2 3 4 5; do
  if ip netns exec "$CLIENT_NS" curl -sk --max-time 5 https://10.200.1.1/ >/dev/null; then
    success=1
    break
  fi
  sleep 1
done

if [ "$success" -ne 1 ]; then
  echo "netns integration test: TLS handshake failed"
  exit 1
fi

echo "netns integration test: OK"
