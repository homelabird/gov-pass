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
      echo "sign_release_manifests: refusing symlinked trusted command for $name: $candidate" >&2
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
  echo "sign_release_manifests: required command not found in trusted directories: $name" >&2
  exit 1
}

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

MKTEMP_BIN="$(lookup_trusted_command mktemp)"
RM_BIN="$(lookup_trusted_command rm)"
BASE64_BIN="$(lookup_trusted_command base64)"
CHMOD_BIN="$(lookup_trusted_command chmod)"
OPENSSL_BIN="$(lookup_trusted_command openssl)"

tmpdir="$("$MKTEMP_BIN" -d)"
cleanup() { "$RM_BIN" -rf -- "$tmpdir"; }
trap cleanup EXIT

key_path=""
passfile=""
key_args=()

prepare_key() {
  if [[ -n "${RELEASE_CHECKSUM_SIGNING_KEY_PATH:-}" ]]; then
    key_path="$RELEASE_CHECKSUM_SIGNING_KEY_PATH"
  elif [[ -n "${RELEASE_CHECKSUM_SIGNING_KEY_PEM_B64:-}" ]]; then
    key_path="$tmpdir/release-signing-key.pem"
    printf '%s' "$RELEASE_CHECKSUM_SIGNING_KEY_PEM_B64" | "$BASE64_BIN" -d >"$key_path"
    "$CHMOD_BIN" 600 "$key_path"
  elif [[ -n "${RELEASE_CHECKSUM_SIGNING_KEY_PEM:-}" ]]; then
    key_path="$tmpdir/release-signing-key.pem"
    printf '%s\n' "$RELEASE_CHECKSUM_SIGNING_KEY_PEM" >"$key_path"
    "$CHMOD_BIN" 600 "$key_path"
  else
    echo "sign_release_manifests: missing release signing key env" >&2
    exit 1
  fi

  if [[ -L "$key_path" ]]; then
    echo "sign_release_manifests: refusing to use symlink private key: $key_path" >&2
    exit 1
  fi
  if [[ ! -f "$key_path" ]]; then
    echo "sign_release_manifests: private key not found: $key_path" >&2
    exit 1
  fi

  if [[ -n "${RELEASE_CHECKSUM_SIGNING_KEY_PASS:-}" ]]; then
    passfile="$tmpdir/release-signing-key.pass"
    printf '%s' "$RELEASE_CHECKSUM_SIGNING_KEY_PASS" >"$passfile"
    "$CHMOD_BIN" 600 "$passfile"
  fi
}

prepare_key

pubkey="$tmpdir/release-signing-pub.pem"
if [[ -n "$passfile" ]]; then
  key_args=(-passin "file:$passfile")
fi

"$OPENSSL_BIN" pkey "${key_args[@]}" -in "$key_path" -pubout -out "$pubkey" >/dev/null 2>&1

for manifest in "$@"; do
  if [[ -L "$manifest" ]]; then
    echo "sign_release_manifests: refusing to sign symlink manifest: $manifest" >&2
    exit 1
  fi
  if [[ ! -f "$manifest" ]]; then
    echo "sign_release_manifests: manifest not found: $manifest" >&2
    exit 1
  fi

  sig_path="${manifest}.sig"
  if [[ -L "$sig_path" ]]; then
    echo "sign_release_manifests: refusing to write signature through symlink: $sig_path" >&2
    exit 1
  fi
  "$OPENSSL_BIN" dgst -sha256 -sign "$key_path" "${key_args[@]}" -out "$sig_path" "$manifest"
  "$OPENSSL_BIN" dgst -sha256 -verify "$pubkey" -signature "$sig_path" "$manifest" >/dev/null
done
