#!/bin/bash
# Fast stop: terminate dhp-registry started by fast_start.sh.

set -e
cd "$(dirname "$0")/.."  # cd to package root (parent of deploy/)

PIDFILE="dhp.pid"

stop_by_pidfile() {
  local pid
  pid=$(cat "$PIDFILE")
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid"
    for i in 1 2 3 4 5 6 7 8 9 10; do
      kill -0 "$pid" 2>/dev/null || { echo "stopped, pid=$pid"; rm -f "$PIDFILE"; return 0; }
      sleep 1
    done
    echo "graceful shutdown timed out, sending SIGKILL"
    kill -9 "$pid" 2>/dev/null || true
    rm -f "$PIDFILE"
    return 0
  fi
  echo "pidfile exists but process not running"
  rm -f "$PIDFILE"
}

if [ -f "$PIDFILE" ]; then
  stop_by_pidfile
  exit 0
fi

# Fallback: match by command line.
if pkill -f './dhp-registry --config config.yaml'; then
  echo "stopped via pkill"
else
  echo "not running"
fi
