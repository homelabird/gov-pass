#!/system/bin/sh
set -eu

TRUSTED_PATH="/system/bin:/system/xbin:/vendor/bin:/sbin:/data/adb/magisk:/data/adb/ksu/bin:/data/adb/ap/bin"
PATH="$TRUSTED_PATH"
export PATH

if [ -n "${MODPATH:-}" ]; then
  MODDIR="$MODPATH"
else
  case "$0" in
    */*) MODDIR=${0%/*} ;;
    *) MODDIR="." ;;
  esac
fi
PIDFILE="/data/adb/gov-pass.pid"
CONFIG="/data/adb/gov-pass.conf"
QUEUE_NUM=100
MARK=1

lookup_trusted_command() {
  name="$1"
  candidate="$(PATH="$TRUSTED_PATH" command -v "$name" 2>/dev/null || true)"
  if [ -z "$candidate" ]; then
    echo "$name is required" >&2
    exit 1
  fi
  case "$candidate" in
    */*) ;;
    *)
      echo "refusing non-path command lookup for $name: $candidate" >&2
      exit 1
      ;;
  esac
  if [ ! -x "$candidate" ] || [ -d "$candidate" ]; then
    echo "trusted command is not executable: $candidate" >&2
    exit 1
  fi
  printf '%s\n' "$candidate"
}

load_config() {
	if [ ! -f "$CONFIG" ]; then
		return 0
	fi
	while IFS= read -r line || [ -n "$line" ]; do
		case "$line" in
			""|\#*)
				continue
				;;
			*=*)
				key=${line%%=*}
				value=${line#*=}
				;;
			*)
				echo "invalid config line: $line" >&2
				exit 1
				;;
		esac
		case "$key" in
			QUEUE_NUM)
				QUEUE_NUM="$value"
				;;
			MARK)
				MARK="$value"
				;;
			*)
				echo "unsupported config key: $key" >&2
				exit 1
				;;
		esac
	done < "$CONFIG"
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

load_config
validate_uint "QUEUE_NUM" "$QUEUE_NUM" 65535
validate_uint "MARK" "$MARK" 4294967295

validate_pid() {
	value="$1"
	validate_uint "PID" "$value" 4194304
	while [ "${value#0}" != "$value" ]; do
		value="${value#0}"
	done
	if [ -z "$value" ] || [ "$value" = "0" ]; then
		echo "PID must be a positive integer" >&2
		exit 1
	fi
}

CAT_BIN="$(lookup_trusted_command cat)"
RM_BIN="$(lookup_trusted_command rm)"

"$MODDIR/iptables_del.sh" --queue-num "$QUEUE_NUM" --mark "$MARK" >/dev/null 2>&1 || true

if [ -f "$PIDFILE" ]; then
  pid="$("$CAT_BIN" "$PIDFILE")"
  validate_pid "$pid"
  kill "$pid" >/dev/null 2>&1 || true
  "$RM_BIN" -f "$PIDFILE"
fi
