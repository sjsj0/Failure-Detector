#!/usr/bin/env bash
#set -euo pipefail

TMUX_SESSION="cs-425-shared-mp2"
TMUX_SOCKET="/tmp/tmux-cs-425-mp2.sock"

echo "=== Killing MP2 tmux on $(hostname -f 2>/dev/null || hostname) ==="

# Kill just the MP2 session if it exists
if sudo tmux -S "$TMUX_SOCKET" has-session -t "$TMUX_SESSION" 2>/dev/null; then
  echo "Killing tmux session: $TMUX_SESSION (socket: $TMUX_SOCKET)"
  sudo tmux -S "$TMUX_SOCKET" kill-session -t "$TMUX_SESSION" || true
else
  echo "No tmux session named $TMUX_SESSION found (socket: $TMUX_SOCKET)"
fi

# Then shut down the tmux server bound to that socket (safe if already gone)
echo "Killing tmux server at socket: $TMUX_SOCKET (if running)"
sudo tmux -S "$TMUX_SOCKET" kill-server 2>/dev/null || true

# Optional: clean up stale socket file if tmux is not running
if [[ -S "$TMUX_SOCKET" ]]; then
  echo "Removing stale socket: $TMUX_SOCKET"
  sudo rm -f "$TMUX_SOCKET" || true
fi

echo "=== Done ==="
