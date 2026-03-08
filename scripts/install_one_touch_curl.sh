#!/usr/bin/env bash
set -euo pipefail

REPO_OWNER="${REPO_OWNER:-homelabird}"
REPO_NAME="${REPO_NAME:-gov-pass}"
VERSION="${GOV_PASS_VERSION:-}"
NO_START="${NO_START:-0}"
INSTALL_TUI="${INSTALL_TUI:-0}"
RELEASE_PUBKEY_PATH="${GOV_PASS_RELEASE_PUBKEY_PATH:-}"
RELEASE_PUBKEY_PEM="${GOV_PASS_RELEASE_PUBKEY_PEM:-}"
RELEASE_PUBKEY_PEM_B64="${GOV_PASS_RELEASE_PUBKEY_PEM_B64:-}"

if [ "$(uname -s)" != "Linux" ]; then
  echo "This installer currently supports Linux only."
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64) ;;
  *)
    echo "Unsupported architecture: $(uname -m). Only x86_64 is supported."
    exit 1
    ;;
esac

for cmd in curl systemctl openssl; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Required command not found: $cmd"
    exit 1
  fi
done

if [ "${EUID:-$(id -u)}" -eq 0 ]; then
  SUDO=""
else
  if ! command -v sudo >/dev/null 2>&1; then
    echo "Please run as root or install sudo."
    exit 1
  fi
  SUDO="sudo"
fi

curl_fetch() {
  curl --proto '=https' --tlsv1.2 -fsSL "$@"
}

curl_download() {
  local url="$1"
  local path="$2"
  curl --proto '=https' --tlsv1.2 -fL "$url" -o "$path"
}

INSTALL_METHOD=""
if command -v apt-get >/dev/null 2>&1 && command -v dpkg >/dev/null 2>&1; then
  INSTALL_METHOD="apt"
elif command -v dnf >/dev/null 2>&1; then
  INSTALL_METHOD="dnf"
elif command -v yum >/dev/null 2>&1; then
  INSTALL_METHOD="yum"
elif command -v zypper >/dev/null 2>&1; then
  INSTALL_METHOD="zypper"
elif command -v rpm >/dev/null 2>&1; then
  INSTALL_METHOD="rpm"
else
  INSTALL_METHOD="tar"
fi

if [ -z "$VERSION" ]; then
  RELEASE_API="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"
  VERSION="$( (curl_fetch "$RELEASE_API" || true) | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 )"
  if [ -z "$VERSION" ]; then
    TAGS_API="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/tags"
    TAGS_JSON="$(curl_fetch "$TAGS_API" || true)"
    VERSION="$(printf '%s\n' "$TAGS_JSON" | sed -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | awk '/^v[0-9]+\.[0-9]+\.[0-9]+$/{print; exit}')"
    if [ -z "$VERSION" ]; then
      VERSION="$(printf '%s\n' "$TAGS_JSON" | sed -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
    fi
  fi
  if [ -z "$VERSION" ]; then
    echo "Could not resolve latest release/tag from GitHub. Set GOV_PASS_VERSION and retry."
    exit 1
  fi
fi

BASE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}"
TMP_DIR="$(mktemp -d)"
LINUX_RELEASE_ASSET="gov-pass-${VERSION}-linux-amd64.tar.gz"
LINUX_RELEASE_PATH="${TMP_DIR}/${LINUX_RELEASE_ASSET}"
LINUX_RELEASE_EXTRACTED="${TMP_DIR}/gov-pass-${VERSION}-linux-amd64"
cleanup() {
  rm -rf "$TMP_DIR"
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
    if ! command -v base64 >/dev/null 2>&1; then
      echo "base64 is required when GOV_PASS_RELEASE_PUBKEY_PEM_B64 is used." >&2
      exit 1
    fi
    release_pubkey_cache="${TMP_DIR}/release-signing-pub.pem"
    printf '%s' "$RELEASE_PUBKEY_PEM_B64" | base64 -d > "$release_pubkey_cache"
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

  if [ ! -f "$release_pubkey_cache" ]; then
    echo "Release signing public key not found: $release_pubkey_cache" >&2
    exit 1
  fi
  if ! openssl pkey -pubin -in "$release_pubkey_cache" -text -noout >/dev/null 2>&1; then
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
  if ! openssl dgst -sha256 -verify "$pubkey_path" -signature "$signature_path" "$manifest_path" >/dev/null 2>&1; then
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
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print $1}'
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$path" | awk '{print $1}'
    return 0
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$path" | awk '{print $NF}'
    return 0
  fi
  echo "No SHA256 tool found. Install sha256sum, shasum, or openssl." >&2
  return 1
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
  expected="$(awk -v asset="$asset" '$2 == asset { print $1; exit }' "$manifest_path")"
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

ensure_linux_release_extracted() {
  if [ -d "$LINUX_RELEASE_EXTRACTED" ]; then
    return
  fi
  if [ ! -f "$LINUX_RELEASE_PATH" ]; then
    download_release_asset "$LINUX_RELEASE_ASSET" "$LINUX_RELEASE_PATH"
  fi
  tar -xzf "$LINUX_RELEASE_PATH" -C "$TMP_DIR"
}

install_tui_from_release() {
  local tui_target="/opt/gov-pass/dist/gov-pass-tui"
  ensure_linux_release_extracted
  if [ ! -f "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" ]; then
    echo "TUI controller binary is missing from ${LINUX_RELEASE_ASSET}."
    exit 1
  fi
  ${SUDO} install -d /opt/gov-pass/dist
  ${SUDO} install -m 0755 "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" "$tui_target"
}

install_tui_host_packages() {
  echo "Installing nmtui-like TUI runtime (whiptail/newt) (best-effort)..."
  case "$INSTALL_METHOD" in
    apt)
      ${SUDO} apt-get update -qq || true
      ${SUDO} apt-get install -y --no-install-recommends whiptail || ${SUDO} apt-get install -y --no-install-recommends newt || true
      ;;
    dnf)
      ${SUDO} dnf install -y newt || true
      ;;
    yum)
      ${SUDO} yum install -y newt || true
      ;;
    zypper)
      ${SUDO} zypper --non-interactive install newt || true
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
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "DEB asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  if ! ${SUDO} apt-get install -y "$path"; then
    ${SUDO} apt-get -f install -y
    ${SUDO} apt-get install -y "$path"
  fi
}

install_from_rpm() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  ${SUDO} rpm -Uvh --replacepkgs "$path"
}

install_from_dnf() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  ${SUDO} dnf install -y "$path"
}

install_from_yum() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  ${SUDO} yum install -y "$path"
}

install_from_zypper() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  if ! curl_download "${BASE_URL}/${asset}" "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  verify_release_asset "${asset}" "$path"
  ${SUDO} zypper --non-interactive install "$path"
}

install_from_tar() {
  ensure_linux_release_extracted

  ${SUDO} install -d /opt/gov-pass/dist
  if [ ! -f "${LINUX_RELEASE_EXTRACTED}/splitter" ]; then
    echo "splitter binary not found in tarball: ${LINUX_RELEASE_EXTRACTED}"
    exit 1
  fi
  if [ ! -f "${LINUX_RELEASE_EXTRACTED}/gov-pass.service" ]; then
    echo "gov-pass.service is missing from ${LINUX_RELEASE_ASSET}."
    exit 1
  fi
  if [ ! -f "${LINUX_RELEASE_EXTRACTED}/gov-pass.default" ]; then
    echo "gov-pass.default is missing from ${LINUX_RELEASE_ASSET}."
    exit 1
  fi
  ${SUDO} install -m 0755 "${LINUX_RELEASE_EXTRACTED}/splitter" /opt/gov-pass/dist/splitter
  if [ -f "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" ]; then
    ${SUDO} install -m 0755 "${LINUX_RELEASE_EXTRACTED}/gov-pass-tui" /opt/gov-pass/dist/gov-pass-tui
  fi

  ${SUDO} install -m 0644 "${LINUX_RELEASE_EXTRACTED}/gov-pass.service" /etc/systemd/system/gov-pass.service
  if [ ! -f /etc/default/gov-pass ]; then
    ${SUDO} install -D -m 0644 "${LINUX_RELEASE_EXTRACTED}/gov-pass.default" /etc/default/gov-pass
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

${SUDO} systemctl daemon-reload
if [ "$NO_START" != "1" ]; then
  ${SUDO} systemctl enable --now gov-pass
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
