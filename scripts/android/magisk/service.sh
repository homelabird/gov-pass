#!/system/bin/sh
set -eu

TRUSTED_PATH="/system/bin:/system/xbin:/vendor/bin:/sbin:/data/adb/magisk:/data/adb/ksu/bin:/data/adb/ap/bin"
PATH="$TRUSTED_PATH"
export PATH

case "$0" in
  */*) MODDIR=${0%/*} ;;
  *) MODDIR="." ;;
esac
CONFIG="/data/adb/gov-pass.conf"
PIDFILE="/data/adb/gov-pass.pid"
LOGFILE="/data/adb/gov-pass.log"

QUEUE_NUM=100
MARK=1

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

if [ -d "$MODDIR/lib" ]; then
  export LD_LIBRARY_PATH="$MODDIR/lib:${LD_LIBRARY_PATH:-}"
fi

"$MODDIR/iptables_add.sh" --queue-num "$QUEUE_NUM" --mark "$MARK" >>"$LOGFILE" 2>&1

"$MODDIR/splitter" --queue-num "$QUEUE_NUM" --mark "$MARK" >>"$LOGFILE" 2>&1 &
echo $! > "$PIDFILE"
