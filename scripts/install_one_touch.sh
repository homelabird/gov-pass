#!/usr/bin/env bash
set -euo pipefail

TRUSTED_PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin"
PATH="${TRUSTED_PATH}"
export PATH

lookup_trusted_command() {
  local name="$1"
  local old_ifs="$IFS"
  IFS=:
  for dir in $TRUSTED_PATH; do
    local candidate="${dir}/${name}"
    if [ -L "$candidate" ]; then
      continue
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

validate_explicit_tool_path() {
  local name="$1"
  local value="$2"
  case "$value" in
    /*) ;;
    *)
      echo "$name override must be an absolute path: $value" >&2
      return 1
      ;;
  esac
  if [ -L "$value" ]; then
    echo "refusing symlinked $name override: $value" >&2
    return 1
  fi
  if [ ! -f "$value" ] || [ ! -x "$value" ]; then
    echo "$name override is not executable: $value" >&2
    return 1
  fi
  printf '%s\n' "$value"
}

lookup_go_command() {
  if [ -n "${GOV_PASS_GO_BIN:-}" ]; then
    validate_explicit_tool_path go "$GOV_PASS_GO_BIN"
    return
  fi
  lookup_trusted_command go
}

script_path="${BASH_SOURCE[0]}"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir="." ;;
esac
ROOT_DIR="$(cd -- "${script_dir}/.." && pwd)"

GO_BIN="$(lookup_go_command)" || {
  echo "go is required — install from https://go.dev/dl/"
  exit 1
}
UNAME_BIN="$(lookup_trusted_command uname)" || {
  echo "uname not found in trusted command directories"
  exit 1
}
ID_BIN="$(lookup_trusted_command id)" || {
  echo "id not found in trusted command directories"
  exit 1
}
INSTALL_BIN="$(lookup_trusted_command install)" || {
  echo "install not found in trusted command directories"
  exit 1
}

OS="$("${UNAME_BIN}" -s)"

if [ "$OS" != "Linux" ] && [ "$OS" != "FreeBSD" ]; then
  echo "unsupported OS: $OS"
  exit 1
fi

if [ -z "${INSTALL_TUI+x}" ]; then
  INSTALL_TUI=0
  [ "$OS" = "Linux" ] && INSTALL_TUI=1
fi

if [ "$("${ID_BIN}" -u)" -ne 0 ]; then
  echo "run as root (e.g. sudo ./scripts/install_one_touch.sh)"
  exit 1
fi

cd "$ROOT_DIR"

if [ "$OS" = "Linux" ]; then
  MAKE_BIN="$(lookup_trusted_command make)" || {
    echo "make not found in trusted command directories"
    exit 1
  }
  "${MAKE_BIN}" GO="${GO_BIN}" install
  if [ "$INSTALL_TUI" = "1" ]; then
    "${MAKE_BIN}" GO="${GO_BIN}" install-tui
  fi
  if SYSTEMCTL_BIN="$(lookup_trusted_command systemctl)"; then
    "${SYSTEMCTL_BIN}" daemon-reload
    "${SYSTEMCTL_BIN}" enable --now gov-pass
  fi
  echo "Installed on Linux: /opt/gov-pass/dist/splitter"
  echo "Linux service defaults keep --auto-install-tools=false --auto-offload=false in /etc/default/gov-pass."
  echo "On multi-egress, VPN, or container hosts, set --iface explicitly in /etc/default/gov-pass."
  if [ "$INSTALL_TUI" = "1" ]; then
    echo "Installed on Linux: /usr/local/bin/gov-pass-tui (TUI controller)"
    echo "Run: gov-pass-tui"
  fi
  exit 0
fi

"${GO_BIN}" build -o dist/splitter ./cmd/splitter
"${INSTALL_BIN}" -d /usr/local/sbin
"${INSTALL_BIN}" -d /usr/local/etc/gov-pass
"${INSTALL_BIN}" -d /usr/local/etc/rc.d
"${INSTALL_BIN}" -d /usr/local/libexec/gov-pass
"${INSTALL_BIN}" -m 0755 dist/splitter /usr/local/sbin/splitter
if [ "$INSTALL_TUI" = "1" ]; then
  "${GO_BIN}" build -o dist/gov-pass-tui ./cmd/gov-pass-tui
  "${INSTALL_BIN}" -m 0755 dist/gov-pass-tui /usr/local/sbin/gov-pass-tui
fi
"${INSTALL_BIN}" -m 0755 scripts/freebsd/gov-pass /usr/local/etc/rc.d/gov-pass
"${INSTALL_BIN}" -m 0755 scripts/freebsd/install_pf_anchor.sh /usr/local/libexec/gov-pass/install_pf_anchor.sh
"${INSTALL_BIN}" -m 0755 scripts/freebsd/uninstall_pf_anchor.sh /usr/local/libexec/gov-pass/uninstall_pf_anchor.sh
"${INSTALL_BIN}" -m 0755 scripts/freebsd/gov-pass-apply-pf.sh /usr/local/libexec/gov-pass/gov-pass-apply-pf.sh
"${INSTALL_BIN}" -m 0755 scripts/freebsd/gov-pass-remove-pf.sh /usr/local/libexec/gov-pass/gov-pass-remove-pf.sh
if [ ! -f /usr/local/etc/gov-pass/config.json ]; then
  "${INSTALL_BIN}" -m 0644 docs/examples/splitter.freebsd.json /usr/local/etc/gov-pass/config.json
fi
if [ ! -f /usr/local/etc/gov-pass/pf.anchor.conf ]; then
  "${INSTALL_BIN}" -m 0644 docs/pf/gov-pass.anchor.wan.conf /usr/local/etc/gov-pass/pf.anchor.conf
fi

echo "Installed on FreeBSD: /usr/local/sbin/splitter"
if [ "$INSTALL_TUI" = "1" ]; then
  echo "Installed on FreeBSD: /usr/local/sbin/gov-pass-tui (TUI controller)"
fi
echo "Installed FreeBSD service: /usr/local/etc/rc.d/gov-pass"
echo "Installed PF helpers: /usr/local/libexec/gov-pass/install_pf_anchor.sh and /usr/local/libexec/gov-pass/uninstall_pf_anchor.sh"
echo "Installed PF service helpers: /usr/local/libexec/gov-pass/gov-pass-apply-pf.sh and /usr/local/libexec/gov-pass/gov-pass-remove-pf.sh"
echo "Edit /usr/local/etc/gov-pass/pf.anchor.conf, then apply with:"
echo "  /usr/local/libexec/gov-pass/install_pf_anchor.sh"
echo "Config path: /usr/local/etc/gov-pass/config.json"
echo "Enable service with:"
echo "  sysrc gov_pass_enable=YES && service gov-pass start"
