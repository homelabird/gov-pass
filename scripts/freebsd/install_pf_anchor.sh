#!/usr/bin/env sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="${TRUSTED_PATH}"
export PATH

ANCHOR_NAME="${ANCHOR_NAME:-gov-pass}"
SOURCE_PATH="${SOURCE_PATH:-/usr/local/etc/gov-pass/pf.anchor.conf}"
ANCHOR_DIR="${ANCHOR_DIR:-/etc/pf.anchors}"
PF_CONF="${PF_CONF:-/etc/pf.conf}"
RELOAD_PF=1

MANAGED_BEGIN="# gov-pass managed block begin"
MANAGED_END="# gov-pass managed block end"

usage() {
	cat <<'EOF'
usage: install_pf_anchor.sh [--source PATH] [--anchor NAME] [--anchor-dir DIR] [--pf-conf PATH] [--no-reload]
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
		--source)
			SOURCE_PATH="$2"
			shift 2
			;;
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

if ! command -v pfctl >/dev/null 2>&1; then
	echo "pfctl not found in PATH"
	exit 1
fi
PFCTL_BIN="$(lookup_trusted_command pfctl)" || {
	echo "pfctl not found in trusted command directories"
	exit 1
}

if [ ! -f "$PF_CONF" ]; then
	echo "pf.conf not found: $PF_CONF"
	exit 1
fi

if [ ! -f "$SOURCE_PATH" ]; then
	echo "anchor source not found: $SOURCE_PATH"
	exit 1
fi

ANCHOR_PATH="${ANCHOR_DIR}/${ANCHOR_NAME}"
TMP_CONF="$(mktemp)"
cleanup() {
	rm -f "$TMP_CONF"
}
trap cleanup EXIT INT TERM

install -d "$ANCHOR_DIR"
install -m 0644 "$SOURCE_PATH" "$ANCHOR_PATH"
cp "$PF_CONF" "$TMP_CONF"

need_anchor_line=1
need_load_line=1
if grep -Fq "anchor \"${ANCHOR_NAME}\"" "$TMP_CONF"; then
	need_anchor_line=0
fi
if grep -Fq "load anchor \"${ANCHOR_NAME}\" from \"${ANCHOR_PATH}\"" "$TMP_CONF"; then
	need_load_line=0
fi

if ! grep -Fq "$MANAGED_BEGIN" "$TMP_CONF" && { [ "$need_anchor_line" -eq 1 ] || [ "$need_load_line" -eq 1 ]; }; then
	{
		echo
		echo "$MANAGED_BEGIN"
		if [ "$need_anchor_line" -eq 1 ]; then
			printf 'anchor "%s"\n' "$ANCHOR_NAME"
		fi
		if [ "$need_load_line" -eq 1 ]; then
			printf 'load anchor "%s" from "%s"\n' "$ANCHOR_NAME" "$ANCHOR_PATH"
		fi
		echo "$MANAGED_END"
	} >>"$TMP_CONF"
fi

"${PFCTL_BIN}" -nf "$TMP_CONF" >/dev/null
if ! cmp -s "$TMP_CONF" "$PF_CONF"; then
	cp "$TMP_CONF" "$PF_CONF"
fi

if [ "$RELOAD_PF" -eq 1 ]; then
	"${PFCTL_BIN}" -f "$PF_CONF" >/dev/null
fi

echo "Installed pf anchor: ${ANCHOR_PATH}"
echo "pf.conf load path ready for anchor ${ANCHOR_NAME}"
