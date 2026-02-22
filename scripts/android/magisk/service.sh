#!/system/bin/sh
set -eu

MODDIR=${0%/*}
CONFIG="/data/adb/gov-pass.conf"
PIDFILE="/data/adb/gov-pass.pid"
LOGFILE="/data/adb/gov-pass.log"

QUEUE_NUM=100
MARK=1
EXTRA_ARGS=""

if [ -f "$CONFIG" ]; then
  . "$CONFIG"
fi

if [ -d "$MODDIR/lib" ]; then
  export LD_LIBRARY_PATH="$MODDIR/lib:$LD_LIBRARY_PATH"
fi

"$MODDIR/iptables_add.sh" --queue-num "$QUEUE_NUM" --mark "$MARK" >>"$LOGFILE" 2>&1

"$MODDIR/splitter" --queue-num "$QUEUE_NUM" --mark "$MARK" $EXTRA_ARGS >>"$LOGFILE" 2>&1 &
echo $! > "$PIDFILE"
