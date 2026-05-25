#!/bin/bash
# Fast start: launch dhp-registry in background with nohup.
# For production, prefer systemd (see deploy/systemd/dhp-registry.service).

set -e
cd "$(dirname "$0")/.."  # cd to package root (parent of deploy/)

BIN="./dhp-registry"
CFG="config.yaml"
LOG="dhp.log"
PIDFILE="dhp.pid"

if [ ! -x "$BIN" ]; then
  echo "error: $BIN not found or not executable (cwd=$(pwd))" >&2
  exit 1
fi

if [ ! -f "$CFG" ]; then
  echo "error: $CFG not found. Copy config.example.yaml to config.yaml first." >&2
  exit 1
fi

if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
  echo "already running, pid=$(cat "$PIDFILE")"
  exit 0
fi

nohup "$BIN" --config "$CFG" > "$LOG" 2>&1 &
echo $! > "$PIDFILE"
sleep 1

if kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
  echo "started, pid=$(cat "$PIDFILE"), log: $(pwd)/$LOG"
else
  echo "failed to start. last log:" >&2
  tail -20 "$LOG" >&2
  rm -f "$PIDFILE"
  exit 1
fi
