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

if [ "$(id -u)" -ne 0 ]; then
	echo "run as root" >&2
	exit 1
fi

if ! command -v pfctl >/dev/null 2>&1; then
	echo "pfctl not found" >&2
	exit 1
fi
PFCTL_BIN="$(lookup_trusted_command pfctl)" || {
	echo "pfctl not found in trusted command directories" >&2
	exit 1
}

if [ ! -f "${PF_CONF}" ]; then
	echo "pf.conf not found: ${PF_CONF}" >&2
	exit 1
fi

if [ ! -f "${ANCHOR_SOURCE}" ]; then
	echo "anchor source not found: ${ANCHOR_SOURCE}" >&2
	exit 1
fi

install -d "$(dirname -- "${ANCHOR_DEST}")"
install -m 0644 "${ANCHOR_SOURCE}" "${ANCHOR_DEST}"

tmp_conf="$(mktemp)"
trap 'rm -f "${tmp_conf}"' EXIT INT TERM
cp "${PF_CONF}" "${tmp_conf}"

if ! grep -Eq "^[[:space:]]*anchor[[:space:]]+\"${ANCHOR_NAME}\"([[:space:]]|\$)" "${tmp_conf}"; then
	printf '\nanchor "%s"\n' "${ANCHOR_NAME}" >> "${tmp_conf}"
fi
if ! grep -Eq "^[[:space:]]*load[[:space:]]+anchor[[:space:]]+\"${ANCHOR_NAME}\"[[:space:]]+from[[:space:]]+\"${ANCHOR_DEST}\"([[:space:]]|\$)" "${tmp_conf}"; then
	printf 'load anchor "%s" from "%s"\n' "${ANCHOR_NAME}" "${ANCHOR_DEST}" >> "${tmp_conf}"
fi

"${PFCTL_BIN}" -vnf "${tmp_conf}" >/dev/null

if ! cmp -s "${PF_CONF}" "${tmp_conf}"; then
	cp "${PF_CONF}" "${PF_CONF}.gov-pass.bak"
	install -m 0644 "${tmp_conf}" "${PF_CONF}"
fi

if "${PFCTL_BIN}" -s info 2>/dev/null | grep -qi "status: enabled"; then
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
