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
  WINDOWS_CODESIGN_TIMESTAMP_URL  RFC3161 timestamp URL (single)
  WINDOWS_CODESIGN_TIMESTAMP_URLS RFC3161 timestamp URLs (comma/space separated; tried in order)
                                Default: http://timestamp.digicert.com,http://timestamp.sectigo.com
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

default_ts_urls=("http://timestamp.digicert.com" "http://timestamp.sectigo.com")

build_ts_args() {
  local raw=""
  local -a urls=()
  local -a args=()

  if [[ -n "${WINDOWS_CODESIGN_TIMESTAMP_URLS:-}" ]]; then
    raw="$WINDOWS_CODESIGN_TIMESTAMP_URLS"
  elif [[ -n "${WINDOWS_CODESIGN_TIMESTAMP_URL:-}" ]]; then
    raw="$WINDOWS_CODESIGN_TIMESTAMP_URL"
  else
    urls=("${default_ts_urls[@]}")
  fi

  if [[ -n "$raw" ]]; then
    # Split on commas and whitespace.
    raw="${raw//,/ }"
    # shellcheck disable=SC2206
    urls=($raw)
  fi

  if [[ ${#urls[@]} -eq 0 ]]; then
    echo "sign_windows_artifacts: no timestamp URLs configured" >&2
    exit 1
  fi

  for u in "${urls[@]}"; do
    if [[ -n "$u" ]]; then
      args+=(-ts "$u")
    fi
  done
  if [[ ${#args[@]} -eq 0 ]]; then
    echo "sign_windows_artifacts: no usable timestamp URLs configured" >&2
    exit 1
  fi

  printf '%s\0' "${args[@]}"
}

sign_one() {
  local in="$1"
  local out="$tmpdir/$(basename "$in").signed"
  local extra=()
  local -a ts_args=()

  case "${in##*.}" in
    msi|MSI)
      extra+=(-add-msi-dse)
      ;;
  esac

  # Read NUL-separated args from build_ts_args into an array.
  while IFS= read -r -d '' a; do
    ts_args+=("$a")
  done < <(build_ts_args)

  # Use RFC3161 timestamping (-ts) and SHA-256 digest.
  osslsigncode sign \
    -pkcs12 "$pfx" \
    -readpass "$passfile" \
    -h sha256 \
    -n "$desc" \
    ${url:+-i "$url"} \
    "${ts_args[@]}" \
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
