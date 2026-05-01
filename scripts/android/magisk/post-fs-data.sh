#!/system/bin/sh
set -eu

TRUSTED_PATH="/system/bin:/system/xbin:/vendor/bin:/sbin:/data/adb/magisk:/data/adb/ksu/bin:/data/adb/ap/bin"
PATH="$TRUSTED_PATH"
export PATH

lookup_trusted_command() {
  name="$1"
  candidate="$(PATH="$TRUSTED_PATH" command -v "$name" 2>/dev/null || true)"
  if [ -z "$candidate" ]; then
    echo "$name is required" >&2
    exit 1
  fi
  case "$candidate" in
    */*) ;;
    *)
      echo "refusing non-path command lookup for $name: $candidate" >&2
      exit 1
      ;;
  esac
  if [ ! -x "$candidate" ] || [ -d "$candidate" ]; then
    echo "trusted command is not executable: $candidate" >&2
    exit 1
  fi
  printf '%s\n' "$candidate"
}

MKDIR_BIN="$(lookup_trusted_command mkdir)"
"$MKDIR_BIN" -p /data/adb
