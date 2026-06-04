#!/usr/bin/env bash
# agenthive.tmux — TPM-compatible plugin entry script.
#
# Skeleton only. The real plugin (in a follow-up commit) will:
#   - Append a status-right segment that renders @notif-msg.
#   - Install a pane-focus-in hook that unsets every @notif-* option.
#   - Mark itself installed via the @agenthive-installed sentinel so
#     re-sourcing is safe.
#
# For now this script just announces itself so TPM and CI smoke tests can
# verify the file is syntactically valid (bash -n) and executable.

set -eu

echo "not implemented: tmux/agenthive.tmux"
