#!/bin/sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="${TRUSTED_PATH}"
export PATH

ANCHOR_NAME="${ANCHOR_NAME:-gov-pass}"
PF_CONF="${PF_CONF:-/etc/pf.conf}"
ANCHOR_DEST="${ANCHOR_DEST:-/etc/pf.anchors/${ANCHOR_NAME}}"

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

tmp_conf="$(mktemp)"
trap 'rm -f "${tmp_conf}"' EXIT INT TERM

awk -v anchor_name="${ANCHOR_NAME}" -v anchor_dest="${ANCHOR_DEST}" '
  $0 ~ "^[[:space:]]*anchor[[:space:]]+\"" anchor_name "\"([[:space:]]|$)" { next }
  $0 ~ "^[[:space:]]*load[[:space:]]+anchor[[:space:]]+\"" anchor_name "\"[[:space:]]+from[[:space:]]+\"" anchor_dest "\"([[:space:]]|$)" { next }
  { print }
' "${PF_CONF}" > "${tmp_conf}"

"${PFCTL_BIN}" -vnf "${tmp_conf}" >/dev/null

if ! cmp -s "${PF_CONF}" "${tmp_conf}"; then
	cp "${PF_CONF}" "${PF_CONF}.gov-pass.bak"
	install -m 0644 "${tmp_conf}" "${PF_CONF}"
fi

if "${PFCTL_BIN}" -s info 2>/dev/null | grep -qi "status: enabled"; then
	"${PFCTL_BIN}" -a "${ANCHOR_NAME}" -F all >/dev/null 2>&1 || true
	"${PFCTL_BIN}" -f "${PF_CONF}"
fi

rm -f "${ANCHOR_DEST}"

echo "Removed anchor: ${ANCHOR_DEST}"
echo "Verify with: pfctl -a ${ANCHOR_NAME} -s rules"
