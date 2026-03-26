# Position Paper: Native tmux-Integrated Notification Architecture

**Author:** Native tmux Approach Advocate
**Date:** 2026-03-26
**Status:** RFC / Technical Debate Position
**Target:** tmux-agent-notifications plugin redesign

---

## Executive Summary

The current file-based notification system in tmux-agent-notifications suffers from fundamental architectural flaws: race conditions between dual-file writes, a shell fork on every status refresh cycle, O(n) directory scans on every pane focus event, and a violation of data/presentation separation by baking tmux format codes into data files. tmux itself provides a complete, purpose-built infrastructure for exactly this problem -- user options for atomic per-pane data storage, native format string conditionals for presentation, built-in hooks for event-driven updates, and `wait-for` channels for synchronization. Migrating to these native primitives eliminates entire categories of bugs, reduces status refresh cost from "shell fork + directory listing + file reads" to a single in-process format string expansion, and produces a system that is both simpler to reason about and inherently correct under concurrency.

---

## 1. Architecture Flaws in the Current File-Based Approach

### 1.1 The Dual-File Race Condition

The core of the current design writes two files per notification:

```bash
# From tmux-notify-lib.sh, lines 73-74
echo "#[fg=${NOTIF_FG}]..." > "$NOTIF_DIR/$notif_key"
echo "${TMUX_PANE}" > "$NOTIF_DIR/.pane_$notif_key"
```

Between these two writes, there exists a window where the notification file exists but its companion `.pane_*` file does not (or vice versa during deletion). If `clear-notification.sh` or `next-notification.sh` runs in this gap -- which is triggered by any pane focus event via tmux hooks -- it will encounter an orphaned file. The `notification-reader.sh` can display a notification that has no pane association, making "jump to notification" fail silently. No file locking is employed anywhere in the codebase.

### 1.2 Shell Fork on Every Status Refresh

The plugin configures the second status line as:

```bash
# From claude-notifications.tmux, line 33
tmux set -g status-format[1] "#($SCRIPTS_DIR/notification-reader.sh)"
```

The `#()` construct causes tmux to fork a shell and execute `notification-reader.sh` on every status refresh interval (default: 15 seconds, but often configured to 1-5 seconds). Each invocation performs: a shell startup, `ls -t` on the notification directory, a `grep` pipeline to filter files, `wc -l` for counting, a `tmux display-message -p` for terminal width, and then a `while` loop that `cat`s each notification file and runs `sed` to strip format codes for width calculation. For a developer with 5 active notifications, this is 5 file reads, 5 sed invocations, and multiple subshell forks -- every few seconds, even when nothing has changed.

### 1.3 O(n) Scan on Every Focus Event

```bash
# From clear-notification.sh, lines 11-29
for pane_file in "$NOTIF_DIR"/.pane_*; do
    [ -f "$pane_file" ] || continue
    STORED_PANE=$(cat "$pane_file")
    # ... stat for age, grep for liveness ...
done
```

Three hooks trigger this script: `client-session-changed`, `client-focus-in`, and `pane-focus-in`. Every pane switch in every session iterates every `.pane_*` file, `cat`s its content, runs `tmux list-panes -a` to check liveness, and `stat`s for age. With 10 notifications across 3 sessions, a single Alt-Tab triggers 10 file reads, 10 process liveness checks, and 10 stat calls. This cost is paid regardless of whether any notification is related to the focused pane.

### 1.4 Data/Presentation Coupling

Notification files contain pre-rendered tmux format strings:

```
#[fg=#c8d3f5][14:32:01] Claude/my-project: #[fg=yellow,bold]Agent has finished #[default]
```

This means the data file is only useful to tmux's status line renderer. It cannot be parsed by `notification-picker.sh` without stripping format codes (line 13: `sed 's/#\[[^]]*\]//g'`). It cannot be consumed by any external tool. The format is determined at write time, not display time, so changing the color scheme requires clearing and re-triggering all active notifications.

### 1.5 Additional Fragilities

- **JSON parsing via grep/sed** (`claude-hook.sh`, line 24): `grep -o '"$1"[[:space:]]*:[[:space:]]*"[^\"]*"'` breaks on escaped quotes, nested objects, multi-line values, and null values.
- **TIMESTAMP captured at source-time** (`tmux-notify-lib.sh`, line 18): `TIMESTAMP=$(date '+%H:%M:%S')` captures the time when the library is sourced, not when `tmux_alert` is called. If any processing occurs between sourcing and alerting, the timestamp is stale.
- **`visible_len` uses fragile regex** (`notification-reader.sh`, line 16): `sed 's/#\[[^]]*\]//g'` fails on nested brackets or format codes containing `]`.
- **Unbounded log growth** (`events.log`): No rotation, no size limit, no cleanup.
- **`sed -i` portability** (`codex/install.sh`, line 25): GNU `sed -i` and BSD `sed -i` have incompatible syntax for in-place editing. The current `.bak` workaround still leaves behavioral differences.

---

## 2. tmux Native Capabilities That Solve These Problems

### 2.1 Per-Pane User Options as Atomic Data Store

tmux supports user options (prefixed with `@`) that can be scoped to server (`-s`), session (default), window (`-w`), or pane (`-p`). These are set and read atomically through the tmux server's single-threaded command queue:

```bash
# Store notification data on the pane itself
tmux set -p -t "%42" @notif-msg "Agent has finished"
tmux set -p -t "%42" @notif-project "my-project"
tmux set -p -t "%42" @notif-time "14:32:01"
tmux set -p -t "%42" @notif-source "Claude"

# Read it back
tmux show -p -t "%42" -v @notif-msg
# => "Agent has finished"

# Clear it
tmux set -p -t "%42" -u @notif-msg
```

This eliminates the dual-file race condition entirely. There is no filesystem I/O. The data is intrinsically tied to the pane's lifecycle -- when the pane is destroyed, its options vanish with it, eliminating the need for stale-notification cleanup.

### 2.2 Native Format String Conditionals for Presentation

tmux's format system supports conditionals, comparisons, and string operations directly in status-format strings:

```bash
# Display notification only if @notif-msg is set on any pane
tmux set -g status-format[1] \
  "#{?@notif-msg,#[fg=#c8d3f5][#{@notif-time}] #{@notif-source}/#{@notif-project}: #[fg=yellow#,bold]#{@notif-msg}#[default],}"
```

This format string is evaluated entirely within the tmux server process. No shell fork. No file I/O. The presentation is defined in the format string; the data is stored in user options. Changing colors requires only changing the format string, not rewriting notification files.

### 2.3 Built-in Hooks for Event-Driven Architecture

The current implementation uses three hooks but processes them all with the same expensive shell script. A native approach can use tmux's own conditional commands:

```bash
# Clear notification when user focuses the pane - no shell fork needed
tmux set-hook -g pane-focus-in \
  'set -p -u @notif-msg ; set -p -u @notif-project ; set -p -u @notif-time ; set -p -u @notif-source ; refresh-client -S'
```

This runs entirely within tmux's command queue. No `run-shell`, no file operations, no process creation. The `set -p -u` commands operate on the currently focused pane, which is exactly the pane whose notification we want to clear.

### 2.4 `wait-for` Channels for Synchronization

For cases where external scripts need to coordinate with tmux state changes, `wait-for` provides named channels with lock semantics:

```bash
# Writer (agent hook):
tmux wait-for -L notif-lock
tmux set -p -t "$TMUX_PANE" @notif-msg "Agent has finished"
tmux set -p -t "$TMUX_PANE" @notif-time "$(date +%H:%M:%S)"
tmux wait-for -U notif-lock
tmux refresh-client -S

# Any reader that needs consistency:
tmux wait-for -L notif-lock
msg=$(tmux show -p -t "$target" -v @notif-msg)
time=$(tmux show -p -t "$target" -v @notif-time)
tmux wait-for -U notif-lock
```

This provides mutual exclusion without file locks, PID files, or `flock`.

---

## 3. Atomicity and Correctness

tmux processes all commands through a single-threaded server via client command queues. When a command is submitted to the tmux server (via `tmux set`, `tmux show`, etc.), it is serialized through this queue. This means:

- **`set -p @notif-msg "value"`** is atomic with respect to any concurrent `show -p @notif-msg` -- there is no partially-written state.
- **Multiple `set` commands chained with `;`** in a single tmux command string execute sequentially within the same queue entry, providing transactional semantics.
- **Hook commands** execute atomically within the server's event loop.

Contrast this with the file-based approach where two separate `echo > file` commands are two separate filesystem operations with no transactional guarantee. Between them, any process can observe an inconsistent state. The OS provides no ordering guarantee between the notification file write and the pane file write, even on the same filesystem.

---

## 4. Performance Analysis

### Status Refresh Cost Comparison

| Operation | File-Based (current) | Native tmux |
|-----------|---------------------|-------------|
| Status refresh | Fork shell + ls + grep + wc + N x (cat + sed) | In-process format expansion |
| Process count per refresh | 5 + 2N (N = notification count) | 0 |
| Pane focus clear | Fork shell + ls + N x (cat + stat + grep) | 0 (inline `set -u`) |
| Notification write | 2 file writes (no lock) | 1 tmux command (atomic) |
| Notification read | File I/O + cat | In-process option lookup |
| Stale cleanup needed | Yes (orphan files, age checks) | No (pane lifecycle) |

For a typical setup with 5 notifications and a 2-second status refresh interval, the file-based approach creates approximately 7-8 processes every 2 seconds (shell, ls, grep, wc, plus cat per file). Over an 8-hour workday, that is roughly 100,000 unnecessary process forks. The native approach: zero.

### Pane Switch Cost

Current implementation on every pane switch: 1 `run-shell` fork + `tmux list-panes -a` + N file reads + N stat calls + potential N file deletions + 1 `refresh-client`. Native approach: the hook runs `set -p -u @notif-msg` and `refresh-client -S` as inline tmux commands with no process creation.

---

## 5. Data/Presentation Separation

The native approach enforces a clean separation:

**Data layer** (per-pane user options):
```
@notif-msg     = "Agent has finished"
@notif-project = "my-project"
@notif-source  = "Claude"
@notif-time    = "14:32:01"
```

**Presentation layer** (status-format string):
```
#{?@notif-msg,#[fg=#c8d3f5][#{@notif-time}] #{@notif-source}/#{@notif-project}: #[fg=yellow,bold]#{@notif-msg}#[default],}
```

Benefits:
- **Theme changes** require modifying only the format string, not rewriting data.
- **Programmatic access** to notification data is trivial: `tmux show -p -t "%42" -v @notif-msg` returns clean text.
- **The picker/navigator** can enumerate panes with notifications: `tmux list-panes -a -F '#{?@notif-msg,#{pane_id}:#{@notif-msg},}'` returns only panes with active notifications, no file system scan required.
- **Multiple presentation formats** (status bar, popup, display-message) can all consume the same data.

---

## 6. Concrete Architecture Proposal

### 6.1 Notification Write (Agent Hook)

```bash
#!/usr/bin/env bash
# claude-hook.sh (simplified)
HOOK_EVENT="$1"
[ ! -t 0 ] && JSON_DATA=$(cat)

# Parse with jq if available, fallback to grep
if command -v jq >/dev/null 2>&1; then
    CWD=$(echo "$JSON_DATA" | jq -r '.cwd // empty')
else
    CWD=$(echo "$JSON_DATA" | grep -o '"cwd"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:.*"\(.*\)"/\1/')
fi

PROJECT=$(basename "${CWD:-unknown}")
TIMESTAMP=$(date '+%H:%M:%S')

case "$HOOK_EVENT" in
    "Stop")
        tmux set -p @notif-msg "Agent has finished"
        tmux set -p @notif-project "$PROJECT"
        tmux set -p @notif-source "Claude"
        tmux set -p @notif-time "$TIMESTAMP"
        tmux refresh-client -S
        ;;
    "UserPromptSubmit"|"SessionEnd")
        tmux set -p -u @notif-msg
        tmux set -p -u @notif-project
        tmux set -p -u @notif-source
        tmux set -p -u @notif-time
        tmux refresh-client -S
        ;;
esac
```

### 6.2 Status Line Display (No Shell Fork)

```bash
# In claude-notifications.tmux -- build a format string that aggregates
# notifications from all panes. Because tmux's format system can iterate
# panes, we can construct the display purely in format-land.
#
# For the simple (and most common) case of displaying the current pane's
# notification:
tmux set -g status-format[1] \
  '#[align=left]#{W:#{P:#{?@notif-msg,#[fg=#{@claude-notif-fg}][#{@notif-time}] #{@notif-source}/#{@notif-project}: #[fg=#{@claude-notif-alert-fg}#,#{@claude-notif-alert-style}]#{@notif-msg}#[default]  ,}}}'
```

The `#{W:...}` iterates windows and `#{P:...}` iterates panes within each window. The inner `#{?@notif-msg,...,}` conditional renders the notification only for panes that have one. This entire expansion happens inside the tmux server -- zero forks, zero file I/O.

### 6.3 Notification Clearing (Inline Hook)

```bash
# Clears notification on the pane when the user focuses it
tmux set-hook -g pane-focus-in \
  'if -F "#{@notif-msg}" "set -p -u @notif-msg ; set -p -u @notif-project ; set -p -u @notif-source ; set -p -u @notif-time ; refresh-client -S"'
```

The `if -F` conditional checks whether the focused pane has a notification before doing anything. No shell fork. No file I/O. No iteration over unrelated panes.

### 6.4 Jump-to-Notification (Next Notification)

```bash
#!/usr/bin/env bash
# next-notification.sh
TARGET=$(tmux list-panes -a -F '#{?@notif-msg,#{pane_id},}' | head -1)
if [ -z "$TARGET" ]; then
    tmux display-message "No pending agents"
    exit 0
fi
tmux switch-client -t "$TARGET"
tmux select-pane -t "$TARGET"
# Clearing happens automatically via pane-focus-in hook
```

### 6.5 Notification Picker (Popup)

```bash
#!/usr/bin/env bash
# notification-picker.sh
NOTIFS=$(tmux list-panes -a \
  -F '#{?@notif-msg,#{pane_id}	[#{@notif-time}] #{@notif-source}/#{@notif-project}: #{@notif-msg},}' \
  | grep -v '^$')

[ -z "$NOTIFS" ] && echo "No notifications" && sleep 1 && exit 0

SELECTED=$(echo "$NOTIFS" | fzf --reverse --header='Jump to notification' --delimiter='	' --with-nth=2)
[ -z "$SELECTED" ] && exit 0

PANE_ID=$(echo "$SELECTED" | cut -f1)
tmux switch-client -t "$PANE_ID"
tmux select-pane -t "$PANE_ID"
```

No file reads. No sed to strip format codes. Clean tab-separated data directly from tmux.

---

## 7. Addressing File-Based Approach Strengths

The file-based advocate will raise several valid points. Here are direct responses:

### "Files are simple and easy to debug"

User options are equally inspectable:
```bash
# List all panes with notifications
tmux list-panes -a -F '#{pane_id} #{@notif-msg} #{@notif-project} #{@notif-time}'

# Inspect a specific pane
tmux show -p -t "%42" @notif-msg

# Manually set a test notification
tmux set -p -t "%42" @notif-msg "Test notification"
```

These are arguably easier to debug than navigating to `~/.tmux-notifications/`, parsing filenames that encode pane IDs with double-underscore separators, and cross-referencing `.pane_*` companion files.

### "Files persist across tmux restarts"

Notifications for AI agent tasks are ephemeral by nature. The prompt says they arrive "minutes apart, not seconds." When tmux restarts, all agent processes are gone too -- their notifications are meaningless. Persistence is not a feature; it is a source of stale state that the current code must actively clean up (the 30-minute age check in `clear-notification.sh`).

### "Files allow external tools to read notifications"

The native approach actually improves external access. Any script can run `tmux list-panes -a -F '#{@notif-msg}'` to get clean notification text. The file-based approach returns format-code-polluted strings that require sed post-processing. If a structured export is needed, a simple wrapper produces clean JSON:

```bash
tmux list-panes -a -F '{"pane":"#{pane_id}","msg":"#{@notif-msg}","project":"#{@notif-project}","time":"#{@notif-time}"}' | jq -s '[.[] | select(.msg != "")]'
```

### "The file approach works outside tmux sessions"

Agent hooks (`claude-hook.sh`, `codex/notify.sh`) always run inside a tmux pane -- `$TMUX_PANE` is required for the current implementation too. There is no use case where a notification is written from outside tmux.

### "Multiple processes can write to the directory concurrently"

This is presented as a strength but is actually the primary source of bugs. Multiple concurrent writers to the same directory without locking is precisely why the dual-file write has a race condition. The tmux server's single-threaded command queue provides correct serialization without any effort from the plugin author.

---

## 8. Risks and Mitigations

### Risk 1: Format String Complexity
**Concern:** Nested `#{W:#{P:#{?...}}}` format strings are hard to read and maintain.
**Mitigation:** Build the format string programmatically in the plugin init script. Store sub-expressions in well-named shell variables. Document the format string structure. The complexity is confined to one location (`claude-notifications.tmux`) rather than scattered across 6 shell scripts.

### Risk 2: tmux Version Compatibility
**Concern:** Per-pane user options (`set -p @option`) require tmux 3.1+; `#{P:}` pane iteration in formats requires tmux 3.2+.
**Mitigation:** The plugin already targets tmux 3.2+. Pane-scoped user options have been stable since tmux 3.1 (released June 2020). Add a version check at plugin init:
```bash
TMUX_VERSION=$(tmux -V | grep -oE '[0-9]+\.[0-9]+')
if [ "$(echo "$TMUX_VERSION < 3.2" | bc)" -eq 1 ]; then
    tmux display-message "tmux-agent-notifications requires tmux 3.2+"
    exit 1
fi
```

### Risk 3: User Option Namespace Collisions
**Concern:** Other plugins might use `@notif-msg` or similar option names.
**Mitigation:** Use a unique prefix: `@agent-notif-msg`, `@agent-notif-project`, etc. User options are namespaced only by convention, but the `@agent-notif-` prefix is distinctive enough to avoid collisions with other known plugins.

### Risk 4: Loss of Event Log
**Concern:** The file-based approach has `events.log` for historical review.
**Mitigation:** Retain a lightweight logging function that appends to a log file. Logging is orthogonal to the notification display mechanism. The native approach does not prevent logging -- it eliminates file-based notification storage, not all file I/O. Add `logrotate`-style size capping:
```bash
log_event() {
    local log="$HOME/.tmux-notifications/events.log"
    local max_size=102400  # 100KB
    [ -f "$log" ] && [ "$(wc -c < "$log")" -gt "$max_size" ] && tail -100 "$log" > "$log.tmp" && mv "$log.tmp" "$log"
    echo "[$(date '+%H:%M:%S')] $*" >> "$log"
}
```

### Risk 5: Aggregating Notifications Across All Panes in Status Bar
**Concern:** The `#{W:#{P:...}}` approach may produce verbose output for many notifications.
**Mitigation:** tmux's format system supports string length operations and truncation. Additionally, the existing width-limiting logic can be preserved by moving it to a lightweight helper that is only invoked when a notification actually changes (via a hook-triggered `run-shell`), rather than on every status refresh.

---

## 9. Decision Matrix

| Criterion | File-Based (Current) | Native tmux (Proposed) | Winner |
|-----------|---------------------|----------------------|--------|
| **Atomicity** | No locking; dual-file race condition | Single-threaded server queue; atomic set/unset | Native |
| **Status refresh cost** | Shell fork + N file reads per interval | Zero-cost format expansion | Native |
| **Focus event cost** | O(n) file scan + stat per file | O(1) inline option unset | Native |
| **Data/presentation separation** | Format codes baked into data files | Clean data in options, format in status string | Native |
| **Stale notification cleanup** | Manual age checks, orphan detection | Automatic (pane destruction clears options) | Native |
| **Debuggability** | `ls ~/.tmux-notifications/` + cat files | `tmux list-panes -a -F '#{@notif-msg}'` | Tie |
| **External tool access** | Requires sed to strip format codes | Clean text via `tmux show` | Native |
| **Concurrency safety** | Unsafe (no locking) | Safe (server serialization + wait-for) | Native |
| **Portability (macOS/Linux)** | `stat` flag differences, `sed -i` issues | tmux commands are platform-independent | Native |
| **Persistence across restart** | Files survive restart (stale state) | Options cleared (correct behavior) | Native |
| **Implementation complexity** | 8 shell scripts, ~250 lines | 3-4 shell scripts, ~120 lines | Native |
| **Learning curve** | Familiar file I/O patterns | tmux format string syntax | File-Based |
| **Dependency count** | ls, cat, grep, sed, wc, stat | tmux only | Native |

**Score: Native 12, File-Based 1, Tie 1**

---

## 10. Conclusion

The file-based notification system was a reasonable first implementation, but it fights against tmux rather than working with it. Every problem in the current codebase -- race conditions, performance overhead, data/presentation coupling, platform portability, stale state management -- traces back to the decision to store notification state outside of tmux. The tmux server already maintains per-pane state, already provides atomic operations, already evaluates format strings without forking, and already cleans up state when panes are destroyed. The native approach does not introduce exotic or unstable tmux features; it uses user options (stable since tmux 3.1), format conditionals (stable since tmux 2.6), and hooks (stable since tmux 2.4). The result is a system that is simultaneously simpler (fewer scripts, fewer moving parts), more correct (no race conditions, no stale state), and more performant (zero forks on status refresh, O(1) focus handling).

The strongest argument for files -- that they are "simple" -- is belied by the 250+ lines of shell code required to manage them correctly (and the bugs that remain). True simplicity is letting tmux manage tmux state.
