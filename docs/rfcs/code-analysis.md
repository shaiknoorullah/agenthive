# tmux-agent-notifications: Deep Code Analysis

## Summary

This document catalogs every bug, race condition, correctness issue, portability problem, and design flaw found in the tmux-agent-notifications plugin. Issues are organized by severity.

---

## Critical Issues

### C1. Race Condition: Non-Atomic File Writes to Notification Files

**File:** `scripts/tmux-notify-lib.sh`, line 73-74
**Severity:** Critical

The notification file is written with a bare `echo > file`, then immediately a second file is written. The `notification-reader.sh` can read the notification file mid-write, producing a truncated or empty line on the status bar.

```bash
echo "#[fg=...]..." > "$NOTIF_DIR/$notif_key"
echo "${TMUX_PANE}" > "$NOTIF_DIR/.pane_$notif_key"
```

**How it manifests:** Two concurrent Claude Code hooks fire simultaneously (e.g., two panes both complete at the same instant). One writes a partial notification file that the reader picks up, displaying garbled tmux formatting escapes on the status line. Worse, if the shell is interrupted between lines 73 and 74, the `.pane_` file is never created, making the notification un-clearable and un-jumpable.

**Suggested fix:** Write to a temporary file and `mv` atomically:
```bash
tmp=$(mktemp "$NOTIF_DIR/.tmp.XXXXXX")
echo "..." > "$tmp"
mv -f "$tmp" "$NOTIF_DIR/$notif_key"
```

---

### C2. Race Condition: Read-Then-Delete in next-notification.sh and clear-notification.sh

**File:** `scripts/next-notification.sh`, lines 8-16; `scripts/clear-notification.sh`, lines 11-17
**Severity:** Critical

Both scripts read a pane file, then conditionally delete files based on the content. Between the `cat` and the `rm`, another invocation (e.g., the `pane-focus-in` hook fires simultaneously on two clients) can read the same file, leading to double-free (benign but wasteful) or, worse, a TOCTOU (time-of-check-time-of-use) race where one process acts on stale data.

**How it manifests:** User presses the "next notification" keybinding rapidly. Two instances read the same `.pane_` file. Both attempt `switch-client` to the same pane and both delete the same files. The second instance may fail silently or display confusing tmux errors.

**Suggested fix:** Use an advisory lock (`flock`) or `mv` the file to claim it before processing:
```bash
CLAIMED=$(mktemp "$DIR/.claimed.XXXXXX")
mv "$DIR/.pane_$TARGET" "$CLAIMED" 2>/dev/null || exit 0
PANE_ID=$(cat "$CLAIMED")
rm -f "$CLAIMED"
```

---

### C3. Unbounded Log File Growth

**File:** `scripts/tmux-notify-lib.sh`, line 92
**Severity:** Critical

The `log_event` function appends to `$NOTIFY_LOG` unconditionally with no rotation, truncation, or size check. Over weeks/months of use, this file grows without bound.

```bash
echo "[$TIMESTAMP] $icon ${NOTIFY_SOURCE}/$label: $msg" >> "$NOTIFY_LOG"
```

**How it manifests:** A user running many Claude Code sessions over months ends up with a multi-gigabyte log file. The `log-viewer.sh` calls `tail -f` on it, and the `notification-picker.sh` does not read it, but disk usage silently climbs.

**Suggested fix:** Add log rotation. Either truncate to the last N lines periodically, or check size before appending:
```bash
if [ -f "$NOTIFY_LOG" ] && [ "$(wc -c < "$NOTIFY_LOG")" -gt 1048576 ]; then
    tail -500 "$NOTIFY_LOG" > "$NOTIFY_LOG.tmp" && mv "$NOTIFY_LOG.tmp" "$NOTIFY_LOG"
fi
```

---

## High Severity Issues

### H1. JSON Parsing with grep/sed Is Fragile and Incorrect

**File:** `scripts/claude-hook.sh`, lines 11, 24; `codex/notify.sh`, lines 9-11
**Severity:** High

JSON is parsed using `grep -o` and `sed` regex, which breaks on:
- Escaped quotes inside values: `"cwd": "path/with\"quote"`
- Multi-line JSON (pretty-printed)
- Unicode escape sequences: `"message": "\u0048ello"`
- Multiple occurrences of the same key at different nesting levels
- Null values: `"cwd": null`

```bash
CWD=$(echo "$JSON_DATA" | grep -o '"cwd"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*: *"\([^"]*\)".*/\1/')
```

**How it manifests:** A project path containing a double-quote or backslash causes `CWD` to be parsed incorrectly, producing a wrong project name. A pretty-printed JSON payload with the key on one line and the value on the next silently fails, leaving `CWD` empty.

**Suggested fix:** Use `jq` with a fallback:
```bash
CWD=$(echo "$JSON_DATA" | jq -r '.cwd // empty' 2>/dev/null)
if [ -z "$CWD" ]; then
    CWD=$(echo "$JSON_DATA" | grep -o '"cwd"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*: *"\([^"]*\)".*/\1/')
fi
```

---

### H2. Notification Key Collision for Same-Named Projects

**File:** `scripts/tmux-notify-lib.sh`, lines 72-74
**Severity:** High

The notification key is `"${display_name}__${_notif_pane_safe}"`. The `display_name` is derived from the directory basename. If two different projects in different locations happen to have the same directory name and run in the same pane (sequentially), their notifications will collide - the second overwrites the first.

More critically, `_notif_pane_safe` is computed at source-time (line 43), stripping `%` from `$TMUX_PANE`. But if the library is sourced from a context where `TMUX_PANE` is unset (e.g., testing, or a non-tmux shell), `_notif_pane_safe` is empty, and all notifications collapse to the same key.

**How it manifests:** User has `~/work/api` and `~/personal/api`. Both produce `display_name="api"`. If Claude Code runs in the same pane for different projects (user `cd`'s between them), the second notification silently overwrites the first.

**Suggested fix:** Include a hash of the full CWD in the notification key, or use the full path with path separators replaced.

---

### H3. `ls` Parsing for File Listing Is Unreliable

**File:** `scripts/notification-reader.sh`, line 5; `scripts/next-notification.sh`, line 5; `scripts/notification-picker.sh`, line 5
**Severity:** High

All three scripts use `ls -t "$DIR" | grep -v ...` to enumerate notification files. Parsing `ls` output is a well-known antipattern because:
- Filenames containing newlines will be split across lines
- Filenames containing special glob characters may be expanded
- `ls` behavior varies across implementations

```bash
FILES=$(ls -t "$DIR" 2>/dev/null | grep -v '^\.' | grep -v '^events\.log$')
```

**How it manifests:** A notification key containing a newline (possible if `display_name` contains one, e.g., from a malformed `basename` result) causes the file listing to be corrupted. While unlikely with normal project names, this is a latent correctness issue.

**Suggested fix:** Use `find` with `-printf` and `sort`:
```bash
FILES=$(find "$DIR" -maxdepth 1 -type f -not -name '.*' -not -name 'events.log' -printf '%T@\t%f\n' | sort -rn | cut -f2)
```

---

### H4. `display_name` Can Contain Characters Unsafe for Filenames

**File:** `scripts/tmux-notify-lib.sh`, lines 72-74
**Severity:** High

The `display_name` (derived from `basename` of the CWD) is used directly in a filename: `$NOTIF_DIR/$notif_key`. Project directory names can contain spaces, parentheses, single quotes, and other shell-significant characters. While these are technically valid in filenames, they interact badly with the unquoted usages throughout the codebase.

**How it manifests:** A project directory named `my project (v2)` produces a notification key like `my project (v2)__%5`. The `ls | grep` pipeline in `notification-reader.sh` may mishandle this. More importantly, the `for pane_file in "$NOTIF_DIR"/.pane_*` glob in `clear-notification.sh` (line 11) is quoted, so it works, but the `rm -f "$pane_file" "$NOTIF_DIR/$NOTIF_KEY"` on line 17 depends on `NOTIF_KEY` being correctly extracted via `sed`, which may fail if the key contains regex-special characters.

**Suggested fix:** Sanitize `display_name` to only contain alphanumeric characters, hyphens, underscores, and slashes:
```bash
display_name=$(echo "$display_name" | tr -c 'a-zA-Z0-9_/-' '_')
```

---

### H5. Hook Commands Fail Silently When Plugin Path Contains Spaces

**File:** `claude-notifications.tmux`, lines 22-24, 26-27, 33
**Severity:** High

The plugin path is embedded in tmux hook commands with single quotes:
```bash
tmux set-hook -g client-session-changed "run-shell '$SCRIPTS_DIR/clear-notification.sh ...'"
```

If the plugin is installed in a directory whose path contains spaces (e.g., `~/tmux plugins/tmux-agent-notifications/`), the single-quoted path will break because the outer double-quotes don't protect the inner single-quoted expansion properly - the space will split the argument to `run-shell`.

**How it manifests:** User clones the repo into a path with spaces. All hooks silently fail, no notifications are ever cleared.

**Suggested fix:** Escape spaces in the path, or use tmux's `run-shell -b` with proper quoting:
```bash
ESCAPED_DIR=$(printf '%s' "$SCRIPTS_DIR" | sed "s/ /\\\\ /g")
```

---

### H6. `clear-notification.sh` Runs on Every Pane Focus, Causing O(n) File Scans

**File:** `claude-notifications.tmux`, lines 22-24; `scripts/clear-notification.sh`, lines 9-30
**Severity:** High

Three hooks trigger `clear-notification.sh`: `client-session-changed`, `client-focus-in`, and `pane-focus-in`. Each invocation:
1. Calls `tmux list-panes -a` (forks tmux, lists all panes across all sessions)
2. Iterates over every `.pane_*` file in the notification directory
3. Calls `cat`, `basename`, `sed`, `stat`, and `date` for each file
4. Calls `tmux refresh-client -S`

This runs on every single pane switch, tab change, or window focus. With many open panes and accumulated notification files, this adds noticeable latency to pane switching.

**How it manifests:** User with 50+ panes notices a slight delay when switching panes. `tmux list-panes -a` is called three times in quick succession (once per hook) on every focus change.

**Suggested fix:**
1. Debounce: write a timestamp file and skip if invoked within the last second
2. Only scan files matching the current pane ID instead of iterating all files
3. Remove the `client-focus-in` hook (redundant with `pane-focus-in`)

---

## Medium Severity Issues

### M1. `is_user_watching` Uses 2-Second Activity Window That Can Miss Active Users

**File:** `scripts/tmux-notify-lib.sh`, lines 45-54
**Severity:** Medium

The function checks if the user's `client_activity` timestamp is within 2 seconds of `now`. But `client_activity` reflects the last keypress or mouse event - not whether the user is looking at the screen. A user reading output without pressing keys for 3 seconds will be considered "not watching," and a notification will be created even though they're staring at the pane.

**How it manifests:** User is watching Claude Code output. It finishes. User hasn't pressed a key in 3 seconds. A notification appears in the status bar for a pane they're already looking at.

**Suggested fix:** Consider also checking `client_flags` for `focused` AND checking whether the active pane is the same as `TMUX_PANE`, which would indicate the pane is visible to the user regardless of activity time.

---

### M2. `sed -i` in `codex/install.sh` Is Not Portable

**File:** `codex/install.sh`, lines 25-27
**Severity:** Medium

```bash
sed -i.bak "1a\\
notify = [\"$NOTIFY_PATH\"]
" "$CONFIG"
rm -f "$CONFIG.bak"
```

While `sed -i.bak` works on both macOS and GNU sed, the `1a\` (append after line 1) syntax with the backslash-newline continuation differs between BSD sed and GNU sed. On some BSD sed implementations, the `\\` followed by a newline is required; on GNU sed, it is not. This specific form happens to work on both, but the `1a` placement is fragile if the config file is empty (line 1 may not exist).

**How it manifests:** User has an empty `~/.codex/config.toml` file (0 bytes). `sed '1a\...'` on an empty file produces no output on some sed implementations because there is no line 1 to append after.

**Suggested fix:** Use `printf` to append to end of file instead:
```bash
printf '\nnotify = ["%s"]\n' "$NOTIFY_PATH" >> "$CONFIG"
```

---

### M3. `TIMESTAMP` Is Captured at Source Time, Not at Event Time

**File:** `scripts/tmux-notify-lib.sh`, line 18
**Severity:** Medium

```bash
TIMESTAMP=$(date '+%H:%M:%S')
```

This is evaluated once when the library is sourced. If the hook script runs for a while (e.g., network delay, slow tmux commands), or if the library is sourced early and events happen later, all events in that invocation share the same timestamp.

For the Claude hook specifically, the library is sourced at line 16, and then `tmux_alert` is called later. If the tmux commands in `is_user_watching` are slow (e.g., tmux server is busy), the timestamp can be several seconds off.

**How it manifests:** The notification and log entry show a timestamp that is slightly earlier than when the event actually occurred. For a long-running codex hook that processes multiple events, all events get the same timestamp.

**Suggested fix:** Compute the timestamp at the point of use:
```bash
log_event() {
    local ts=$(date '+%H:%M:%S')
    echo "[$ts] $icon ..."
}
```

---

### M4. `notification-reader.sh` Forks Many Subshells Per Status Refresh

**File:** `scripts/notification-reader.sh`, lines 15-17, 22-43
**Severity:** Medium

The `visible_len` function is called once per notification file on every tmux status refresh (default: every 15 seconds, but can be as frequent as every second). Each call pipes through `sed` and `wc`, spawning two subprocesses.

```bash
visible_len() {
    echo -n "$1" | sed 's/#\[[^]]*\]//g' | wc -c | tr -d ' '
}
```

Additionally, each iteration does `cat "$DIR/$file"` (line 23), adding another fork.

**How it manifests:** With 10 pending notifications and `status-interval` set to 1, the reader spawns ~40+ subprocesses per second just for status rendering. On a resource-constrained system, this contributes to noticeable CPU overhead.

**Suggested fix:** Use bash built-in string manipulation instead of sed:
```bash
visible_len() {
    local clean="${1//#\[*\]/}"
    echo "${#clean}"
}
```

---

### M5. `log-viewer.sh` Does Not Handle Terminal Resize or Cleanup

**File:** `scripts/log-viewer.sh`, lines 1-9
**Severity:** Medium

```bash
tail -f "$LOG_FILE" &
TAIL_PID=$!
trap "kill $TAIL_PID 2>/dev/null" EXIT
while IFS= read -rsn1 key; do
    [ "$key" = "q" ] && exit
done
```

Issues:
1. Only `q` exits. No support for `Ctrl-C` (handled by trap), `Escape`, or closing the popup window. If the popup is closed by tmux, the `EXIT` trap fires, but the `read` loop may not terminate cleanly.
2. `tail -f` output mixes with the `read` prompt, causing visual glitches.
3. No instructions are shown to the user about how to exit.
4. If the log file is deleted while `tail -f` is running, `tail` continues running and consuming a file descriptor until the popup is closed.

**How it manifests:** User opens the log viewer, does not know to press `q`, closes the popup with `Escape`. The `tail -f` process may linger as an orphan if the trap does not fire.

**Suggested fix:** Use `less +F` instead for a proper follow-mode with navigation, or show a header line with instructions. Also add `INT` and `TERM` to the trap.

---

### M6. `pane_focus_in` Hook in `clear-notification.sh` Does Not Match Panes by CWD

**File:** `scripts/clear-notification.sh`, lines 1-32
**Severity:** Medium

The script receives `$PANE_PATH` (line 3) but never uses it. It clears notifications based on matching `$PANE_ID` against stored pane IDs in `.pane_*` files. But the `resolve_project_name` in the hook generates display names from the CWD, meaning the notification key contains the *project name*, not the pane ID. The clearing logic matches on pane ID correctly, but the unused `$PANE_PATH` argument suggests an incomplete design where CWD-based matching was intended but never implemented.

**How it manifests:** No functional bug currently, but the unused parameter is misleading and may indicate a missing feature (e.g., clearing notifications when the user navigates to the project directory in a different pane).

**Suggested fix:** Either remove the unused `$PANE_PATH` parameter from the hook invocation in `claude-notifications.tmux` line 22-24, or implement CWD-based clearing.

---

### M7. `grep -q "^${STORED_PANE}$"` Is Vulnerable to Regex Injection

**File:** `scripts/clear-notification.sh`, line 16; `scripts/next-notification.sh`, line 10; `scripts/notification-picker.sh`, line 25
**Severity:** Medium

```bash
echo "$LIVE_PANES" | grep -q "^${STORED_PANE}$"
# and
tmux list-panes -a -F '#{pane_id}' | grep -q "^${PANE_ID}$"
```

`PANE_ID` / `STORED_PANE` comes from reading a file (`cat "$pane_file"`). If that file is corrupted, truncated, or tampered with, the value may contain regex metacharacters (`.`, `*`, `+`, etc.) that would cause false matches.

Tmux pane IDs are normally `%N` format, so the `%` is stripped in `_notif_pane_safe` but retained in the `.pane_` file (line 74 writes `${TMUX_PANE}` which includes the `%`). The `%` is not a regex metacharacter, so this works by accident, but the pattern is fragile.

**How it manifests:** A corrupted `.pane_` file containing `.*` would match any pane ID, causing the wrong notification to be cleared or the wrong pane to be switched to.

**Suggested fix:** Use `grep -Fxq` (fixed-string, whole-line match):
```bash
echo "$LIVE_PANES" | grep -Fxq "$STORED_PANE"
```

---

### M8. `codex/install.sh` Has Path Injection in TOML Generation

**File:** `codex/install.sh`, lines 12, 26
**Severity:** Medium

```bash
echo "notify = [\"$NOTIFY_PATH\"]" > "$CONFIG"
```

If `$NOTIFY_PATH` contains a double-quote or backslash (possible if the install directory has such characters), the generated TOML is syntactically invalid. Additionally, the `grep -q "$NOTIFY_PATH"` check on line 18 treats the path as a regex pattern, so paths containing `.` (very common: `/home/user/tmux-agent-notifications/codex/notify.sh` - the dots match any character) can produce false positive matches.

**How it manifests:** If the path is `/path/to/codex/notify.sh` and the config already contains `/path/to/codex/notifyXsh`, the grep check on line 18 reports "Already configured" due to `.` matching any character.

**Suggested fix:** Use `grep -Fq` for literal matching:
```bash
if grep -Fq "$NOTIFY_PATH" "$CONFIG"; then
```

---

### M9. `SESSION_TITLE` Sed Regex Strips Legitimate Prefixes

**File:** `scripts/claude-hook.sh`, line 20
**Severity:** Medium

```bash
SESSION_TITLE=$(tmux display-message -t "$TMUX_PANE" -p '#{pane_title}' 2>/dev/null | sed 's/^[^a-zA-Z0-9]* *//')
```

This strips all leading non-alphanumeric characters. If the pane title is something like `(feature-branch) my-task`, this becomes `feature-branch) my-task` - the opening parenthesis is stripped but the closing one remains. This produces an unbalanced parenthesis in the notification display (line 68 of tmux-notify-lib.sh wraps it in parens again).

**How it manifests:** Notification displays as `api (feature-branch) my-task)` with mismatched parentheses.

**Suggested fix:** Strip only known emoji/icon prefixes, or strip the specific unicode prefix characters that Claude Code uses, rather than a broad character class.

---

### M10. `notification-reader.sh` Glob Excludes Dotfiles But Allows `events.log` Through Edge Cases

**File:** `scripts/notification-reader.sh`, line 5
**Severity:** Medium

```bash
FILES=$(ls -t "$DIR" 2>/dev/null | grep -v '^\.' | grep -v '^events\.log$')
```

The regex `^events\.log$` uses `.` which matches any character, but since this is used to *exclude* the file, a false match (e.g., a file named `eventsXlog`) would also be excluded. This is minor but shows fragile filtering. More importantly, any other non-notification files placed in `~/.tmux-notifications/` (e.g., by another tool, or temp files from the atomic write fix) would be read as notification content and displayed on the status bar.

**How it manifests:** If the atomic write suggestion from C1 is implemented using temp files in the same directory, those temp files would briefly appear in the notification listing.

**Suggested fix:** Use a consistent naming convention (e.g., `notif_` prefix) for notification files and filter by that prefix, or use a subdirectory for notifications.

---

### M11. `PreToolUse` Clears Alert Conditionally But Other Events Don't Check Consistency

**File:** `scripts/claude-hook.sh`, lines 39-41
**Severity:** Medium

```bash
"PreToolUse")
    is_user_watching && tmux_clear_alert "$DISPLAY_NAME"
    ;;
```

The `PreToolUse` event clears the alert only if the user is watching. But `UserPromptSubmit` and `SessionEnd` clear unconditionally (line 43). This is logically correct (user submitting a prompt means they're definitely interacting), but `PreToolUse` has a subtle issue: the `CWD` used to derive `DISPLAY_NAME` may differ from the CWD when the original alert was set (if the agent changed directories), causing the `tmux_clear_alert` to target the wrong notification key.

**How it manifests:** Claude Code sends a notification from `/project-a`, user doesn't interact, then Claude starts a tool use from a different working directory. The `PreToolUse` tries to clear the alert for the new directory, leaving the old notification orphaned.

**Suggested fix:** Store the notification key in an environment variable or file that persists across hook invocations within the same session.

---

## Low Severity Issues

### L1. `refresh_all_clients` Iterates All Clients Unnecessarily

**File:** `scripts/tmux-notify-lib.sh`, lines 56-60
**Severity:** Low

```bash
refresh_all_clients() {
    tmux list-clients -F '#{client_name}' 2>/dev/null | while read -r c; do
        tmux refresh-client -S -t "$c" 2>/dev/null
    done
}
```

`tmux refresh-client -S` without `-t` refreshes the current client. To refresh all clients, a simpler approach is:
```bash
tmux refresh-client -S -a 2>/dev/null  # if tmux version supports -a
```
However, the current approach forks `tmux` N+1 times for N clients.

**Suggested fix:** Check if `tmux refresh-client -S` alone is sufficient (it may already refresh all status lines), or use a single tmux command.

---

### L2. `resolve_project_name` Does Not Handle Symlinks

**File:** `scripts/tmux-notify-lib.sh`, lines 20-30
**Severity:** Low

```bash
project=$(basename "${cwd:-unknown}")
parent=$(basename "$(dirname "$cwd")" 2>/dev/null)
```

If `$cwd` is a symlink, `basename` operates on the symlink name, not the target. If the user's project is at `~/current -> ~/repos/my-project`, the display name will be `current` instead of `my-project`.

Also, the `.git` worktree detection (line 25) checks if the parent directory name ends in `.git`, but this only catches bare git worktrees of a specific layout. Linked worktrees created with `git worktree add` have a different structure.

**How it manifests:** Users of `git worktree` or symlinked project directories see incorrect project names in notifications.

**Suggested fix:** Resolve symlinks with `readlink -f` or `realpath`:
```bash
cwd=$(realpath "$cwd" 2>/dev/null || echo "$cwd")
```

---

### L3. `notification-picker.sh` Delimiter Handling Is Fragile

**File:** `scripts/notification-picker.sh`, line 18
**Severity:** Low

```bash
SELECTED=$(echo "$LIST" | fzf --reverse --header='Jump to notification' --delimiter='\t' --with-nth=2)
```

The `--delimiter='\t'` passes a literal backslash-t to fzf, not a tab character. In most shells and fzf versions, this happens to work because fzf interprets `\t` as a tab. However, this is fzf-version-dependent behavior, not guaranteed by POSIX or fzf's documentation.

**How it manifests:** On older fzf versions or alternative implementations, the delimiter may not be recognized as a tab, causing the entire line (including the filename key) to be displayed to the user.

**Suggested fix:** Use `$'\t'`:
```bash
SELECTED=$(echo "$LIST" | fzf --reverse --header='Jump to notification' --delimiter=$'\t' --with-nth=2)
```

---

### L4. No File Permission Restrictions on Notification Directory

**File:** `scripts/tmux-notify-lib.sh`, line 14; `claude-notifications.tmux`, line 6
**Severity:** Low

```bash
mkdir -p "$NOTIF_DIR"
# and
mkdir -p "$HOME/.tmux-notifications"
```

The notification directory is created with default permissions (typically 0755). Other users on the system can read notification file contents, which may contain project names and task descriptions. While not highly sensitive, this is information leakage.

**How it manifests:** On a shared system, another user can `ls ~/.tmux-notifications/` of any user and see which projects they're working on and what their agents are doing.

**Suggested fix:**
```bash
mkdir -p "$NOTIF_DIR" && chmod 700 "$NOTIF_DIR"
```

---

### L5. `clear-notification.sh` stat Command Differs Between macOS and Linux

**File:** `scripts/clear-notification.sh`, lines 22-26
**Severity:** Low

```bash
if [ "$(uname)" = "Darwin" ]; then
    FILE_AGE=$(( $(date +%s) - $(stat -f %m "$pane_file") ))
else
    FILE_AGE=$(( $(date +%s) - $(stat -c %Y "$pane_file") ))
fi
```

This is already handled with a platform check, which is good. However, the `uname` call is executed once per file in the loop (since it's inside the loop body). It should be hoisted outside the loop.

Also, if the file is deleted between the existence check on line 12 and the `stat` call on line 23-25, `stat` will fail and the arithmetic expression will produce a syntax error (attempting to subtract empty string from a number).

**How it manifests:** Very rare. A concurrent deletion between lines 12 and 23 produces an error like `bash: ((: 1711234567 - : syntax error: operand expected`.

**Suggested fix:** Move `uname` check outside the loop and add error handling:
```bash
IS_DARWIN=$([[ "$(uname)" = "Darwin" ]] && echo 1 || echo 0)
# Inside loop:
if [ "$IS_DARWIN" = "1" ]; then
    FILE_MOD=$(stat -f %m "$pane_file" 2>/dev/null) || continue
else
    FILE_MOD=$(stat -c %Y "$pane_file" 2>/dev/null) || continue
fi
FILE_AGE=$(( $(date +%s) - FILE_MOD ))
```

---

### L6. Keybinding Conflicts Not Checked

**File:** `claude-notifications.tmux`, lines 26-27
**Severity:** Low

```bash
tmux bind-key "$key_next" run-shell "$SCRIPTS_DIR/next-notification.sh"
tmux bind-key "$key_picker" display-popup -w 60% -h 60% -E "$SCRIPTS_DIR/notification-picker.sh"
```

The default keys `n` and `S` are bound unconditionally in the prefix table. `prefix + n` is tmux's default for `next-window`. Users who don't customize the key may lose their `next-window` binding without realizing it.

**How it manifests:** User installs the plugin and finds that `prefix + n` no longer switches to the next window.

**Suggested fix:** Document this clearly in the README, or use less common default keys (e.g., `M-n` for alt+n in the root table, or `N` instead of `n`).

---

### L7. `notification-reader.sh` Does Not Handle Empty Notification Directory Gracefully

**File:** `scripts/notification-reader.sh`, line 5
**Severity:** Low

```bash
FILES=$(ls -t "$DIR" 2>/dev/null | grep -v '^\.' | grep -v '^events\.log$')
[ -z "$FILES" ] && exit 0
```

When `exit 0` is called with no output, tmux renders an empty status line (status-format[1]). This is visually a blank bar, which is the intended behavior. However, the script is called every `status-interval` seconds. Each invocation forks a shell, runs `ls`, `grep`, and then exits. This is wasted work when there are no notifications (the common case).

**Suggested fix:** Consider using tmux's built-in conditional formatting to skip the script call entirely when a marker file does not exist:
```
#(if [ -f ~/.tmux-notifications/.has-notifs ]; then scripts/notification-reader.sh; fi)
```

---

### L8. `codex/notify.sh` Falls Through to "Needs attention" for Unknown Types

**File:** `codex/notify.sh`, lines 43-45
**Severity:** Low

```bash
*)
    log_event "" "Needs attention" "$DISPLAY_NAME"
    tmux_alert "Needs attention" "$DISPLAY_NAME"
    ;;
```

When both `TYPE` and `STATUS` are unknown/empty/unrecognized, the script creates a notification saying "Needs attention." This can produce false alarms for benign events that just happen to have unrecognized type/status values.

**How it manifests:** Codex sends an informational event with type `"progress-update"` and status `"running"`. The script shows "Needs attention" even though no user action is required.

**Suggested fix:** Add a `"running"|"in-progress"` case that logs without alerting, or simply skip unknown events:
```bash
*)
    log_event "?" "Unknown event: type=$TYPE status=$STATUS" "$DISPLAY_NAME"
    # Do NOT alert for unknown events
    ;;
```

---

### L9. `SESSION_TITLE` and `PANE_PATH` Are Not Sanitized

**File:** `scripts/claude-hook.sh`, line 20
**Severity:** Low

The pane title from `tmux display-message` is used in `tmux_alert` where it is embedded in a tmux format string written to a file. If the pane title contains tmux format sequences (e.g., `#[bg=red]`), they will be interpreted by tmux when rendering the status line. This could be exploited to inject arbitrary tmux formatting.

**How it manifests:** A process sets its terminal title to `#[bg=red]HACKED#[default]` via an escape sequence. The notification reader renders this as a red background alert, confusing the user. This is a cosmetic issue, not a security vulnerability, since tmux format sequences cannot execute commands.

**Suggested fix:** Escape `#` characters in the session title:
```bash
SESSION_TITLE="${SESSION_TITLE//\#/##}"
```

---

### L10. Double `mkdir -p` for the Same Directory

**File:** `claude-notifications.tmux`, line 6; `scripts/tmux-notify-lib.sh`, line 14
**Severity:** Low

Both the plugin entry point and the library create `~/.tmux-notifications`. This is redundant but harmless. However, it indicates a lack of clear ownership of initialization.

**Suggested fix:** Remove the `mkdir` from `claude-notifications.tmux` and let the library own directory creation.

---

### L11. `echo "$JSON_DATA"` Will Mangle Backslashes on Some Systems

**File:** `scripts/claude-hook.sh`, line 11; `codex/notify.sh`, lines 9-11
**Severity:** Low

```bash
CWD=$(echo "$JSON_DATA" | grep ...)
```

The behavior of `echo` with backslash sequences is unspecified by POSIX. Some shells' built-in `echo` will interpret `\n`, `\t`, etc. in the JSON data. If the JSON contains `"cwd": "C:\\Users\\name"`, some `echo` implementations will convert `\\` to `\`, then `\U` may be interpreted as a Unicode escape.

**Suggested fix:** Use `printf '%s\n' "$JSON_DATA"` instead of `echo "$JSON_DATA"`.

---

## Design Issues (Non-Bug)

### D1. Flat-File Storage Model Limits Scalability

The plugin uses individual files in a flat directory as its data store. This is simple but creates O(n) scanning overhead for every operation. A single JSON or line-oriented file with structured records would reduce filesystem operations at the cost of needing file locking.

### D2. No Versioning or Migration Strategy

There is no version marker in the notification directory. If the file format or naming convention changes in a future release, old notification files will be incorrectly parsed or ignored without warning.

### D3. Global tmux Hooks Affect All Sessions

The hooks are set with `-g` (global), meaning they fire in every tmux session, even those not using Claude Code or Codex. This is unnecessary overhead for users who only use AI tools in some sessions.

### D4. No Test Infrastructure

There are no unit tests, integration tests, or test fixtures. The JSON parsing, project name resolution, and notification lifecycle are all untested, making regressions likely during refactoring.

### D5. Coupling Between Hook Scripts and Library

The hook scripts (`claude-hook.sh`, `codex/notify.sh`) must set specific variables (`NOTIFY_SOURCE`) before sourcing the library. This implicit contract is documented only in comments and is easy to violate. A function-based API (e.g., `notify_init "Claude"`) would be more robust.

---

## Summary Table

| ID  | Severity | File | Summary |
|-----|----------|------|---------|
| C1  | Critical | tmux-notify-lib.sh:73 | Non-atomic file writes cause read-during-write |
| C2  | Critical | next-notification.sh:8 | TOCTOU race on read-then-delete |
| C3  | Critical | tmux-notify-lib.sh:92 | Unbounded log file growth |
| H1  | High | claude-hook.sh:11 | grep/sed JSON parsing breaks on edge cases |
| H2  | High | tmux-notify-lib.sh:72 | Notification key collisions for same-named projects |
| H3  | High | notification-reader.sh:5 | Parsing `ls` output is unreliable |
| H4  | High | tmux-notify-lib.sh:72 | Unsafe characters in display_name used as filename |
| H5  | High | claude-notifications.tmux:22 | Plugin paths with spaces break hooks |
| H6  | High | clear-notification.sh:9 | O(n) file scan on every pane focus |
| M1  | Medium | tmux-notify-lib.sh:45 | 2-second activity window misidentifies active users |
| M2  | Medium | codex/install.sh:25 | `sed -i` append on empty file is fragile |
| M3  | Medium | tmux-notify-lib.sh:18 | Timestamp captured at source-time, not event-time |
| M4  | Medium | notification-reader.sh:15 | Excessive subshell forks per status refresh |
| M5  | Medium | log-viewer.sh:4 | No exit instructions, potential orphan processes |
| M6  | Medium | clear-notification.sh:3 | Unused $PANE_PATH parameter |
| M7  | Medium | clear-notification.sh:16 | grep regex injection from file contents |
| M8  | Medium | codex/install.sh:12 | Path injection in TOML and regex false matches |
| M9  | Medium | claude-hook.sh:20 | Sed strips legitimate characters from pane title |
| M10 | Medium | notification-reader.sh:5 | Non-notification files in dir get displayed |
| M11 | Medium | claude-hook.sh:39 | CWD drift causes mismatched clear in PreToolUse |
| L1  | Low | tmux-notify-lib.sh:56 | N+1 tmux forks to refresh clients |
| L2  | Low | tmux-notify-lib.sh:20 | Symlinks and worktrees not resolved |
| L3  | Low | notification-picker.sh:18 | fzf delimiter backslash-t may not parse as tab |
| L4  | Low | tmux-notify-lib.sh:14 | Notification directory world-readable |
| L5  | Low | clear-notification.sh:22 | uname called per-file; stat race on deleted file |
| L6  | Low | claude-notifications.tmux:26 | Default keybinding overrides tmux's next-window |
| L7  | Low | notification-reader.sh:5 | Wasted work when no notifications exist |
| L8  | Low | codex/notify.sh:43 | Unknown events trigger false "Needs attention" |
| L9  | Low | claude-hook.sh:20 | Tmux format injection via pane title |
| L10 | Low | claude-notifications.tmux:6 | Redundant mkdir in two files |
| L11 | Low | claude-hook.sh:11 | `echo` may mangle backslashes in JSON |

---

*Analysis generated 2026-03-26. 3 Critical, 6 High, 11 Medium, 11 Low issues identified across 10 source files.*
