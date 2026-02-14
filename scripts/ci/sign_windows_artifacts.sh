#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF' >&2
Usage:
  sign_windows_artifacts.sh <file> [file...]

Required env:
  WINDOWS_CODESIGN_PFX_B64       Base64-encoded PFX/PKCS12 (no quotes)
  WINDOWS_CODESIGN_PFX_PASSWORD  PFX password

Optional env:
  WINDOWS_CODESIGN_TIMESTAMP_URL RFC3161 timestamp URL (default: http://timestamp.digicert.com)
  WINDOWS_CODESIGN_DESC          Signature description (default: gov-pass)
  WINDOWS_CODESIGN_URL           URL (default: empty)
EOF
  exit 2
}

if [[ $# -lt 1 ]]; then
  usage
fi

: "${WINDOWS_CODESIGN_PFX_B64:?missing WINDOWS_CODESIGN_PFX_B64}"
: "${WINDOWS_CODESIGN_PFX_PASSWORD:?missing WINDOWS_CODESIGN_PFX_PASSWORD}"

ts_url="${WINDOWS_CODESIGN_TIMESTAMP_URL:-http://timestamp.digicert.com}"
desc="${WINDOWS_CODESIGN_DESC:-gov-pass}"
url="${WINDOWS_CODESIGN_URL:-}"

tmpdir="$(mktemp -d)"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

pfx="$tmpdir/codesign.pfx"
passfile="$tmpdir/pass.txt"

printf '%s' "$WINDOWS_CODESIGN_PFX_B64" | base64 -d >"$pfx"
printf '%s' "$WINDOWS_CODESIGN_PFX_PASSWORD" >"$passfile"
chmod 600 "$pfx" "$passfile"

sign_one() {
  local in="$1"
  local out="$tmpdir/$(basename "$in").signed"
  local extra=()

  case "${in##*.}" in
    msi|MSI)
      extra+=(-add-msi-dse)
      ;;
  esac

  # Use RFC3161 timestamping (-ts) and SHA-256 digest.
  osslsigncode sign \
    -pkcs12 "$pfx" \
    -readpass "$passfile" \
    -h sha256 \
    -n "$desc" \
    ${url:+-i "$url"} \
    -ts "$ts_url" \
    "${extra[@]}" \
    -in "$in" \
    -out "$out" >/dev/null

  mv -f "$out" "$in"
}

for f in "$@"; do
  if [[ ! -f "$f" ]]; then
    echo "sign_windows_artifacts: file not found: $f" >&2
    exit 1
  fi
  sign_one "$f"
done

