#!/usr/bin/env bash
set -euo pipefail
umask 077

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

lookup_optional_trusted_command() {
  local name="$1"
  local old_ifs="$IFS"
  local dir candidate
  IFS=:
  for dir in $TRUSTED_PATH; do
    candidate="${dir}/${name}"
    if [[ -L "$candidate" ]]; then
      echo "verify_release_manifest_signature: refusing symlinked trusted command for $name: $candidate" >&2
      IFS="$old_ifs"
      exit 1
    fi
    if [[ -f "$candidate" && -x "$candidate" ]]; then
      printf '%s\n' "$candidate"
      IFS="$old_ifs"
      return 0
    fi
  done
  IFS="$old_ifs"
  return 1
}

lookup_trusted_command() {
  local name="$1"
  local candidate
  if candidate="$(lookup_optional_trusted_command "$name")"; then
    printf '%s\n' "$candidate"
    return 0
  fi
  echo "verify_release_manifest_signature: required command not found in trusted directories: $name" >&2
  exit 1
}

usage() {
  cat <<'EOF' >&2
Usage:
  verify_release_manifest_signature.sh <manifest> [signature]

Required env (choose one):
  RELEASE_CHECKSUM_VERIFY_PUBKEY_PATH       Path to a PEM-encoded public key
  RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM_B64    Base64-encoded PEM public key
  RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM        Literal PEM public key content
EOF
  exit 2
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
fi

manifest="$1"
sig_path="${2:-${manifest}.sig}"

if [[ -L "$manifest" ]]; then
  echo "verify_release_manifest_signature: refusing to verify symlink manifest: $manifest" >&2
  exit 1
fi
if [[ ! -f "$manifest" ]]; then
  echo "verify_release_manifest_signature: manifest not found: $manifest" >&2
  exit 1
fi
if [[ -L "$sig_path" ]]; then
  echo "verify_release_manifest_signature: refusing to verify symlink signature: $sig_path" >&2
  exit 1
fi
if [[ ! -f "$sig_path" ]]; then
  echo "verify_release_manifest_signature: signature not found: $sig_path" >&2
  exit 1
fi

MKTEMP_BIN="$(lookup_trusted_command mktemp)"
RM_BIN="$(lookup_trusted_command rm)"
BASE64_BIN="$(lookup_trusted_command base64)"
OPENSSL_BIN="$(lookup_trusted_command openssl)"

tmpdir="$("$MKTEMP_BIN" -d)"
cleanup() { "$RM_BIN" -rf -- "$tmpdir"; }
trap cleanup EXIT

pubkey_path=""
if [[ -n "${RELEASE_CHECKSUM_VERIFY_PUBKEY_PATH:-}" ]]; then
  pubkey_path="$RELEASE_CHECKSUM_VERIFY_PUBKEY_PATH"
elif [[ -n "${RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM_B64:-}" ]]; then
  pubkey_path="$tmpdir/release-signing-pub.pem"
  printf '%s' "$RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM_B64" | "$BASE64_BIN" -d >"$pubkey_path"
elif [[ -n "${RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM:-}" ]]; then
  pubkey_path="$tmpdir/release-signing-pub.pem"
  printf '%s\n' "$RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM" >"$pubkey_path"
else
  echo "verify_release_manifest_signature: missing release verification public key env" >&2
  exit 1
fi

if [[ -L "$pubkey_path" ]]; then
  echo "verify_release_manifest_signature: refusing to verify with symlink public key: $pubkey_path" >&2
  exit 1
fi
if [[ ! -f "$pubkey_path" ]]; then
  echo "verify_release_manifest_signature: public key not found: $pubkey_path" >&2
  exit 1
fi

"$OPENSSL_BIN" pkey -pubin -in "$pubkey_path" -text -noout >/dev/null 2>&1
"$OPENSSL_BIN" dgst -sha256 -verify "$pubkey_path" -signature "$sig_path" "$manifest" >/dev/null
