#!/usr/bin/env bash
# notification-clear.sh — clears every @notif-* tmux user option.
#
# Invoked from the `pane-focus-in` hook installed by agenthive.tmux. The
# user focusing a pane is treated as acknowledgement of the active
# notification, so we wipe the option namespace the daemon writes into.
#
# `set -gu` is the global-unset form of `set-option`; it removes the
# option from the server-global table so the status-right conditional
# (`#{?@notif-msg,...}`) renders empty until the daemon writes again.
#
# Failures are tolerated per-option: a missing option is not an error
# worth aborting the focus event for, so each unset is independent.

set -u

tmux set -gu '@notif-msg'      2>/dev/null || true
tmux set -gu '@notif-project'  2>/dev/null || true
tmux set -gu '@notif-source'   2>/dev/null || true
tmux set -gu '@notif-time'     2>/dev/null || true
tmux set -gu '@notif-priority' 2>/dev/null || true

# Action prompt options used by Surface.DispatchAction. Clearing them on
# focus matches the message-clear behaviour above.
tmux set -gu '@notif-action-id'   2>/dev/null || true
tmux set -gu '@notif-action-tool' 2>/dev/null || true

exit 0
