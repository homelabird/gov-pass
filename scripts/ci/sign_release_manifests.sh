#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF' >&2
Usage:
  sign_release_manifests.sh <manifest> [manifest...]

Required env (choose one):
  RELEASE_CHECKSUM_SIGNING_KEY_PATH       Path to a PEM-encoded private key
  RELEASE_CHECKSUM_SIGNING_KEY_PEM_B64    Base64-encoded PEM private key
  RELEASE_CHECKSUM_SIGNING_KEY_PEM        Literal PEM private key content

Optional env:
  RELEASE_CHECKSUM_SIGNING_KEY_PASS       Passphrase for encrypted private keys
EOF
  exit 2
}

if [[ $# -lt 1 ]]; then
  usage
fi

tmpdir="$(mktemp -d)"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

key_path=""
passfile=""
key_args=()

prepare_key() {
  if [[ -n "${RELEASE_CHECKSUM_SIGNING_KEY_PATH:-}" ]]; then
    key_path="$RELEASE_CHECKSUM_SIGNING_KEY_PATH"
  elif [[ -n "${RELEASE_CHECKSUM_SIGNING_KEY_PEM_B64:-}" ]]; then
    key_path="$tmpdir/release-signing-key.pem"
    printf '%s' "$RELEASE_CHECKSUM_SIGNING_KEY_PEM_B64" | base64 -d >"$key_path"
    chmod 600 "$key_path"
  elif [[ -n "${RELEASE_CHECKSUM_SIGNING_KEY_PEM:-}" ]]; then
    key_path="$tmpdir/release-signing-key.pem"
    printf '%s\n' "$RELEASE_CHECKSUM_SIGNING_KEY_PEM" >"$key_path"
    chmod 600 "$key_path"
  else
    echo "sign_release_manifests: missing release signing key env" >&2
    exit 1
  fi

  if [[ ! -f "$key_path" ]]; then
    echo "sign_release_manifests: private key not found: $key_path" >&2
    exit 1
  fi

  if [[ -n "${RELEASE_CHECKSUM_SIGNING_KEY_PASS:-}" ]]; then
    passfile="$tmpdir/release-signing-key.pass"
    printf '%s' "$RELEASE_CHECKSUM_SIGNING_KEY_PASS" >"$passfile"
    chmod 600 "$passfile"
  fi
}

prepare_key

pubkey="$tmpdir/release-signing-pub.pem"
if [[ -n "$passfile" ]]; then
  key_args=(-passin "file:$passfile")
fi

openssl pkey "${key_args[@]}" -in "$key_path" -pubout -out "$pubkey" >/dev/null 2>&1

for manifest in "$@"; do
  if [[ ! -f "$manifest" ]]; then
    echo "sign_release_manifests: manifest not found: $manifest" >&2
    exit 1
  fi

  sig_path="${manifest}.sig"
  openssl dgst -sha256 -sign "$key_path" "${key_args[@]}" -out "$sig_path" "$manifest"
  openssl dgst -sha256 -verify "$pubkey" -signature "$sig_path" "$manifest" >/dev/null
done
