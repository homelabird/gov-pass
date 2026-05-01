#!/usr/bin/env bash
set -euo pipefail

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin"
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

REPO_OWNER="${REPO_OWNER:-homelabird}"
REPO_NAME="${REPO_NAME:-gov-pass}"
VERSION="${GOV_PASS_VERSION:-}"
NO_START="${NO_START:-0}"
INSTALL_TUI="${INSTALL_TUI:-0}"
RELEASE_PUBKEY_PATH="${GOV_PASS_RELEASE_PUBKEY_PATH:-}"
RELEASE_PUBKEY_PEM="${GOV_PASS_RELEASE_PUBKEY_PEM:-}"
RELEASE_PUBKEY_PEM_B64="${GOV_PASS_RELEASE_PUBKEY_PEM_B64:-}"

validate_id_component() {
  local label="$1"
  local value="$2"
  if [[ -z "$value" || "$value" == .* || "$value" == *"/"* || "$value" == *"\\"* ]]; then
    echo "Invalid $label: $value"
    exit 1
  fi
  if ! [[ "$value" =~ ^[A-Za-z0-9_.-]+$ ]]; then
    echo "Invalid $label: $value"
    exit 1
  fi
}

validate_release_version() {
  local value="$1"
  if [[ -z "$value" || "$value" == .* || "$value" == *"/"* || "$value" == *"\\"* ]]; then
    echo "Invalid GOV_PASS_VERSION: $value"
    exit 1
  fi
  if ! [[ "$value" =~ ^v?[0-9][0-9A-Za-z._+-]*$ ]]; then
    echo "Invalid GOV_PASS_VERSION: $value"
    exit 1
  fi
}

validate_id_component "REPO_OWNER" "$REPO_OWNER"
validate_id_component "REPO_NAME" "$REPO_NAME"
if [ -n "$VERSION" ]; then
  validate_release_version "$VERSION"
fi

for cmd in curl systemctl openssl uname id sed awk head tar mktemp rm install; do
  if ! lookup_trusted_command "$cmd" >/dev/null; then
    echo "Required command not found: $cmd"
    exit 1
  fi
done
CURL_BIN="$(lookup_trusted_command curl)"
SYSTEMCTL_BIN="$(lookup_trusted_command systemctl)"
OPENSSL_BIN="$(lookup_trusted_command openssl)"
UNAME_BIN="$(lookup_trusted_command uname)"
ID_BIN="$(lookup_trusted_command id)"
SED_BIN="$(lookup_trusted_command sed)"
AWK_BIN="$(lookup_trusted_command awk)"
HEAD_BIN="$(lookup_trusted_command head)"
TAR_BIN="$(lookup_trusted_command tar)"
MKTEMP_BIN="$(lookup_trusted_command mktemp)"
RM_BIN="$(lookup_trusted_command rm)"

if [ "$("$UNAME_BIN" -s)" != "Linux" ]; then
  echo "This installer currently supports Linux only."
  exit 1
fi

case "$("$UNAME_BIN" -m)" in
  x86_64|amd64) ;;
  *)
    echo "Unsupported architecture: $("$UNAME_BIN" -m). Only x86_64 is supported."
    exit 1
    ;;
esac

if [ "${EUID:-$("$ID_BIN" -u)}" -eq 0 ]; then
  SUDO=""
else
  if ! lookup_trusted_command sudo >/dev/null; then
    echo "Please run as root or install sudo."
    exit 1
  fi
  SUDO="$(lookup_trusted_command sudo)"
fi

run_privileged() {
  if [ -n "$SUDO" ]; then
    "$SUDO" "$@"
  else
    "$@"
  fi
}

curl_fetch() {
  "$CURL_BIN" --proto '=https' --tlsv1.2 -fsSL "$@"
}

curl_download() {
  local url="$1"
  local path="$2"
  "$CURL_BIN" --proto '=https' --tlsv1.2 -fL "$url" -o "$path"
}

INSTALL_METHOD=""
if lookup_trusted_command apt-get >/dev/null && lookup_trusted_command dpkg >/dev/null; then
  INSTALL_METHOD="apt"
elif lookup_trusted_command dnf >/dev/null; then
  INSTALL_METHOD="dnf"
elif lookup_trusted_command yum >/dev/null; then
  INSTALL_METHOD="yum"
elif lookup_trusted_command zypper >/dev/null; then
  INSTALL_METHOD="zypper"
elif lookup_trusted_command rpm >/dev/null; then
  INSTALL_METHOD="rpm"
else
  INSTALL_METHOD="tar"
fi

if [ -z "$VERSION" ]; then
  RELEASE_API="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"
  VERSION="$( (curl_fetch "$RELEASE_API" || true) | "$SED_BIN" -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | "$HEAD_BIN" -n1 )"
  if [ -z "$VERSION" ]; then
    TAGS_API="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/tags"
    TAGS_JSON="$(curl_fetch "$TAGS_API" || true)"
    VERSION="$(printf '%s\n' "$TAGS_JSON" | "$SED_BIN" -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | "$AWK_BIN" '/^v[0-9]+\.[0-9]+\.[0-9]+$/{print; exit}')"
    if [ -z "$VERSION" ]; then
      VERSION="$(printf '%s\n' "$TAGS_JSON" | "$SED_BIN" -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | "$HEAD_BIN" -n1)"
    fi
  fi
  if [ -z "$VERSION" ]; then
    echo "Could not resolve latest release/tag from GitHub. Set GOV_PASS_VERSION and retry."
    exit 1
  fi
  validate_release_version "$VERSION"
fi

BASE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}"
TMP_DIR="$("$MKTEMP_BIN" -d)"
LINUX_RELEASE_ASSET="gov-pass-${VERSION}-linux-amd64.tar.gz"
LINUX_RELEASE_PATH="${TMP_DIR}/${LINUX_RELEASE_ASSET}"
LINUX_RELEASE_EXTRACTED="${TMP_DIR}/gov-pass-${VERSION}-linux-amd64"
cleanup() {
  "$RM_BIN" -rf "$TMP_DIR"
}
trap cleanup EXIT

release_pubkey_cache=""

prepare_release_pubkey() {
  if [ -n "$release_pubkey_cache" ]; then
    printf '%s\n' "$release_pubkey_cache"
    return
  fi

  if [ -n "$RELEASE_PUBKEY_PATH" ]; then
    release_pubkey_cache="$RELEASE_PUBKEY_PATH"
  elif [ -n "$RELEASE_PUBKEY_PEM" ]; then
    release_pubkey_cache="${TMP_DIR}/release-signing-pub.pem"
    printf '%s\n' "$RELEASE_PUBKEY_PEM" > "$release_pubkey_cache"
  elif [ -n "$RELEASE_PUBKEY_PEM_B64" ]; then
    local base64_bin
    base64_bin="$(lookup_trusted_command base64 || true)"
    if [ -z "$base64_bin" ]; then
      echo "base64 is required when GOV_PASS_RELEASE_PUBKEY_PEM_B64 is used." >&2
      exit 1
    fi
    release_pubkey_cache="${TMP_DIR}/release-signing-pub.pem"
    printf '%s' "$RELEASE_PUBKEY_PEM_B64" | "$base64_bin" -d > "$release_pubkey_cache"
  else
    cat >&2 <<'EOF'
Missing trusted release signing public key.
Set one of:
  GOV_PASS_RELEASE_PUBKEY_PATH
  GOV_PASS_RELEASE_PUBKEY_PEM
  GOV_PASS_RELEASE_PUBKEY_PEM_B64

Obtain this key from a maintainer-controlled channel before running the installer.
EOF
    exit 1
  fi

  if [ -L "$release_pubkey_cache" ]; then
    echo "Release signing public key must not be a symlink: $release_pubkey_cache" >&2
    exit 1
  fi
  if [ ! -f "$release_pubkey_cache" ]; then
    echo "Release signing public key not found: $release_pubkey_cache" >&2
    exit 1
  fi
  if ! "$OPENSSL_BIN" pkey -pubin -in "$release_pubkey_cache" -text -noout >/dev/null 2>&1; then
    echo "Release signing public key is not a valid PEM public key: $release_pubkey_cache" >&2
    exit 1
  fi
  printf '%s\n' "$release_pubkey_cache"
}

checksum_manifest_for_asset() {
  case "$1" in
    *.deb)
      printf '%s\n' "SHA256SUMS.deb"
      ;;
    *.rpm)
      printf '%s\n' "SHA256SUMS.rpm"
      ;;
    *)
      printf '%s\n' "SHA256SUMS"
      ;;
  esac
}

checksum_manifest_path() {
  local manifest="$1"
  printf '%s\n' "${TMP_DIR}/${manifest}"
}

checksum_signature_asset() {
  local manifest="$1"
  printf '%s.sig\n' "$manifest"
}

checksum_signature_path() {
  local manifest="$1"
  printf '%s\n' "${TMP_DIR}/$(checksum_signature_asset "$manifest")"
}

download_checksum_manifest_signature() {
  local manifest="$1"
  local asset
  local path
  asset="$(checksum_signature_asset "$manifest")"
  path="$(checksum_signature_path "$manifest")"
  if [ -f "$path" ]; then
    printf '%s\n' "$path"
    return
  fi
  curl_download "${BASE_URL}/${asset}" "$path"
  printf '%s\n' "$path"
}

verify_checksum_manifest_signature() {
  local manifest="$1"
  local manifest_path="$2"
  local signature_path
  local pubkey_path

  signature_path="$(download_checksum_manifest_signature "$manifest")"
  pubkey_path="$(prepare_release_pubkey)"
  if ! "$OPENSSL_BIN" dgst -sha256 -verify "$pubkey_path" -signature "$signature_path" "$manifest_path" >/dev/null 2>&1; then
    echo "Signature verification failed for ${manifest}." >&2
    return 1
  fi
}

download_checksum_manifest() {
  local manifest="$1"
  local path
  path="$(checksum_manifest_path "$manifest")"
  if [ -f "$path" ]; then
    printf '%s\n' "$path"
    return
  fi
  curl_download "${BASE_URL}/${manifest}" "$path"
  verify_checksum_manifest_signature "$manifest" "$path"
  printf '%s\n' "$path"
}

compute_sha256() {
  local path="$1"
  local sha_bin
  sha_bin="$(lookup_trusted_command sha256sum || true)"
  if [ -n "$sha_bin" ]; then
    "$sha_bin" "$path" | "$AWK_BIN" '{print $1}'
    return 0
  fi
  sha_bin="$(lookup_trusted_command shasum || true)"
  if [ -n "$sha_bin" ]; then
    "$sha_bin" -a 256 "$path" | "$AWK_BIN" '{print $1}'
    return 0
  fi
  "$OPENSSL_BIN" dgst -sha256 "$path" | "$AWK_BIN" '{print $NF}'
  return 0
}

verify_release_asset() {
  local asset="$1"
  local path="$2"
  local manifest
  local manifest_path
  local expected
  local actual

  manifest="$(checksum_manifest_for_asset "$asset")"
  manifest_path="$(download_checksum_manifest "$manifest")"
  expected="$("$AWK_BIN" -v asset="$asset" '$2 == asset { print $1; exit }' "$manifest_path")"
  if [ -z "$expected" ]; then
    echo "Checksum manifest ${manifest} does not contain ${asset}." >&2
    return 1
  fi

  actual="$(compute_sha256 "$path")"
  if [ "$actual" != "$expected" ]; then
    echo "Checksum verification failed for ${asset}." >&2
    echo "Expected: ${expected}" >&2
    echo "Actual:   ${actual}" >&2
    return 1
  fi
  return 0
}

download_release_asset() {
  local asset="$1"
  local path="$2"
  curl_download "${BASE_URL}/${asset}" "$path"
  verify_release_asset "$asset" "$path"
}

validate_tarball_members() {
  local archive="$1"
  local prefix="$2"
  local entry
  if ! "$TAR_BIN" -tvzf "$archive" | "$AWK_BIN" 'substr($0, 1, 1) != "-" && substr($0, 1, 1) != "d" { found = 1 } END { exit found ? 1 : 0 }'; then
    echo "Refusing to extract tarball containing non-regular entries." >&2
    return 1
  fi
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    case "$entry" in
      /*|../*|*/../*|*/..|..)
        echo "Refusing to extract unsafe tar member: $entry" >&2
        return 1
        ;;
    esac
    case "$entry" in
      "$prefix"|"$prefix/"|"$prefix"/*)
        ;;
      *)
        echo "Refusing to extract unexpected tar member outside ${prefix}: $entry" >&2
        return 1
        ;;
    esac
  done < <("$TAR_BIN" -tzf "$archive")
}

require_release_file() {
  local path="$1"
  local label="$2"
  if [ -L "$path" ]; then
    echo "${label} must not be a symlink: $path" >&2
    exit 1
  fi
  if [ ! -f "$path" ]; then
    echo "${label} is missing: $path" >&2
    exit 1
  fi
}

ensure_linux_release_extracted() {
  if [ -d "$LINUX_RELEASE_EXTRACTED" ]; then
    return
  fi
  if [ ! -f "$LINUX_RELEASE_PATH" ]; then
    download_release_asset "$LINUX_RELEASE_ASSET" "$LINUX_RELEASE_PATH"
  fi
  validate_tarball_members "$LINUX_RELEASE_PATH" "gov-pass-${VERSION}-linux-amd64"
  "$TAR_BIN" -xzf "$LINUX_RELEASE_PATH" -C "$TMP_DIR"
}

install_tui_from_release() {
  local tui_target="/opt/gov-pass/dist/gov-pass-tui"
  local install_bin
  install_bin="$(lookup_trusted_command install)"
  ensure_linux_release_extracted
  require_release_file "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" "TUI controller binary"
  run_privileged "$install_bin" -d /opt/gov-pass/dist
  run_privileged "$install_bin" -m 0755 "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" "$tui_target"
}

install_tui_host_packages() {
  echo "Installing nmtui-like TUI runtime (whiptail/newt) (best-effort)..."
  case "$INSTALL_METHOD" in
    apt)
      local apt_bin
      apt_bin="$(lookup_trusted_command apt-get)"
      run_privileged "$apt_bin" update -qq || true
      run_privileged "$apt_bin" install -y --no-install-recommends whiptail || run_privileged "$apt_bin" install -y --no-install-recommends newt || true
      ;;
    dnf)
      local dnf_bin
      dnf_bin="$(lookup_trusted_command dnf)"
      run_privileged "$dnf_bin" install -y newt || true
      ;;
    yum)
      local yum_bin
      yum_bin="$(lookup_trusted_command yum)"
      run_privileged "$yum_bin" install -y newt || true
      ;;
    zypper)
      local zypper_bin
      zypper_bin="$(lookup_trusted_command zypper)"
      run_privileged "$zypper_bin" --non-interactive install newt || true
      ;;
    rpm|tar)
      echo "Skipping auto-install for method=${INSTALL_METHOD}; install package manually: whiptail (or newt)"
      ;;
  esac
}

maybe_install_tui() {
  if [ "$INSTALL_TUI" != "1" ]; then
    return
  fi

  echo "INSTALL_TUI=1 detected: installing TUI controller components..."
  if [ ! -x /opt/gov-pass/dist/gov-pass-tui ]; then
    install_tui_from_release
  fi
  install_tui_host_packages
}

install_from_apt() {
  local asset="gov-pass-${VERSION}-linux-amd64.deb"
  local path="${TMP_DIR}/${asset}"
  local apt_bin
  apt_bin="$(lookup_trusted_command apt-get)"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "DEB asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  if ! run_privileged "$apt_bin" install -y "$path"; then
    run_privileged "$apt_bin" -f install -y
    run_privileged "$apt_bin" install -y "$path"
  fi
}

install_from_rpm() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  local rpm_bin
  rpm_bin="$(lookup_trusted_command rpm)"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  run_privileged "$rpm_bin" -Uvh --replacepkgs "$path"
}

install_from_dnf() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  local dnf_bin
  dnf_bin="$(lookup_trusted_command dnf)"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  run_privileged "$dnf_bin" install -y "$path"
}

install_from_yum() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  local yum_bin
  yum_bin="$(lookup_trusted_command yum)"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  run_privileged "$yum_bin" install -y "$path"
}

install_from_zypper() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  local zypper_bin
  zypper_bin="$(lookup_trusted_command zypper)"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  run_privileged "$zypper_bin" --non-interactive install "$path"
}

install_from_tar() {
  local install_bin
  install_bin="$(lookup_trusted_command install)"
  ensure_linux_release_extracted

  run_privileged "$install_bin" -d /opt/gov-pass/dist
  require_release_file "${LINUX_RELEASE_EXTRACTED}/splitter" "splitter binary"
  require_release_file "${LINUX_RELEASE_EXTRACTED}/gov-pass.service" "gov-pass.service"
  require_release_file "${LINUX_RELEASE_EXTRACTED}/gov-pass.default" "gov-pass.default"
  run_privileged "$install_bin" -m 0755 "${LINUX_RELEASE_EXTRACTED}/splitter" /opt/gov-pass/dist/splitter
  if [ -L "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" ]; then
    echo "optional gov-pass-tui must not be a symlink: ${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" >&2
    exit 1
  fi
  if [ -f "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" ]; then
    run_privileged "$install_bin" -m 0755 "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" /opt/gov-pass/dist/gov-pass-tui
  fi

  run_privileged "$install_bin" -m 0644 "${LINUX_RELEASE_EXTRACTED}/gov-pass.service" /etc/systemd/system/gov-pass.service
  if [ ! -f /etc/default/gov-pass ]; then
    run_privileged "$install_bin" -D -m 0644 "${LINUX_RELEASE_EXTRACTED}/gov-pass.default" /etc/default/gov-pass
  fi
}

case "$INSTALL_METHOD" in
  apt)
    install_from_apt
    ;;
  dnf)
    install_from_dnf
    ;;
  yum)
    install_from_yum
    ;;
  zypper)
    install_from_zypper
    ;;
  rpm)
    install_from_rpm
    ;;
  tar)
    install_from_tar
    ;;
esac

maybe_install_tui

run_privileged "$SYSTEMCTL_BIN" daemon-reload
if [ "$NO_START" != "1" ]; then
  run_privileged "$SYSTEMCTL_BIN" enable --now gov-pass
  echo "gov-pass installed and started (version: ${VERSION}, method: ${INSTALL_METHOD})."
else
  echo "gov-pass installed (version: ${VERSION}, method: ${INSTALL_METHOD}). Start skipped (NO_START=1)."
fi

echo "Verify with: systemctl status gov-pass --no-pager"
if [ "$INSTALL_TUI" = "1" ]; then
  echo "TUI controller binary: /opt/gov-pass/dist/gov-pass-tui"
  echo "Linux runs as terminal TUI controller (nmtui-like via whiptail when available)."
  echo "Manual start: /opt/gov-pass/dist/gov-pass-tui --service-name gov-pass"
fi
