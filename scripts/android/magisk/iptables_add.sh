#!/system/bin/sh
set -eu

TRUSTED_PATH="/system/bin:/system/xbin:/vendor/bin:/sbin:/data/adb/magisk:/data/adb/ksu/bin:/data/adb/ap/bin"
PATH="$TRUSTED_PATH"
export PATH

QUEUE_NUM=100
MARK=1
EXCLUDE_LOOPBACK=1

usage() {
  echo "usage: $0 [--queue-num N] [--mark N] [--no-loopback]"
}

require_value() {
  if [ "$#" -lt 2 ]; then
    echo "$1 requires a value" >&2
    usage
    exit 1
  fi
}

validate_uint() {
	name="$1"
	value="$2"
	max="$3"
	case "$value" in
    ""|*[!0-9]*)
      echo "$name must be a decimal integer" >&2
			exit 1
			;;
	esac
	while [ "${value#0}" != "$value" ]; do
		value="${value#0}"
	done
	if [ -z "$value" ]; then
		value=0
	fi
	if [ "${#value}" -gt "${#max}" ] || { [ "${#value}" -eq "${#max}" ] && [ "$value" \> "$max" ]; }; then
		echo "$name out of range: $value" >&2
		exit 1
	fi
}

resolve_iptables_command() {
  value="${IPTABLES:-iptables}"
  case "$value" in
    ""|-*)
      echo "IPTABLES must be a command name or absolute path" >&2
      exit 1
      ;;
    */*)
      case "$value" in
        /*) ;;
        *)
          echo "IPTABLES path must be absolute: $value" >&2
          exit 1
          ;;
      esac
      if [ -L "$value" ]; then
        echo "refusing symlinked IPTABLES path: $value" >&2
        exit 1
      fi
      if [ ! -x "$value" ] || [ -d "$value" ]; then
        echo "IPTABLES path is not executable: $value" >&2
        exit 1
      fi
      printf '%s\n' "$value"
      return 0
      ;;
    *[!A-Za-z0-9_.+-]*)
      echo "IPTABLES command contains unsupported characters: $value" >&2
      exit 1
      ;;
  esac
  candidate="$(PATH="$TRUSTED_PATH" command -v "$value" 2>/dev/null || true)"
  if [ -z "$candidate" ]; then
    echo "iptables not found" >&2
    exit 1
  fi
  case "$candidate" in
    */*) ;;
    *)
      echo "refusing non-path iptables lookup: $candidate" >&2
      exit 1
      ;;
  esac
  if [ ! -x "$candidate" ] || [ -d "$candidate" ]; then
    echo "iptables command is not executable: $candidate" >&2
    exit 1
  fi
  printf '%s\n' "$candidate"
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

IPTABLES="$(resolve_iptables_command)"

"$IPTABLES" -t mangle -C OUTPUT -m mark --mark "$MARK"/"$MARK" -j RETURN 2>/dev/null || \
  "$IPTABLES" -t mangle -A OUTPUT -m mark --mark "$MARK"/"$MARK" -j RETURN

if [ "$EXCLUDE_LOOPBACK" -eq 1 ]; then
  "$IPTABLES" -t mangle -C OUTPUT -o lo -j RETURN 2>/dev/null || \
    "$IPTABLES" -t mangle -A OUTPUT -o lo -j RETURN
fi

"$IPTABLES" -t mangle -C OUTPUT -p tcp --dport 443 -j NFQUEUE --queue-num "$QUEUE_NUM" --queue-bypass 2>/dev/null || \
  "$IPTABLES" -t mangle -A OUTPUT -p tcp --dport 443 -j NFQUEUE --queue-num "$QUEUE_NUM" --queue-bypass
