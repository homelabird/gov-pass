#!/usr/bin/env bash
set -euo pipefail

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

if [[ ! -f "$manifest" ]]; then
  echo "verify_release_manifest_signature: manifest not found: $manifest" >&2
  exit 1
fi
if [[ ! -f "$sig_path" ]]; then
  echo "verify_release_manifest_signature: signature not found: $sig_path" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

pubkey_path=""
if [[ -n "${RELEASE_CHECKSUM_VERIFY_PUBKEY_PATH:-}" ]]; then
  pubkey_path="$RELEASE_CHECKSUM_VERIFY_PUBKEY_PATH"
elif [[ -n "${RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM_B64:-}" ]]; then
  pubkey_path="$tmpdir/release-signing-pub.pem"
  printf '%s' "$RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM_B64" | base64 -d >"$pubkey_path"
elif [[ -n "${RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM:-}" ]]; then
  pubkey_path="$tmpdir/release-signing-pub.pem"
  printf '%s\n' "$RELEASE_CHECKSUM_VERIFY_PUBKEY_PEM" >"$pubkey_path"
else
  echo "verify_release_manifest_signature: missing release verification public key env" >&2
  exit 1
fi

if [[ ! -f "$pubkey_path" ]]; then
  echo "verify_release_manifest_signature: public key not found: $pubkey_path" >&2
  exit 1
fi

openssl pkey -pubin -in "$pubkey_path" -text -noout >/dev/null 2>&1
openssl dgst -sha256 -verify "$pubkey_path" -signature "$sig_path" "$manifest" >/dev/null
