#!/usr/bin/env bash
set -euo pipefail

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
    awk -v tag="comment \"$TAG\"" '$0 ~ tag { for (i=1;i<=NF;i++) if ($i=="handle") print $(i+1) }' | \
    while read -r h; do
      [ -n "$h" ] || continue
      nft delete rule inet "$TABLE" "$CHAIN" handle "$h" 2>/dev/null || true
    done

  if [ "$MARK" -ne 0 ]; then
    nft add rule inet "$TABLE" "$CHAIN" meta mark \& "$MARK" == "$MARK" return comment "$TAG"
  fi
  if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
    nft add rule inet "$TABLE" "$CHAIN" oifname "lo" return comment "$TAG"
  fi
  for FAMILY in ipv4 ipv6; do
    nft add rule inet "$TABLE" "$CHAIN" meta nfproto "$FAMILY" tcp dport 443 queue num "$QUEUE_NUM" bypass comment "$TAG"
  done
  exit 0
fi

if ! command -v iptables >/dev/null 2>&1 || ! command -v ip6tables >/dev/null 2>&1; then
  echo "iptables+ip6tables or nft is required"
  exit 1
fi

install_family() {
  TOOL="$1"
  CHAIN="$2"

  # Dedicated chains let the helper manage only its own rules and cleanly
  # remove them later without disturbing user-owned OUTPUT rules.
  "$TOOL" -t mangle -N "$CHAIN" 2>/dev/null || true
  "$TOOL" -t mangle -F "$CHAIN"

  "$TOOL" -t mangle -C OUTPUT -j "$CHAIN" 2>/dev/null || \
    "$TOOL" -t mangle -I OUTPUT 1 -j "$CHAIN"

  if [ "$MARK" -ne 0 ]; then
    "$TOOL" -t mangle -A "$CHAIN" -m mark --mark "$MARK"/"$MARK" -j RETURN
  fi
  if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
    "$TOOL" -t mangle -A "$CHAIN" -o lo -j RETURN
  fi
  "$TOOL" -t mangle -A "$CHAIN" -p tcp --dport 443 -j NFQUEUE --queue-num "$QUEUE_NUM" --queue-bypass
}

uninstall_family() {
  TOOL="$1"
  CHAIN="$2"

  while "$TOOL" -t mangle -D OUTPUT -j "$CHAIN" 2>/dev/null; do
    :
  done
  "$TOOL" -t mangle -F "$CHAIN" 2>/dev/null || true
  "$TOOL" -t mangle -X "$CHAIN" 2>/dev/null || true
}

install_family iptables GOVPASS_OUTPUT
if ! install_family ip6tables GOVPASS_OUTPUT6; then
  uninstall_family iptables GOVPASS_OUTPUT || true
  exit 1
fi
