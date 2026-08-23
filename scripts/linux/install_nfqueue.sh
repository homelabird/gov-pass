#!/usr/bin/env bash
set -euo pipefail

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin"
PATH="${TRUSTED_PATH}"
export PATH

QUEUE_NUM=100
MARK=1
EXCLUDE_LOOPBACK=1

usage() {
  echo "usage: $0 [--queue-num N] [--mark N] [--no-loopback]"
}

require_value() {
  if [ "$#" -lt 2 ]; then
    echo "$1 requires a value"
    usage
    exit 1
  fi
}

lookup_trusted_command() {
  local name="$1"
  local old_ifs="$IFS"
  IFS=:
  for dir in $TRUSTED_PATH; do
    local candidate="${dir}/${name}"
    if [ -L "$candidate" ]; then
      local resolved
      resolved="$(readlink -f -- "$candidate" 2>/dev/null || true)"
      case "$resolved" in
        /usr/local/sbin/*|/usr/local/bin/*|/usr/sbin/*|/usr/bin/*|/sbin/*|/bin/*|/run/current-system/sw/bin/*|/nix/var/nix/profiles/default/bin/*)
          candidate="$resolved"
          ;;
        *)
          continue
          ;;
      esac
    fi
    if [ -f "$candidate" ] && [ -x "$candidate" ]; then
      printf '%s\n' "$candidate"
      IFS="$old_ifs"
      return 0
    fi
  done
  IFS="$old_ifs"
  return 1
}

validate_uint() {
  local label="$1"
  local value="$2"
  local max="$3"
  if ! [[ "$value" =~ ^[0-9]+$ ]]; then
    echo "$label must be a non-negative integer"
    exit 1
  fi
  if (( ${#value} > ${#max} )) || (( 10#$value > max )); then
    echo "$label must be <= $max"
    exit 1
  fi
}

validate_nft_handle() {
  local value="$1"
  case "$value" in
    ''|*[!0-9]*)
      return 1
      ;;
  esac
  while [ "${value#0}" != "$value" ]; do
    value="${value#0}"
  done
  [ -n "$value" ]
}

while [ $# -gt 0 ]; do
  case "$1" in
    --queue-num)
      require_value "$@"
      QUEUE_NUM="$2"
      shift 2
      ;;
    --mark)
      require_value "$@"
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

validate_uint "--queue-num" "$QUEUE_NUM" 65535
validate_uint "--mark" "$MARK" 4294967295

ID_BIN="$(lookup_trusted_command id)" || {
  echo "id not found in trusted command directories"
  exit 1
}

if [ "$("$ID_BIN" -u)" -ne 0 ]; then
  echo "root required"
  exit 1
fi

if NFT_BIN="$(lookup_trusted_command nft)"; then
  AWK_BIN="$(lookup_trusted_command awk)" || {
    echo "awk not found in trusted command directories"
    exit 1
  }
  TABLE="gov_pass"
  CHAIN="output"
  TAG="gov-pass"

  "$NFT_BIN" list table inet "$TABLE" >/dev/null 2>&1 || "$NFT_BIN" add table inet "$TABLE"
  "$NFT_BIN" list chain inet "$TABLE" "$CHAIN" >/dev/null 2>&1 || \
    "$NFT_BIN" add chain inet "$TABLE" "$CHAIN" "{ type filter hook output priority mangle; policy accept; }"

  # Delete only rules we previously installed (tagged), do not flush user rules.
  "$NFT_BIN" -a list chain inet "$TABLE" "$CHAIN" 2>/dev/null | \
    "$AWK_BIN" -v tag="comment \"$TAG\"" '$0 ~ tag { for (i=1;i<=NF;i++) if ($i=="handle") print $(i+1) }' | \
    while read -r h; do
      [ -n "$h" ] || continue
      validate_nft_handle "$h" || continue
      "$NFT_BIN" delete rule inet "$TABLE" "$CHAIN" handle "$h" 2>/dev/null || true
    done

  if [ "$MARK" -ne 0 ]; then
    "$NFT_BIN" add rule inet "$TABLE" "$CHAIN" meta mark \& "$MARK" == "$MARK" return comment "$TAG"
  fi
  if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
    "$NFT_BIN" add rule inet "$TABLE" "$CHAIN" oifname "lo" return comment "$TAG"
  fi
  for FAMILY in ipv4 ipv6; do
    "$NFT_BIN" add rule inet "$TABLE" "$CHAIN" meta nfproto "$FAMILY" tcp dport 443 queue num "$QUEUE_NUM" bypass comment "$TAG"
  done
  exit 0
fi

IPTABLES_BIN="$(lookup_trusted_command iptables || true)"
IP6TABLES_BIN="$(lookup_trusted_command ip6tables || true)"
if [ -z "$IPTABLES_BIN" ] || [ -z "$IP6TABLES_BIN" ]; then
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

install_family "$IPTABLES_BIN" GOVPASS_OUTPUT
if ! install_family "$IP6TABLES_BIN" GOVPASS_OUTPUT6; then
  uninstall_family "$IPTABLES_BIN" GOVPASS_OUTPUT || true
  exit 1
fi
