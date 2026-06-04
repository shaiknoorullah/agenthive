# agenthive tmux plugin

This directory holds the TPM-compatible tmux plugin that renders agenthive
notifications in the tmux status line and clears them when the user focuses
the affected pane.

## Layout

- `agenthive.tmux` — the plugin entry script TPM sources. It appends a
  notification segment to `status-right` and installs a `pane-focus-in`
  hook that clears the notification options. Idempotent via the
  `@agenthive-installed` sentinel so re-sourcing is safe.
- `scripts/notification-clear.sh` — helper script invoked from the hook to
  unset every `@notif-*` option in one place.

## How it works

The agenthive daemon writes notification fields into tmux user options:

| Option            | Contents                                      |
| ----------------- | --------------------------------------------- |
| `@notif-msg`      | One-line message body                         |
| `@notif-project`  | Originating project name                      |
| `@notif-source`   | Source name (e.g. `claude-code`)              |
| `@notif-time`     | ISO-8601 timestamp the notification was sent  |
| `@notif-priority` | `low` / `normal` / `critical`                 |

The plugin reads these via the format string

```
#{?@notif-msg,#[fg=yellow] #{@notif-msg}#[default],}
```

which tmux interpolates on every status redraw. No shell forks per render.

When you focus any pane, the `pane-focus-in` hook runs
`scripts/notification-clear.sh`, which unsets every `@notif-*` option so
the status segment renders empty until the next notification arrives.

## Install

### Via TPM (`~/.tmux.conf`)

```tmux
set -g @plugin 'shaiknoorullah/agenthive'
```

Then `prefix + I` to install.

### Manual source (no TPM)

```tmux
run-shell ~/path/to/agenthive/tmux/agenthive.tmux
```

Both paths are idempotent — re-sourcing the plugin will not duplicate the
status-right segment thanks to the `@agenthive-installed` sentinel option.

## Uninstall

```tmux
tmux set-option -gu @agenthive-installed
tmux set-hook   -gu pane-focus-in
```

(You will also want to remove the notification segment from `status-right`
if you'd previously sourced the plugin.)

## Testing

Unit tests in `tmux_test.go` run a `bash -n` syntax check across every
shell file and assert the plugin wires up the expected hooks. Live tmux
integration tests are out of scope for CI — run them locally with
`tmux -V` available.
