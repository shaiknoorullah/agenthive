# agenthive tmux plugin

This directory holds the TPM-compatible tmux plugin that renders agenthive
notifications in the tmux status line and clears them when the user focuses
the affected pane.

## Layout

- `agenthive.tmux` — the plugin entry script TPM sources. It installs the
  status-right format segment and the `pane-focus-in` hook that clears the
  notification options. Idempotent via the `@agenthive-installed` sentinel.
- `scripts/notification-clear.sh` — helper script invoked from the hook to
  unset every `@notif-*` option in one place.

## Install

Via TPM (`~/.tmux.conf`):

```tmux
set -g @plugin 'shaiknoorullah/agenthive'
```

Then `prefix + I` to install.

Without TPM, source the script directly from `~/.tmux.conf`:

```tmux
run-shell ~/path/to/agenthive/tmux/agenthive.tmux
```

The plugin only renders. The agenthive daemon (run separately) is what
writes the `@notif-*` options the plugin reads.
