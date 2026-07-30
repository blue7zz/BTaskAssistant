#!/bin/sh

set -eu

wails_pid=$PPID
watcher_process_group=$$

stop_when_wails_exits() {
  while kill -0 "$wails_pid" 2>/dev/null; do
    sleep 1
  done

  kill -TERM -- "-$watcher_process_group" 2>/dev/null || true
}

stop_when_wails_exits &
monitor_pid=$!

stop_monitor() {
  kill "$monitor_pid" 2>/dev/null || true
  wait "$monitor_pid" 2>/dev/null || true
}

trap stop_monitor EXIT HUP INT TERM

if [ -n "${BTA_VITE_PORT:-}" ]; then
  pnpm exec vite --host 0.0.0.0 --port "$BTA_VITE_PORT"
else
  pnpm exec vite --host 0.0.0.0
fi
