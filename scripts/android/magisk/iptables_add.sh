#!/system/bin/sh
set -eu

QUEUE_NUM=100
MARK=1
EXCLUDE_LOOPBACK=1

usage() {
  echo "usage: $0 [--queue-num N] [--mark N] [--no-loopback]"
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
    --no-loopback)
      EXCLUDE_LOOPBACK=0
      shift 1
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

IPTABLES="${IPTABLES:-iptables}"
if ! command -v "$IPTABLES" >/dev/null 2>&1; then
  echo "iptables not found"
  exit 1
fi

$IPTABLES -t mangle -C OUTPUT -m mark --mark "$MARK"/"$MARK" -j RETURN 2>/dev/null || \
  $IPTABLES -t mangle -A OUTPUT -m mark --mark "$MARK"/"$MARK" -j RETURN

if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
  $IPTABLES -t mangle -C OUTPUT -o lo -j RETURN 2>/dev/null || \
    $IPTABLES -t mangle -A OUTPUT -o lo -j RETURN
fi

$IPTABLES -t mangle -C OUTPUT -p tcp --dport 443 -j NFQUEUE --queue-num "$QUEUE_NUM" --queue-bypass 2>/dev/null || \
  $IPTABLES -t mangle -A OUTPUT -p tcp --dport 443 -j NFQUEUE --queue-num "$QUEUE_NUM" --queue-bypass
