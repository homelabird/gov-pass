#!/usr/bin/env sh
set -eu

IFACE="eth0"
OUT="/tmp/gov-pass.pcap"
CMD="curl -sk https://example.com >/dev/null"
EXTRA_WAIT=2

usage() {
  echo "usage: $0 [--iface IFACE] [--out FILE] [--cmd COMMAND] [--wait SEC]"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --iface)
      IFACE="$2"
      shift 2
      ;;
    --out)
      OUT="$2"
      shift 2
      ;;
    --cmd)
      CMD="$2"
      shift 2
      ;;
    --wait)
      EXTRA_WAIT="$2"
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

if ! command -v tcpdump >/dev/null 2>&1; then
  echo "tcpdump is required"
  exit 1
fi

tcpdump -i "$IFACE" -s 0 -w "$OUT" 'tcp port 443' >/dev/null 2>&1 &
PID=$!
sleep 1
sh -c "$CMD" || true
sleep "$EXTRA_WAIT"
kill -INT "$PID" >/dev/null 2>&1 || true
wait "$PID" >/dev/null 2>&1 || true

echo "pcap saved to $OUT"
if command -v tshark >/dev/null 2>&1; then
  echo "first data segments:"
  tshark -r "$OUT" -Y 'tcp.port==443 && tcp.len>0' -T fields -e frame.number -e tcp.seq -e tcp.len -e ip.src -e ip.dst | head -n 10
  echo "expect a small segment (e.g. tcp.len=5) at the start of ClientHello"
else
  echo "install tshark for quick inspection, or open the pcap in Wireshark"
fi
