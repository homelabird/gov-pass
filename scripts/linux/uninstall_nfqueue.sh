#!/usr/bin/env sh
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

if [ "$(id -u)" -ne 0 ]; then
  echo "root required"
  exit 1
fi

if command -v nft >/dev/null 2>&1; then
  TABLE="gov_pass"
  CHAIN="output"
  TAG="gov-pass"

  if nft list chain inet "$TABLE" "$CHAIN" >/dev/null 2>&1; then
    nft -a list chain inet "$TABLE" "$CHAIN" 2>/dev/null | \
      awk -v tag="comment \\\"$TAG\\\"" '$0 ~ tag { for (i=1;i<=NF;i++) if ($i==\"handle\") print $(i+1) }' | \
      while read -r h; do
        [ -n "$h" ] || continue
        nft delete rule inet "$TABLE" "$CHAIN" handle "$h" 2>/dev/null || true
      done
  fi
  exit 0
fi

if ! command -v iptables >/dev/null 2>&1; then
  echo "iptables or nft is required"
  exit 1
fi

CHAIN="GOVPASS_OUTPUT"

while iptables -t mangle -D OUTPUT -j "$CHAIN" 2>/dev/null; do
  :
done

iptables -t mangle -F "$CHAIN" 2>/dev/null || true
iptables -t mangle -X "$CHAIN" 2>/dev/null || true
