#!/usr/bin/env sh
set -eu

NS="govpass"
VETH_HOST="veth-gp0"
VETH_NS="veth-gp1"
HOST_IP="10.200.1.1/24"
NS_IP="10.200.1.2/24"
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
  if ip netns list | grep -q "^${NS}\b"; then
    ip netns exec "$NS" "$ROOT/scripts/linux/uninstall_nfqueue.sh" --queue-num "$QUEUE_NUM" --mark "$MARK" >/dev/null 2>&1 || true
    ip netns del "$NS" >/dev/null 2>&1 || true
  fi
  ip link del "$VETH_HOST" >/dev/null 2>&1 || true
  if [ -n "${CERT_DIR:-}" ]; then
    rm -rf "$CERT_DIR" || true
  fi
}
trap cleanup EXIT

ip netns add "$NS"
ip link add "$VETH_HOST" type veth peer name "$VETH_NS"
ip link set "$VETH_NS" netns "$NS"
ip addr add "$HOST_IP" dev "$VETH_HOST"
ip link set "$VETH_HOST" up
ip -n "$NS" addr add "$NS_IP" dev "$VETH_NS"
ip -n "$NS" link set "$VETH_NS" up
ip -n "$NS" link set lo up

CERT_DIR="$(mktemp -d)"
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$CERT_DIR/key.pem" -out "$CERT_DIR/cert.pem" \
  -subj "/CN=gov-pass-test" -days 1 >/dev/null 2>&1

openssl s_server -quiet -accept 10.200.1.1:443 \
  -key "$CERT_DIR/key.pem" -cert "$CERT_DIR/cert.pem" >/dev/null 2>&1 &
SERVER_PID=$!

ip netns exec "$NS" "$ROOT/scripts/linux/install_nfqueue.sh" --queue-num "$QUEUE_NUM" --mark "$MARK"
ip netns exec "$NS" "$BIN" --queue-num "$QUEUE_NUM" --mark "$MARK" >/dev/null 2>&1 &
SPLITTER_PID=$!

sleep 1
ip netns exec "$NS" curl -sk --max-time 5 https://10.200.1.1/ >/dev/null

echo "netns integration test: OK"
