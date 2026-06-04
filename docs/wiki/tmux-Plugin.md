# tmux Plugin

The tmux plugin gives agenthive a status-line surface with zero shell forks on the hot path. Notifications are written as native per-pane tmux options and rendered via the tmux format-string engine — no `bash -c` between every status refresh.

## Install via TPM

If you use [TPM](https://github.com/tmux-plugins/tpm):

```tmux
# In ~/.tmux.conf
set -g @plugin 'shaiknoorullah/agenthive'
```

Reload tmux config (`prefix + r` or `tmux source-file ~/.tmux.conf`), then:

```
prefix + I
```

to install. TPM clones the repo and sources `tmux/agenthive.tmux`.

## Install manually (no TPM)

```bash
git clone https://github.com/shaiknoorullah/agenthive.git ~/.tmux/plugins/agenthive
echo "run-shell ~/.tmux/plugins/agenthive/tmux/agenthive.tmux" >> ~/.tmux.conf
tmux source-file ~/.tmux.conf
```

Or from the v0.1.0 release tarball (the tarball ships the `tmux/` directory):

```bash
curl -L https://github.com/shaiknoorullah/agenthive/releases/download/v0.1.0/agenthive_0.1.0_linux_amd64.tar.gz \
  | tar -xz -C /tmp
mkdir -p ~/.tmux/plugins/agenthive
cp -r /tmp/tmux ~/.tmux/plugins/agenthive/
echo "run-shell ~/.tmux/plugins/agenthive/tmux/agenthive.tmux" >> ~/.tmux.conf
tmux source-file ~/.tmux.conf
```

## What it does

When you source the plugin:

1. Appends a notification format-string snippet to your `status-right`, idempotently (a sentinel `@agenthive-installed` option prevents duplicate installs on re-source).
2. Installs a `pane-focus-in` hook that clears the notification options for the focused pane.

The status-line snippet looks like:

```
#{?@notif-msg,#[fg=yellow] #{@notif-msg}#[default],}
```

When the daemon writes `@notif-msg` to a pane, the status bar lights up. When you focus the pane, the hook clears the option, the status bar reverts. Zero shell forks at any step.

## What the daemon writes

The agenthive daemon's tmux surface invokes `tmux set-option -g` with these option names per notification:

| Option | Contents |
|---|---|
| `@notif-msg` | The notification body |
| `@notif-project` | Project label, if present |
| `@notif-source` | Source agent (`claude-code`, `codex-cli`, …) |
| `@notif-time` | ISO-8601 timestamp |
| `@notif-priority` | `info` / `warning` / `critical` |

For action requests, additional options are set: `@notif-action-id`, `@notif-action-tool`. You can extend your `status-right` to surface those too.

## Customize the format string

If you don't want the default snippet, set `@agenthive-status-fmt` before sourcing the plugin:

```tmux
set -g @agenthive-status-fmt '#{?@notif-msg,⚠ #{@notif-msg},}'
set -g @plugin 'shaiknoorullah/agenthive'
```

(The plugin's idempotency check picks up your value.)

## Customize per-priority colors

Use `#{?@notif-priority,…}` matchers:

```tmux
set -g @agenthive-status-fmt '#{?@notif-msg,#{?#{==:#{@notif-priority},critical},#[fg=red#,bold],#[fg=yellow]} #{@notif-msg}#[default],}'
set -g @plugin 'shaiknoorullah/agenthive'
```

## Uninstall

Remove the plugin line from `.tmux.conf`, kill the tmux server, and unset the sentinel:

```bash
tmux kill-server
```

Next start, agenthive's status snippet is gone.

## Surface activation

The agenthive daemon enables the tmux surface only if `tmux` is on `$PATH`. If you run the daemon from outside a tmux server (e.g., as a systemd unit), set `$PATH` to include where `tmux` lives. Otherwise the surface silently no-ops and your other surfaces handle dispatch.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Status bar never lights up | Daemon ran before tmux was started, or no `tmux` on `$PATH` | Restart `agenthive start` after tmux is running |
| Status bar stays lit forever | `pane-focus-in` hook overwritten by another plugin | Make sure agenthive's hook is the LAST `set-hook -g pane-focus-in` your config sets, or use `-a` to append |
| Plugin re-installs the format on every reload | Sentinel got cleared | Don't `set -gu @agenthive-installed` anywhere in your config |
| Format breaks colors in other tmux plugins | Your other plugins use `#[default]` before agenthive's snippet | Re-order plugin loading so agenthive is last |

See [[Troubleshooting]] for daemon-side issues.

## See also

- [[CLI Reference]] — the daemon side of the surface
- [[Architecture]] — where the tmux surface sits in the dispatch path
- [[Routing]] — send only specific notifications to the tmux surface
