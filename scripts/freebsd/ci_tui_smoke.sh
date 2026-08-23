#!/usr/bin/env sh
set -eu

SERVICE_NAME="${SERVICE_NAME:-gov-pass}"
TUI_BIN="${TUI_BIN:-/usr/local/bin/gov-pass-tui}"
RUN_RESTART="${FREEBSD_TUI_SMOKE_RESTART:-0}"

if [ ! -x "$TUI_BIN" ]; then
  printf '%s\n' "TUI binary not found or not executable: $TUI_BIN" >&2
  exit 1
fi

status_output="$("$TUI_BIN" --service-name "$SERVICE_NAME" --action status 2>&1)" || {
  printf '%s\n' "status smoke failed:" >&2
  printf '%s\n' "$status_output" >&2
  exit 1
}

case "$(printf '%s' "$status_output" | tr '[:upper:]' '[:lower:]' | tr -d '\r')" in
  active|inactive)
    ;;
  *)
    printf '%s\n' "unexpected status output: $status_output" >&2
    exit 1
    ;;
esac

if reload_output="$("$TUI_BIN" --service-name "$SERVICE_NAME" --action reload 2>&1)"; then
  printf '%s\n' "reload smoke unexpectedly succeeded on FreeBSD" >&2
  exit 1
fi
printf '%s' "$reload_output" | grep -F "reload is not supported on FreeBSD; use restart" >/dev/null || {
  printf '%s\n' "reload smoke returned unexpected error: $reload_output" >&2
  exit 1
}

if [ "$RUN_RESTART" = "1" ]; then
  "$TUI_BIN" --service-name "$SERVICE_NAME" --action restart >/dev/null
fi

printf '%s\n' "FreeBSD TUI smoke verification passed."
