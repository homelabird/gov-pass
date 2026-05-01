#!/bin/sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="${TRUSTED_PATH}"
export PATH

ANCHOR_NAME="${ANCHOR_NAME:-gov-pass}"
PF_CONF="${PF_CONF:-/etc/pf.conf}"
ANCHOR_SOURCE="${ANCHOR_SOURCE:-/usr/local/etc/gov-pass/pf.anchor.conf}"
ANCHOR_DEST="${ANCHOR_DEST:-/etc/pf.anchors/${ANCHOR_NAME}}"
ENABLE_PF="${ENABLE_PF:-0}"

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

validate_anchor_name
validate_absolute_path "PF_CONF" "$PF_CONF"
validate_absolute_path "ANCHOR_SOURCE" "$ANCHOR_SOURCE"
validate_absolute_path "ANCHOR_DEST" "$ANCHOR_DEST"
case "$ANCHOR_DEST" in
	*/"$ANCHOR_NAME")
		;;
	*)
		echo "ANCHOR_DEST must end with /${ANCHOR_NAME}: ${ANCHOR_DEST}" >&2
		exit 1
	;;
esac

ID_BIN="$(lookup_trusted_command id)" || {
	echo "id not found in trusted command directories" >&2
	exit 1
}
PFCTL_BIN="$(lookup_trusted_command pfctl)" || {
	echo "pfctl not found in trusted command directories" >&2
	exit 1
}
INSTALL_BIN="$(lookup_trusted_command install)" || {
	echo "install not found in trusted command directories" >&2
	exit 1
}
DIRNAME_BIN="$(lookup_trusted_command dirname)" || {
	echo "dirname not found in trusted command directories" >&2
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
CP_BIN="$(lookup_trusted_command cp)" || {
	echo "cp not found in trusted command directories" >&2
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
	echo "run as root" >&2
	exit 1
fi

if [ ! -f "${PF_CONF}" ]; then
	echo "pf.conf not found: ${PF_CONF}" >&2
	exit 1
fi

if [ ! -f "${ANCHOR_SOURCE}" ]; then
	echo "anchor source not found: ${ANCHOR_SOURCE}" >&2
	exit 1
fi

if [ -L "${ANCHOR_DEST}" ]; then
	echo "refusing to overwrite symlink anchor destination: ${ANCHOR_DEST}" >&2
	exit 1
fi

"$INSTALL_BIN" -d "$("$DIRNAME_BIN" -- "${ANCHOR_DEST}")"
"$INSTALL_BIN" -m 0644 "${ANCHOR_SOURCE}" "${ANCHOR_DEST}"

tmp_conf="$("$MKTEMP_BIN")"
trap '"${RM_BIN}" -f "${tmp_conf}"' EXIT INT TERM
"$CP_BIN" "${PF_CONF}" "${tmp_conf}"

if ! "$GREP_BIN" -Eq "^[[:space:]]*anchor[[:space:]]+\"${ANCHOR_NAME}\"([[:space:]]|\$)" "${tmp_conf}"; then
	printf '\nanchor "%s"\n' "${ANCHOR_NAME}" >> "${tmp_conf}"
fi
if ! "$GREP_BIN" -Eq "^[[:space:]]*load[[:space:]]+anchor[[:space:]]+\"${ANCHOR_NAME}\"[[:space:]]+from[[:space:]]+\"${ANCHOR_DEST}\"([[:space:]]|\$)" "${tmp_conf}"; then
	printf 'load anchor "%s" from "%s"\n' "${ANCHOR_NAME}" "${ANCHOR_DEST}" >> "${tmp_conf}"
fi

"${PFCTL_BIN}" -vnf "${tmp_conf}" >/dev/null

if ! "$CMP_BIN" -s "${PF_CONF}" "${tmp_conf}"; then
	"$CP_BIN" "${PF_CONF}" "${PF_CONF}.gov-pass.bak"
	"$INSTALL_BIN" -m 0644 "${tmp_conf}" "${PF_CONF}"
fi

if "${PFCTL_BIN}" -s info 2>/dev/null | "$GREP_BIN" -qi "status: enabled"; then
	"${PFCTL_BIN}" -a "${ANCHOR_NAME}" -f "${ANCHOR_DEST}"
elif [ "${ENABLE_PF}" = "1" ]; then
	"${PFCTL_BIN}" -e >/dev/null 2>&1 || true
	"${PFCTL_BIN}" -f "${PF_CONF}"
else
	echo "pf.conf updated, but pf is not enabled; enable pf or rerun with ENABLE_PF=1" >&2
fi

echo "Installed anchor: ${ANCHOR_DEST}"
echo "Anchor source: ${ANCHOR_SOURCE}"
echo "Verify with: pfctl -a ${ANCHOR_NAME} -s rules"
