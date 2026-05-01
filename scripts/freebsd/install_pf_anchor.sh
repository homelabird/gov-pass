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
	printf '%s\n' \
		"usage: install_pf_anchor.sh [--source PATH] [--anchor NAME] [--anchor-dir DIR] [--pf-conf PATH] [--no-reload]" \
		"Anchor names may contain only letters, numbers, underscore, and hyphen."
}

need_arg() {
	if [ "$#" -lt 2 ] || [ -z "$2" ]; then
		echo "$1 requires a value" >&2
		usage >&2
		exit 1
	fi
}

validate_anchor_name() {
	case "$ANCHOR_NAME" in
		""|*[!A-Za-z0-9_-]*)
			echo "invalid anchor name: $ANCHOR_NAME" >&2
			exit 1
			;;
	esac
}

validate_absolute_path() {
	name="$1"
	value="$2"
	case "$value" in
		""|*[!A-Za-z0-9_./:@+-]*|*"/../"*|*/..|../*|..)
			echo "invalid ${name}: ${value}" >&2
			exit 1
			;;
		/*)
			;;
		*)
			echo "${name} must be an absolute path: ${value}" >&2
			exit 1
			;;
	esac
}

lookup_trusted_command() {
	name="$1"
	old_ifs="$IFS"
	IFS=:
	for dir in $TRUSTED_PATH; do
		candidate="${dir}/${name}"
		if [ -L "$candidate" ]; then
			echo "refusing symlinked trusted command for ${name}: ${candidate}" >&2
			IFS="$old_ifs"
			exit 1
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

while [ $# -gt 0 ]; do
	case "$1" in
		--source)
			need_arg "$@"
			SOURCE_PATH="$2"
			shift 2
			;;
		--anchor)
			need_arg "$@"
			ANCHOR_NAME="$2"
			shift 2
			;;
		--anchor-dir)
			need_arg "$@"
			ANCHOR_DIR="$2"
			shift 2
			;;
		--pf-conf)
			need_arg "$@"
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

validate_anchor_name
validate_absolute_path "SOURCE_PATH" "$SOURCE_PATH"
validate_absolute_path "ANCHOR_DIR" "$ANCHOR_DIR"
validate_absolute_path "PF_CONF" "$PF_CONF"

ID_BIN="$(lookup_trusted_command id)" || {
	echo "id not found in trusted command directories" >&2
	exit 1
}
PFCTL_BIN="$(lookup_trusted_command pfctl)" || {
	echo "pfctl not found in trusted command directories" >&2
	exit 1
}
MKTEMP_BIN="$(lookup_trusted_command mktemp)" || {
	echo "mktemp not found in trusted command directories" >&2
	exit 1
}
RM_BIN="$(lookup_trusted_command rm)" || {
	echo "rm not found in trusted command directories" >&2
	exit 1
}
INSTALL_BIN="$(lookup_trusted_command install)" || {
	echo "install not found in trusted command directories" >&2
	exit 1
}
CP_BIN="$(lookup_trusted_command cp)" || {
	echo "cp not found in trusted command directories" >&2
	exit 1
}
AWK_BIN="$(lookup_trusted_command awk)" || {
	echo "awk not found in trusted command directories" >&2
	exit 1
}
MV_BIN="$(lookup_trusted_command mv)" || {
	echo "mv not found in trusted command directories" >&2
	exit 1
}
GREP_BIN="$(lookup_trusted_command grep)" || {
	echo "grep not found in trusted command directories" >&2
	exit 1
}
CMP_BIN="$(lookup_trusted_command cmp)" || {
	echo "cmp not found in trusted command directories" >&2
	exit 1
}

if [ "$("$ID_BIN" -u)" -ne 0 ]; then
	echo "run as root"
	exit 1
fi

if [ ! -f "$PF_CONF" ]; then
	echo "pf.conf not found: $PF_CONF"
	exit 1
fi

if [ ! -f "$SOURCE_PATH" ]; then
	echo "anchor source not found: $SOURCE_PATH"
	exit 1
fi

ANCHOR_PATH="${ANCHOR_DIR}/${ANCHOR_NAME}"
if [ -L "$ANCHOR_PATH" ]; then
	echo "refusing to overwrite symlink anchor destination: $ANCHOR_PATH"
	exit 1
fi
TMP_CONF="$("$MKTEMP_BIN")"
TMP_STRIPPED="$("$MKTEMP_BIN")"
cleanup() {
	"$RM_BIN" -f "$TMP_CONF" "$TMP_STRIPPED"
}
trap cleanup EXIT INT TERM

"$INSTALL_BIN" -d "$ANCHOR_DIR"
"$INSTALL_BIN" -m 0644 "$SOURCE_PATH" "$ANCHOR_PATH"
"$CP_BIN" "$PF_CONF" "$TMP_CONF"

"$AWK_BIN" -v begin="$MANAGED_BEGIN" -v end="$MANAGED_END" '
	$0 == begin { skip = 1; next }
	$0 == end { skip = 0; next }
	!skip { print }
' "$TMP_CONF" >"$TMP_STRIPPED"
"$MV_BIN" "$TMP_STRIPPED" "$TMP_CONF"

need_anchor_line=1
need_load_line=1
if "$GREP_BIN" -Eq "^[[:space:]]*anchor[[:space:]]+\"${ANCHOR_NAME}\"([[:space:]]|$)" "$TMP_CONF"; then
	need_anchor_line=0
fi
if "$GREP_BIN" -Eq "^[[:space:]]*load[[:space:]]+anchor[[:space:]]+\"${ANCHOR_NAME}\"[[:space:]]+from[[:space:]]+\"${ANCHOR_PATH}\"([[:space:]]|$)" "$TMP_CONF"; then
	need_load_line=0
fi

if [ "$need_anchor_line" -eq 1 ] || [ "$need_load_line" -eq 1 ]; then
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
if ! "$CMP_BIN" -s "$TMP_CONF" "$PF_CONF"; then
	"$CP_BIN" "$TMP_CONF" "$PF_CONF"
fi

if [ "$RELOAD_PF" -eq 1 ]; then
	"${PFCTL_BIN}" -f "$PF_CONF" >/dev/null
fi

echo "Installed pf anchor: ${ANCHOR_PATH}"
echo "pf.conf load path ready for anchor ${ANCHOR_NAME}"
