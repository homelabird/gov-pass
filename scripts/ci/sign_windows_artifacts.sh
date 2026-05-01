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
      echo "sign_windows_artifacts: refusing symlinked trusted command for $name: $candidate" >&2
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
  echo "sign_windows_artifacts: required command not found in trusted directories: $name" >&2
  exit 1
}

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
url_args=()

validate_http_url() {
  local label="$1"
  local value="$2"
  case "$value" in
    http://*|https://*)
      ;;
    *)
      echo "sign_windows_artifacts: ${label} must start with http:// or https://: $value" >&2
      exit 1
      ;;
  esac
}

if [[ -n "$url" ]]; then
  validate_http_url "WINDOWS_CODESIGN_URL" "$url"
  url_args=(-i "$url")
fi

MKTEMP_BIN="$(lookup_trusted_command mktemp)"
RM_BIN="$(lookup_trusted_command rm)"
BASE64_BIN="$(lookup_trusted_command base64)"
CHMOD_BIN="$(lookup_trusted_command chmod)"
MV_BIN="$(lookup_trusted_command mv)"
OSSLSIGNCODE_BIN="$(lookup_trusted_command osslsigncode)"

tmpdir="$("$MKTEMP_BIN" -d)"
cleanup() { "$RM_BIN" -rf -- "$tmpdir"; }
trap cleanup EXIT

pfx="$tmpdir/codesign.pfx"
passfile="$tmpdir/pass.txt"

printf '%s' "$WINDOWS_CODESIGN_PFX_B64" | "$BASE64_BIN" -d >"$pfx"
printf '%s' "$WINDOWS_CODESIGN_PFX_PASSWORD" >"$passfile"
"$CHMOD_BIN" 600 "$pfx" "$passfile"

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
      validate_http_url "timestamp URL" "$u"
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
  local base="${in##*/}"
  local out="$tmpdir/${base}.signed"
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
  "$OSSLSIGNCODE_BIN" sign \
    -pkcs12 "$pfx" \
    -readpass "$passfile" \
    -h sha256 \
    -n "$desc" \
    "${url_args[@]}" \
    "${ts_args[@]}" \
    "${extra[@]}" \
    -in "$in" \
    -out "$out" >/dev/null

  "$MV_BIN" -f -- "$out" "$in"
}

for f in "$@"; do
  if [[ -L "$f" ]]; then
    echo "sign_windows_artifacts: refusing to sign through symlink: $f" >&2
    exit 1
  fi
  if [[ ! -f "$f" ]]; then
    echo "sign_windows_artifacts: file not found: $f" >&2
    exit 1
  fi
  sign_one "$f"
done
