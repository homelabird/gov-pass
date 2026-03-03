#!/usr/bin/env bash
set -euo pipefail

REPO_OWNER="${REPO_OWNER:-homelabird}"
REPO_NAME="${REPO_NAME:-gov-pass}"
VERSION="${GOV_PASS_VERSION:-}"
NO_START="${NO_START:-0}"

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

for cmd in curl systemctl; do
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
  VERSION="$( (curl -fsSL "$RELEASE_API" || true) | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 )"
  if [ -z "$VERSION" ]; then
    TAGS_API="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/tags"
    TAGS_JSON="$(curl -fsSL "$TAGS_API" || true)"
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
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

install_from_apt() {
  local asset="gov-pass-${VERSION}-linux-amd64.deb"
  local path="${TMP_DIR}/${asset}"
  if ! curl -fL "${BASE_URL}/${asset}" -o "$path"; then
    echo "DEB asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  if ! ${SUDO} apt-get install -y "$path"; then
    ${SUDO} apt-get -f install -y
    ${SUDO} apt-get install -y "$path"
  fi
}

install_from_rpm() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  if ! curl -fL "${BASE_URL}/${asset}" -o "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  ${SUDO} rpm -Uvh --replacepkgs "$path"
}

install_from_dnf() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  if ! curl -fL "${BASE_URL}/${asset}" -o "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  ${SUDO} dnf install -y "$path"
}

install_from_yum() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  if ! curl -fL "${BASE_URL}/${asset}" -o "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  ${SUDO} yum install -y "$path"
}

install_from_zypper() {
  local asset="gov-pass-${VERSION}-linux-amd64.rpm"
  local path="${TMP_DIR}/${asset}"
  if ! curl -fL "${BASE_URL}/${asset}" -o "$path"; then
    echo "RPM asset not found for ${VERSION}; falling back to tarball install."
    install_from_tar
    return
  fi
  ${SUDO} zypper --non-interactive install --allow-unsigned-rpm "$path"
}

install_from_tar() {
  local asset="gov-pass-${VERSION}-linux-amd64.tar.gz"
  local path="${TMP_DIR}/${asset}"
  local extracted="${TMP_DIR}/gov-pass-${VERSION}-linux-amd64"
  local service_url="https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/${VERSION}/scripts/linux/gov-pass.service"
  local service_url_main="https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/main/scripts/linux/gov-pass.service"
  local used_source_fallback="0"

  if ! curl -fL "${BASE_URL}/${asset}" -o "$path"; then
    echo "Tarball release asset not found for ${VERSION}; falling back to source tag archive build."
    local src_archive="${TMP_DIR}/${REPO_NAME}-${VERSION}.src.tar.gz"
    local src_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/archive/refs/tags/${VERSION}.tar.gz"
    if ! curl -fL "$src_url" -o "$src_archive"; then
      echo "Source tag archive not found at ${src_url}"
      exit 1
    fi
    local src_root
    local src_listing="${TMP_DIR}/source-list.txt"
    tar -tzf "$src_archive" > "$src_listing"
    src_root="$(head -n1 "$src_listing" | cut -d/ -f1)"
    tar -xzf "$src_archive" -C "$TMP_DIR"
    local src_dir="${TMP_DIR}/${src_root}"
    if [ ! -d "$src_dir" ]; then
      echo "Extracted source directory not found: ${src_dir}"
      exit 1
    fi
    if ! command -v go >/dev/null 2>&1; then
      echo "go is required for source fallback build but was not found."
      exit 1
    fi
    (cd "$src_dir" && go build -o "${TMP_DIR}/splitter" ./cmd/splitter)
    used_source_fallback="1"
  fi

  ${SUDO} install -d /opt/gov-pass/dist
  if [ "$used_source_fallback" = "1" ]; then
    ${SUDO} install -m 0755 "${TMP_DIR}/splitter" /opt/gov-pass/dist/splitter
  else
    tar -xzf "$path" -C "$TMP_DIR"
    if [ ! -f "${extracted}/splitter" ]; then
      echo "splitter binary not found in tarball: ${extracted}"
      exit 1
    fi
    ${SUDO} install -m 0755 "${extracted}/splitter" /opt/gov-pass/dist/splitter
    if [ -f "${extracted}/gov-pass-tray" ]; then
      ${SUDO} install -m 0755 "${extracted}/gov-pass-tray" /opt/gov-pass/dist/gov-pass-tray
    fi
  fi

  if ! curl -fL "$service_url" -o "${TMP_DIR}/gov-pass.service"; then
    curl -fL "$service_url_main" -o "${TMP_DIR}/gov-pass.service"
  fi
  ${SUDO} install -m 0644 "${TMP_DIR}/gov-pass.service" /etc/systemd/system/gov-pass.service
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

${SUDO} systemctl daemon-reload
if [ "$NO_START" != "1" ]; then
  ${SUDO} systemctl enable --now gov-pass
  echo "gov-pass installed and started (version: ${VERSION}, method: ${INSTALL_METHOD})."
else
  echo "gov-pass installed (version: ${VERSION}, method: ${INSTALL_METHOD}). Start skipped (NO_START=1)."
fi

echo "Verify with: systemctl status gov-pass --no-pager"
