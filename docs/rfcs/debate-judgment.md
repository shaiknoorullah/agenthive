# Judgment: File-Based vs. Native tmux Notification Architecture

**Judge:** Impartial technical evaluation
**Date:** 2026-03-26
**Status:** Final verdict

---

## 1. Fact-Check Results

The following table evaluates the most consequential factual claims from both position papers. Claims were verified against the actual source code in the repository, tmux documentation, the tmux wiki (Formats, Advanced Use), the tmux CHANGES file, and POSIX specifications.

| # | Claim | Source | Verdict | Evidence |
|---|-------|--------|---------|----------|
| 1 | "tmux provides no `show-options --prefix @notif-*` wildcard query" | File-Based, Section 1 | **TRUE** | tmux `show-options` does not support wildcard or prefix filtering. You must pipe through `grep`. However, `tmux show-options -g \| grep @notif` works reliably and is a stable interface. The claim is true but overstates the difficulty -- a single grep pipeline is not a serious impediment. |
| 2 | "Files are debuggable with `ls` and `cat`. tmux options require `tmux show-options` plus grep plus an active server." | File-Based, Section 1 | **TRUE** | Accurate. File inspection requires no running process; tmux option inspection requires a live tmux server. However, in the context of this plugin, tmux is always running when notifications are relevant, so the "active server" requirement is always met during normal usage. |
| 3 | "`echo > file` using shell redirection performs a create-write-close sequence that is atomic at the rename level when using `O_CREAT\|O_TRUNC`" | File-Based, Section 6 | **MISLEADING** | Shell redirection `echo "..." > file` performs `open(O_WRONLY\|O_CREAT\|O_TRUNC)`, `write()`, `close()`. The `write()` call is NOT guaranteed atomic for regular files by POSIX. A concurrent reader can observe partial content. The `O_CREAT\|O_TRUNC` only makes the file creation/truncation atomic, not the subsequent write. The claim conflates file creation atomicity with write atomicity. The code analysis confirms this as bug C1. |
| 4 | "The notification key naming scheme ensures that each notification file has exactly one writer (the hook for that specific pane), eliminating write-write races entirely." | File-Based, Section 6 | **MOSTLY TRUE** | The key `{display_name}__{pane_id}` ensures one pane maps to one key. However, the code analysis (H2) documents that same-named projects in different directories collide, and that the dual-file write (`notif_key` + `.pane_notif_key`) creates a race window. Single-writer per key is true in the common case but not guaranteed. |
| 5 | "Per-pane user options (`set -p @option`) require tmux 3.1+" | Native, Section 8 | **TRUE** | Per-pane options via `set-option -p` were added in tmux 3.1 (released June 2020). The `@` user option prefix with pane scope is stable since that version. The plugin targets tmux 3.2+, so this is within range. |
| 6 | "#{W:#{P:}} iterates windows/panes in format strings" and "requires tmux 3.2+" | Native, Section 6.2 | **TRUE** | The `W`, `P`, and `S` loop modifiers are documented in the tmux Formats wiki. The tmux 3.2 CHANGES file added "sorting to W, P, L loop operators," confirming they existed by 3.2. Nested `#{W:#{P:...}}` to iterate all panes across all windows is a supported pattern in tmux 3.2+. |
| 7 | "The `#()` construct causes tmux to fork a shell on every status refresh interval" | Native, Section 1.2 | **PARTIALLY TRUE** | tmux does fork a shell for `#()` commands, but tmux caches the result between `status-interval` ticks. The shell is not re-forked on every redraw, only on each interval expiration. The tmux documentation confirms: "Shell commands are only executed once at the interval specified by status-interval; if the status line is redrawn in the meantime, the previous result is used." The claim is true at the interval level, not at every redraw. |
| 8 | "With a native approach, notification clearing on pane focus requires zero forks: `set -p -u @notif-msg` as an inline hook command" | Native, Section 2.3 | **TRUE** | tmux hooks can execute tmux commands directly without `run-shell`. The `if -F "#{@notif-msg}" "set -p -u @notif-msg ; ..."` pattern executes entirely in the tmux server process. Confirmed by tmux documentation on hooks and the `if-shell -F` flag (which evaluates a format string rather than running a shell command). |
| 9 | "tmux's single-threaded command queue provides transactional semantics for chained commands" | Native, Section 3 | **PARTIALLY TRUE** | Commands chained with `;` in a single hook command do execute sequentially in the server's event loop. However, this is not a true transaction -- there is no rollback, and interleaving with other clients' commands is possible between individual commands in the chain. The claim overstates the guarantees. Still, for the set/unset use case, sequential execution in the server is sufficient. |
| 10 | "When the tmux server crashes, all user options are lost" | File-Based, Section 3 | **TRUE** | tmux stores all options in memory. Server crash or `kill-server` destroys all state, including user options. This is a well-known limitation that spawned plugins like tmux-resurrect. |
| 11 | "Notifications for AI agent tasks are ephemeral by nature. When tmux restarts, all agent processes are gone too -- their notifications are meaningless." | Native, Section 7 | **TRUE** | Agent processes run inside tmux panes. When the tmux server dies, those processes receive SIGHUP and terminate. Notifications for dead agents are indeed stale. The File-Based advocate's persistence argument has limited practical value for this specific use case. |
| 12 | "`tmux list-panes -a -F '#{?@notif-msg,#{pane_id}:#{@notif-msg},}'` returns only panes with active notifications" | Native, Section 5 | **TRUE with caveat** | This format string works: the conditional `#{?@notif-msg,...,}` produces output only when `@notif-msg` is set. However, panes where `@notif-msg` was never set will output an empty string per line (not be omitted), so a `grep -v '^$'` is needed to filter empty lines. The Native paper acknowledges this in Section 6.5. |
| 13 | "User option scoping has varied across tmux versions; server-scoped user options have had inconsistent support" | File-Based, Section 5 | **MISLEADING** | While early tmux versions had some ambiguity around server-scoped options, per-pane user options (`set -p @var`) have been stable and consistent since tmux 3.1. The plugin targets 3.2+. The claim implies instability that does not apply to the version range in question. The `-p` scope for user options has not had documented behavioral changes since its introduction. |
| 14 | "A native approach would require every hook script to invoke `tmux set-option`, which requires the `tmux` binary to be on PATH (not always guaranteed in restricted environments or containers)" | File-Based, Section 2 | **MISLEADING** | The hook scripts already invoke `tmux` commands extensively: `tmux display-message`, `tmux list-clients`, `tmux list-panes`, `tmux refresh-client`. The current implementation is deeply dependent on the tmux binary being available. Claiming that a native approach adds a new dependency on tmux is incorrect -- that dependency already exists. |
| 15 | "Over an 8-hour workday, that is roughly 100,000 unnecessary process forks" | Native, Section 4 | **MISLEADING** | The math assumes a 2-second status-interval (aggressive) and 7-8 processes per refresh. With the default 15-second interval, that drops to ~15,000 forks per day. With a more typical 5-second interval and 5 notifications, it is ~46,000. The order of magnitude is correct (tens of thousands), but "100,000" cherry-picks the worst case. More importantly, the tmux `#()` cache means only one fork per interval, not one per redraw. |

---

## 2. Strongest Arguments

### Option A (File-Based): Top 3

**A1. Cross-process IPC simplicity for third-party integrations.**
Any process that can write a file can create a notification. No tmux binary, no socket, no protocol knowledge required. This is the plugin's most distinctive feature -- a notification can be written from Python, Go, Node.js, a cron job, or a process that does not even know it is running inside tmux. The Native approach raises the integration barrier: every producer must have the tmux binary available and a valid `$TMUX` socket. While this is a minor barrier (most agent hooks already run in tmux), it closes the door on truly decoupled integrations. *Strength: high. This is a genuine architectural advantage that the native approach cannot fully replicate.*

**A2. Crash persistence is a real (if narrow) benefit.**
While the Native paper correctly notes that agent notifications are ephemeral, there are edge cases where persistence matters: a brief tmux server restart (e.g., version upgrade, config reload via `kill-server && tmux`), or a tmux crash during a long-running session. Files survive these events; options do not. The practical impact is small given this plugin's use case, but the asymmetry is real. *Strength: moderate. The benefit is real but narrow.*

**A3. The proven, working system has ecosystem momentum.**
The current system works. It has hooks for Claude Code and Codex CLI. Rewriting it carries non-trivial risk (new bugs, broken integrations, development time diverted from features). The "improve rather than replace" approach is lower risk. *Strength: moderate. This is a pragmatic argument, not a technical one, but pragmatism matters.*

### Option B (Native tmux): Top 3

**B1. Elimination of the shell fork on every status refresh.**
This is the single most impactful technical argument. The current `#()` approach forks a shell, runs `ls`, `grep`, `cat`, `sed`, and `wc` on every `status-interval` tick -- even when there are zero notifications. The native `#{W:#{P:#{?@notif-msg,...}}}` format string evaluates entirely inside the tmux server. Zero forks, zero file I/O. For a status line that updates every 1-15 seconds indefinitely, this is a meaningful resource savings and eliminates an entire class of bugs (M4, L7, the entire notification-reader.sh script). *Strength: very high. This is an unambiguous, measurable improvement.*

**B2. Per-pane option scoping eliminates the dual-file race condition and stale-notification cleanup.**
The current architecture's most severe bugs (C1, C2, H6) stem from maintaining two files per notification with no locking, plus an O(n) scan on every focus event. Per-pane options tie notification data to the pane lifecycle: when the pane dies, its options vanish. When you focus a pane, `set -p -u` clears only that pane's options in O(1). The entire `clear-notification.sh` script (32 lines, 3 hooks, O(n) scans, stat calls, age checks) is replaced by a single inline tmux command. This eliminates bugs C1, C2, H6, L5, and M6 at the architectural level. *Strength: very high. This addresses the most severe bugs found in the code analysis.*

**B3. Data/presentation separation enables themes, multiple views, and programmatic access.**
The current system bakes tmux format codes (`#[fg=yellow,bold]`) into notification data files. This means: (a) changing colors requires re-triggering notifications, (b) the notification picker must strip format codes with fragile sed, (c) external tools cannot consume the data without parsing tmux escapes. The native approach stores clean data (`@notif-msg = "Agent has finished"`) and applies formatting in the status-format string. Theme changes are instant. The picker gets clean text via `tmux list-panes -a -F '#{@notif-msg}'`. Multiple views (status bar, popup, desktop notification) can consume the same data with different formatting. *Strength: high. This directly enables proposed features #1 (desktop notifications), #3 (priority levels), #7 (dashboard popup), and #8 (callbacks).*

---

## 3. Weakest Arguments

### Option A (File-Based): Weakest Claims

**"Files decouple notification producers from the tmux server process."** (Section 2)
This is presented as a fundamental architectural advantage, but the current implementation already requires `$TMUX_PANE`, calls `tmux list-clients`, `tmux display-message`, and `tmux refresh-client`. The hooks are deeply coupled to tmux. The "decoupling" is theoretical: in practice, every notification producer in this system needs tmux. The file write is the one operation that does not need tmux, but it is followed immediately by `refresh_all_clients()` which needs it. Claiming decoupling when 4 out of 5 operations require the tmux server is misleading.

**The decision matrix scoring "File-Based 11, Native Options 1" is absurdly tilted.** (Section final)
The matrix assigns "File-Based wins" to categories like "Write performance" (1ms file write vs. 2ms tmux command -- an unmeasurable difference) and "State enumeration" (`ls directory` vs. `show-options | grep` -- both trivial). Several categories are artificially separated to pad the score. A fair matrix would not produce an 11-1 result when the code analysis found 31 bugs in the file-based implementation, including 3 critical ones.

**"tmux command length limits hit with many notifications."** (Risk table)
This is presented as a medium-severity risk of the native approach. tmux commands and format strings have no documented length limit that would be reached by notification data. This is a speculative risk with no evidence.

### Option B (Native tmux): Weakest Claims

**"Implementation complexity: 8 shell scripts, ~250 lines [current] vs. 3-4 shell scripts, ~120 lines [native]."** (Decision matrix)
The line count comparison is misleading. The native approach's complexity is not eliminated -- it is moved into tmux format strings, which are notoriously hard to read, debug, and maintain. A `#{W:#{P:#{?@notif-msg,#[fg=#{@claude-notif-fg}][#{@notif-time}] #{@notif-source}/#{@notif-project}: #[fg=#{@claude-notif-alert-fg}#,#{@claude-notif-alert-style}]#{@notif-msg}#[default]  ,}}}` string is not "simpler" than shell code -- it is different complexity in a less familiar syntax.

**"Multiple `set` commands chained with `;` provide transactional semantics."** (Section 3)
This overstates tmux's guarantees. While commands in a single hook do execute sequentially in the server, they are not transactional. A crash between two `set` commands leaves partial state. There is no rollback. The practical risk is low, but the claim of "transactional semantics" is technically incorrect.

**The decision matrix scoring "Native 12, File-Based 1" is equally tilted.** (Section 9)
Awarding "Native wins" for "Persistence across restart" by arguing that losing options is "correct behavior" is a framing trick. The scoring methodology mirrors the same bias seen in Option A's matrix, just in the opposite direction.

---

## 4. Missed Arguments

Neither position paper raised the following relevant points:

**4.1 The multi-agent scaling problem favors native options.**
With 5-20+ agents (the "growing trend" cited in the team context), the file-based system creates 10-40+ files in a flat directory (notification + pane companion per agent). The O(n) scan in `clear-notification.sh` runs on every pane switch. The native approach's `#{W:#{P:...}}` iterates panes in the tmux server's internal data structures, which are indexed, not a linear directory scan. Neither paper quantified this scaling behavior.

**4.2 The proposed features overwhelmingly favor structured data.**
Of the 14 proposed features, at least 9 require notification metadata beyond what the current format-string-in-a-file approach supports:
- Feature #1 (desktop notifications): needs clean message text, not tmux format codes.
- Feature #3 (priority levels): needs a priority field.
- Feature #4 (grouping/deduplication): needs structured project + message data.
- Feature #6 (agent state tracking): needs state enum.
- Feature #7 (dashboard popup): needs structured data for rendering.
- Feature #8 (callbacks): needs clean data for external consumers.
- Feature #10 (log analytics): needs structured log format.
- Feature #13 (rules engine): needs structured fields to match against.
- Feature #14 (timeline visualization): needs structured timestamps.

The file-based advocate's "Improvement #4" (structured notification format) acknowledges this need but does not recognize that it effectively concedes the data/presentation separation argument.

**4.3 Testing and correctness are architecture-dependent.**
The code analysis identifies D4 (no test infrastructure) as a design issue. Testing a file-based system requires filesystem mocking, temp directories, and cleanup. Testing a tmux-option-based system requires a running tmux server. Neither is trivial, but the native approach has fewer moving parts to test (no file race conditions, no stat portability, no sed parsing).

**4.4 A hybrid approach was not seriously considered.**
Neither paper explored using native tmux options for the display/clearing hot path while retaining files for logging, external integration, and persistence. This hybrid would capture the performance and correctness benefits of native options while preserving the file-based system's strengths for non-display use cases.

**4.5 The `#()` caching behavior mitigates some performance concerns.**
tmux caches `#()` results between `status-interval` ticks. The shell is forked once per interval, not once per redraw. This reduces the performance gap somewhat, though zero forks (native) is still strictly better than one fork per interval (file-based).

---

## 5. Decision Factors

Weighted for this specific team and use case:

| Factor | Weight | Favors | Rationale |
|--------|--------|--------|-----------|
| **Bug elimination** | 25% | Native | 3 critical + 6 high-severity bugs trace directly to file-based IPC. The native approach eliminates C1, C2, H6, and reduces H2, M6, M7 at the architectural level. This is the highest-weighted factor because the code analysis found serious issues. |
| **Multi-agent scalability** | 20% | Native | Growing to 5-20+ agents means 10-40+ files in a flat directory, O(n) scans on every focus event. Per-pane options scale with tmux's internal structures. The "growing multi-agent trend" makes this a forward-looking priority. |
| **Feature enablement** | 20% | Native | 9 of 14 proposed features require structured notification data. The native approach's data/presentation separation is a prerequisite for features #1, #3, #4, #6, #7, #8, #10, #13, #14. Implementing these on the file-based system would require sidecar metadata files, effectively building a second data layer. |
| **Status line performance** | 10% | Native | Zero-fork format expansion vs. shell fork + file I/O per interval. The gap is modest for 15-second intervals but meaningful for users who set `status-interval 1`. |
| **Integration simplicity** | 10% | File-Based | File writes from arbitrary processes remain the lowest-barrier integration mechanism. This matters for the plugin's extensibility story. |
| **Debugging & developer experience** | 10% | Slight File-Based | Files are marginally easier to inspect for developers unfamiliar with tmux internals. However, `tmux list-panes -a -F '#{@notif-msg}'` is not difficult, and the native approach's data is cleaner (no format code stripping). |
| **Migration risk** | 5% | File-Based | A rewrite carries risk. However, the current codebase has 31 documented bugs, so the risk of not rewriting is also substantial. |

**Weighted score: Native tmux approach is favored on factors representing ~75% of the total weight.**

---

## 6. Verdict

### Recommendation: Hybrid Architecture with Native Core

**The notification display and clearing system should migrate to native tmux per-pane options. The notification writing system should support both native options (primary) and a file-based compatibility layer (secondary).**

This is not a full endorsement of either pure approach. It is a recognition that each side is right about different parts of the system.

### Top 3 Deciding Factors

1. **Bug severity.** The 3 critical bugs and 6 high-severity bugs in the code analysis are overwhelmingly concentrated in the file-based IPC layer (race conditions, dual-file atomicity, O(n) scans). Migrating the hot path to native options eliminates these bugs at the architectural level rather than patching them individually. This is the single strongest argument in the entire debate.

2. **Feature roadmap alignment.** 9 of 14 proposed features require structured notification metadata. The native approach's data/presentation separation provides this as a natural consequence of the architecture. Building it on files would require adding metadata sidecar files, creating a second data layer that duplicates what tmux options already provide.

3. **Multi-agent scaling trajectory.** The team context explicitly identifies a "growing trend: multi-agent teams (5-20+ agents in parallel)." The O(n) directory scans, O(n) focus-event processing, and flat-file storage model are architectural bottlenecks that will worsen with scale. Per-pane options eliminate these scaling concerns.

### Conditions That Would Change the Verdict

- If the plugin needed to support tmux versions below 3.1 (where per-pane options do not exist), the file-based approach would be necessary.
- If a large ecosystem of third-party tools had built integrations against the file-based directory interface, the migration cost would outweigh the benefits. Currently, only Claude Code and Codex CLI integrate, so migration cost is low.
- If `#{W:#{P:...}}` nested format iteration proved unreliable or performance-poor at scale (20+ agents), a server-side aggregation script would be needed, partially negating the zero-fork benefit.
- If tmux's format string language proved too limited for features like notification grouping/deduplication (#4), a hybrid with a lightweight aggregation helper would be needed.

### Concessions to the File-Based Side

The File-Based advocate got several things right:

1. **The filesystem as a universal interface** is a genuinely powerful idea. The hybrid approach should preserve file-based logging and offer a file-based notification creation path for tools that cannot easily invoke `tmux set-option`.

2. **Crash persistence** is a real advantage, even if narrow. The hybrid approach should log all notification events to a file so that notification history survives server restarts (even though active notification display state does not need to).

3. **Debuggability with standard Unix tools** is a valid developer experience concern. The hybrid approach should include a `tmux-notif-debug` command that dumps all notification state to a readable format, and should retain the event log for post-hoc debugging.

4. **The "improve rather than rewrite" instinct** is generally sound engineering advice. The recommended implementation approach below is a phased migration, not a big-bang rewrite.

5. **Format string complexity** is a real maintenance burden. The Native paper's `#{W:#{P:#{?@notif-msg,...}}}` string is difficult to read, test, and maintain. The implementation should build these strings programmatically with good comments, not write them as raw string literals.

---

## 7. Recommended Path Forward

### Phase 0: Foundation (Week 1)

**Fix critical bugs in the current system first, before any architectural changes.**

1. **Fix C3 (unbounded log growth):** Add log rotation to `log_event()`. This is a one-line fix that benefits both architectures.
2. **Fix H1 (JSON parsing):** Add `jq` with grep/sed fallback. This fix is architecture-independent.
3. **Fix L6 (keybinding conflict):** Change default from `n` to a non-conflicting key.
4. **Add version check:** Verify tmux >= 3.2 at plugin init; display a clear error otherwise.
5. **Add basic test infrastructure (D4):** Even a simple test that sources the library, calls `tmux_alert`, and verifies the result.

### Phase 1: Native Core (Weeks 2-3)

**Migrate the notification hot path to native tmux options while keeping file-based logging.**

1. **Notification write:** Change `tmux_alert()` to set per-pane options:
   ```bash
   tmux set -p -t "$TMUX_PANE" @notif-msg "$msg"
   tmux set -p -t "$TMUX_PANE" @notif-project "$display_name"
   tmux set -p -t "$TMUX_PANE" @notif-source "$NOTIFY_SOURCE"
   tmux set -p -t "$TMUX_PANE" @notif-time "$(date +%H:%M:%S)"
   ```
   Continue writing to `events.log` for history/debugging.

2. **Notification display:** Replace `#(notification-reader.sh)` with a `#{W:#{P:...}}` format string. Build the format string programmatically in `claude-notifications.tmux` for readability.

3. **Notification clearing:** Replace the `clear-notification.sh` script and its 3 hooks with a single inline hook:
   ```bash
   tmux set-hook -g pane-focus-in \
     'if -F "#{@notif-msg}" "set -p -u @notif-msg ; set -p -u @notif-project ; set -p -u @notif-source ; set -p -u @notif-time ; refresh-client -S"'
   ```

4. **Next notification / picker:** Update to use `tmux list-panes -a -F '#{?@notif-msg,...}'` instead of file listing.

5. **File compatibility layer:** Optionally write a notification summary file to `~/.tmux-notifications/` for external tools that want to read it. This is a write-only path; the plugin itself reads from tmux options.

6. **Delete:** Remove `clear-notification.sh`, simplify `notification-reader.sh` (or remove entirely if pure format strings suffice). Remove the dual-file write. Remove the `.pane_*` companion file pattern.

**Bugs eliminated by Phase 1:** C1, C2, H2 (partially -- per-pane scoping means each pane has its own options), H3, H6, M4, M6, M7, M10, L1, L5, L7.

### Phase 2: Quick-Win Features (Week 4)

Implement the highest-impact, lowest-effort features from the feature research, all of which are easier on the new architecture:

1. **Feature #5 (configurable stale timeout):** With native options, stale cleanup is less critical (pane destruction handles it), but add a timer-based cleanup for long-lived panes. Store timeout in `@claude-notif-stale-timeout`.

2. **Feature #2 (audio alerts):** Add a `printf '\a'` bell option triggered in `tmux_alert()`. Trivial to implement.

3. **Feature #1 (desktop notifications):** Add `notify-send` / `osascript` / OSC 9 support. The native architecture makes this easy because `@notif-msg` contains clean text, no format-code stripping needed.

4. **Feature #9 (DND mode):** A single tmux option `@claude-notif-dnd` checked in `tmux_alert()`.

### Phase 3: Multi-Agent Features (Weeks 5-6)

Build features that matter for the growing multi-agent trend:

1. **Feature #3 (priority levels):** Add `@notif-priority` per-pane option. Update the format string to use priority-based colors.

2. **Feature #6 (agent state tracking):** Add `@agent-state` per-pane option (running/waiting/done). Set it from the hook events (`UserPromptSubmit` = running, `Notification` = waiting, `Stop` = done).

3. **Feature #4 (notification grouping):** The `#{W:#{P:...}}` format string can group by project using tmux's string comparison operators. If format-string-only grouping proves too complex, use a lightweight helper script invoked via `run-shell` only when a notification changes (not on every status refresh).

4. **Feature #8 (notification callbacks):** Add a callback option `@claude-notif-callback`. Invoke it from `tmux_alert()` with structured environment variables.

### Phase 4: Power User Features (Weeks 7-8)

1. **Feature #7 (dashboard popup):** Build a `display-popup` TUI that reads state from `tmux list-panes -a -F '...'`.

2. **Feature #10 (log analytics):** Switch `events.log` to a structured format (tab-separated or JSON lines). Build a summary popup.

3. **Feature #13 (rules engine):** If community demand warrants it. Start with a simple per-project config.

### Migration Strategy

- **No big-bang rewrite.** Each phase produces a working plugin.
- **Phase 1 is the critical migration.** It should be done as a single coherent PR to avoid maintaining two parallel systems.
- **Backward compatibility:** If any external tools read from `~/.tmux-notifications/`, the Phase 1 compatibility layer (writing a summary file) preserves that interface. Document the deprecation.
- **Testing:** Add integration tests that verify notification creation, display, clearing, and jump-to-pane behavior against a real tmux server. Use `tmux new-session -d` in tests.

---

## Summary

The file-based architecture was a reasonable first implementation that served the plugin well. Its strengths -- filesystem universality, crash persistence, simple debugging -- are real. But the code analysis revealed that the architecture's weaknesses are also real: 3 critical race conditions, O(n) scans on every focus event, shell forks on every status refresh, and data/presentation coupling that blocks 9 of 14 proposed features.

The native tmux approach eliminates these issues at the architectural level, not through patches. The hybrid recommendation preserves the file-based system's genuine strengths (logging, external integration compatibility) while moving the performance-critical and correctness-critical paths to native tmux primitives.

The filesystem was a good first solution. tmux's own infrastructure is the better second one.
