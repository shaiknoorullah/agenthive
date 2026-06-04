#!/usr/bin/env bash
# agenthive.tmux — TPM-compatible tmux plugin entry script.
#
# Responsibilities (per docs/superpowers/plans/2026-06-04-v0.1.0.md L5.C):
#
#   1. Append a notification segment to status-right that renders
#      whatever the agenthive daemon writes into the user option
#      `@notif-msg`. Append is idempotent — guarded by the
#      `@agenthive-installed` sentinel so re-sourcing (TPM's
#      "install plugins" cycle, or a manual `source-file`) does not
#      duplicate the segment.
#
#   2. Install a `pane-focus-in` hook that runs the helper script
#      `scripts/notification-clear.sh`. Focusing a pane is the user
#      acknowledging the notification, so we unset every `@notif-*`
#      option to clear the status line.
#
# The plugin only renders. The agenthive daemon (run separately) is what
# writes the `@notif-*` options the plugin reads. None of these commands
# fork shells per render: tmux interpolates the format string itself.

set -eu

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLEAR_SCRIPT="${CURRENT_DIR}/scripts/notification-clear.sh"

# Sentinel guard. tmux's `show-option -gqv` returns the value silently
# (empty + zero exit) when unset, so the conditional below stays simple.
sentinel="$(tmux show-option -gqv '@agenthive-installed' 2>/dev/null || true)"
if [ "${sentinel}" = "1" ]; then
	# Re-sourcing the plugin must be a no-op.
	exit 0
fi

# 1. Status-right append.
#
# The segment uses tmux's `#{?cond,then,else}` conditional so it only
# renders when `@notif-msg` is set. We append rather than overwrite so we
# co-exist with whatever the user already has in their status-right.
current_status_right="$(tmux show-option -gv status-right 2>/dev/null || true)"
notif_segment='#{?@notif-msg,#[fg=yellow] #{@notif-msg}#[default],}'

case "${current_status_right}" in
*"${notif_segment}"*)
	# Already present (e.g. user copy-pasted it manually). Skip.
	;;
*)
	tmux set-option -g status-right "${current_status_right}${notif_segment}"
	;;
esac

# 2. pane-focus-in hook → clear all @notif-* options.
#
# We delegate to the helper script so the unset list lives in one place
# (and stays in sync with whatever the daemon writes).
tmux set-hook -g pane-focus-in "run-shell '${CLEAR_SCRIPT}'"

# 3. Mark ourselves installed. Setting this last means a partial failure
# above does not poison future re-runs.
tmux set-option -g '@agenthive-installed' 1
