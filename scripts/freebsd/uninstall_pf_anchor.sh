#!/usr/bin/env sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="${TRUSTED_PATH}"
export PATH

ANCHOR_NAME="${ANCHOR_NAME:-gov-pass}"
ANCHOR_DIR="${ANCHOR_DIR:-/etc/pf.anchors}"
PF_CONF="${PF_CONF:-/etc/pf.conf}"
RELOAD_PF=1

MANAGED_BEGIN="# gov-pass managed block begin"
MANAGED_END="# gov-pass managed block end"

usage() {
	cat <<'EOF'
usage: uninstall_pf_anchor.sh [--anchor NAME] [--anchor-dir DIR] [--pf-conf PATH] [--no-reload]
EOF
}

lookup_trusted_command() {
	name="$1"
	old_ifs="$IFS"
	IFS=:
	for dir in $TRUSTED_PATH; do
		candidate="${dir}/${name}"
		if [ -f "$candidate" ] && [ -x "$candidate" ]; then
			printf '%s\n' "$candidate"
			IFS="$old_ifs"
			return 0
		fi
	done
	IFS="$old_ifs"
	return 1
}

while [ $# -gt 0 ]; do
	case "$1" in
		--anchor)
			ANCHOR_NAME="$2"
			shift 2
			;;
		--anchor-dir)
			ANCHOR_DIR="$2"
			shift 2
			;;
		--pf-conf)
			PF_CONF="$2"
			shift 2
			;;
		--no-reload)
			RELOAD_PF=0
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "unknown argument: $1" >&2
			usage >&2
			exit 1
			;;
	esac
done

if [ "$(id -u)" -ne 0 ]; then
	echo "run as root"
	exit 1
fi

if [ ! -f "$PF_CONF" ]; then
	echo "pf.conf not found: $PF_CONF"
	exit 1
fi

if ! command -v pfctl >/dev/null 2>&1; then
	echo "pfctl not found in PATH"
	exit 1
fi
PFCTL_BIN="$(lookup_trusted_command pfctl)" || {
	echo "pfctl not found in trusted command directories"
	exit 1
}

ANCHOR_PATH="${ANCHOR_DIR}/${ANCHOR_NAME}"
TMP_CONF="$(mktemp)"
cleanup() {
	rm -f "$TMP_CONF"
}
trap cleanup EXIT INT TERM

awk -v begin="$MANAGED_BEGIN" -v end="$MANAGED_END" '
	$0 == begin { skip = 1; next }
	$0 == end { skip = 0; next }
	!skip { print }
' "$PF_CONF" >"$TMP_CONF"

"${PFCTL_BIN}" -nf "$TMP_CONF" >/dev/null
if ! cmp -s "$TMP_CONF" "$PF_CONF"; then
	cp "$TMP_CONF" "$PF_CONF"
fi

if ! grep -Fq "load anchor \"${ANCHOR_NAME}\" from \"${ANCHOR_PATH}\"" "$TMP_CONF"; then
	rm -f "$ANCHOR_PATH"
fi

if [ "$RELOAD_PF" -eq 1 ]; then
	"${PFCTL_BIN}" -f "$PF_CONF" >/dev/null
fi

echo "Removed managed pf anchor block for ${ANCHOR_NAME}"
