#!/system/bin/sh
set -eu

MODDIR=${MODPATH:-${0%/*}}
PIDFILE="/data/adb/gov-pass.pid"
QUEUE_NUM=100
MARK=1

if [ -f "/data/adb/gov-pass.conf" ]; then
  . "/data/adb/gov-pass.conf"
fi

"$MODDIR/iptables_del.sh" --queue-num "$QUEUE_NUM" --mark "$MARK" >/dev/null 2>&1 || true

if [ -f "$PIDFILE" ]; then
  kill "$(cat "$PIDFILE")" >/dev/null 2>&1 || true
  rm -f "$PIDFILE"
fi
