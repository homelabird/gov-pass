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

  nft list table inet "$TABLE" >/dev/null 2>&1 || nft add table inet "$TABLE"
  nft list chain inet "$TABLE" "$CHAIN" >/dev/null 2>&1 || \
    nft add chain inet "$TABLE" "$CHAIN" "{ type filter hook output priority mangle; policy accept; }"

  nft flush chain inet "$TABLE" "$CHAIN"
  nft add rule inet "$TABLE" "$CHAIN" meta mark \& "$MARK" == "$MARK" return
  if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
    nft add rule inet "$TABLE" "$CHAIN" oifname "lo" return
  fi
  nft add rule inet "$TABLE" "$CHAIN" tcp dport 443 queue num "$QUEUE_NUM" bypass
  exit 0
fi

if ! command -v iptables >/dev/null 2>&1; then
  echo "iptables or nft is required"
  exit 1
fi

iptables -t mangle -C OUTPUT -m mark --mark "$MARK"/"$MARK" -j RETURN 2>/dev/null || \
  iptables -t mangle -A OUTPUT -m mark --mark "$MARK"/"$MARK" -j RETURN

if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
  iptables -t mangle -C OUTPUT -o lo -j RETURN 2>/dev/null || \
    iptables -t mangle -A OUTPUT -o lo -j RETURN
fi

iptables -t mangle -C OUTPUT -p tcp --dport 443 -j NFQUEUE --queue-num "$QUEUE_NUM" --queue-bypass 2>/dev/null || \
  iptables -t mangle -A OUTPUT -p tcp --dport 443 -j NFQUEUE --queue-num "$QUEUE_NUM" --queue-bypass
