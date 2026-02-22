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

  nft list table inet "$TABLE" >/dev/null 2>&1 || nft add table inet "$TABLE"
  nft list chain inet "$TABLE" "$CHAIN" >/dev/null 2>&1 || \
    nft add chain inet "$TABLE" "$CHAIN" "{ type filter hook output priority mangle; policy accept; }"

  # Delete only rules we previously installed (tagged), do not flush user rules.
  nft -a list chain inet "$TABLE" "$CHAIN" 2>/dev/null | \
    awk -v tag="comment \\\"$TAG\\\"" '$0 ~ tag { for (i=1;i<=NF;i++) if ($i==\"handle\") print $(i+1) }' | \
    while read -r h; do
      [ -n "$h" ] || continue
      nft delete rule inet "$TABLE" "$CHAIN" handle "$h" 2>/dev/null || true
    done

  nft add rule inet "$TABLE" "$CHAIN" meta mark \& "$MARK" == "$MARK" return comment "$TAG"
  if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
    nft add rule inet "$TABLE" "$CHAIN" oifname "lo" return comment "$TAG"
  fi
  # Restrict to IPv4 only; the splitter currently only handles AF_INET packets.
  nft add rule inet "$TABLE" "$CHAIN" meta nfproto ipv4 tcp dport 443 queue num "$QUEUE_NUM" bypass comment "$TAG"
  exit 0
fi

if ! command -v iptables >/dev/null 2>&1; then
  echo "iptables or nft is required"
  exit 1
fi

CHAIN="GOVPASS_OUTPUT"

# Dedicated chain so we only manage our own rules and can cleanly uninstall.
iptables -t mangle -N "$CHAIN" 2>/dev/null || true
iptables -t mangle -F "$CHAIN"

iptables -t mangle -C OUTPUT -j "$CHAIN" 2>/dev/null || \
  iptables -t mangle -I OUTPUT 1 -j "$CHAIN"

iptables -t mangle -A "$CHAIN" -m mark --mark "$MARK"/"$MARK" -j RETURN
if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
  iptables -t mangle -A "$CHAIN" -o lo -j RETURN
fi
iptables -t mangle -A "$CHAIN" -p tcp --dport 443 -j NFQUEUE --queue-num "$QUEUE_NUM" --queue-bypass
