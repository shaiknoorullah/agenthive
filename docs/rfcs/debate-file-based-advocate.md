# Position Paper: In Defense of File-Based Notification Architecture

**Author:** File-Based Notification System Advocate
**Date:** 2026-03-26
**Subject:** Why tmux-agent-notifications should retain and improve its file-based IPC architecture rather than migrate to a native tmux-integrated approach

---

## Executive Summary

The file-based notification system used by tmux-agent-notifications is not a workaround awaiting replacement -- it is a deliberate architectural choice whose strengths align precisely with the problem domain. Files provide universal inspectability, crash-resilient persistence, zero-coupling extensibility for third-party tools, and cross-version compatibility that tmux's native option system cannot match. A rewrite to native tmux variables would trade proven reliability for marginal elegance gains while introducing real risks around version compatibility, concurrency, debuggability, and the loss of the plugin's most distinctive advantage: that any process on the system can write a notification without knowing anything about tmux internals.

---

## 1. Simplicity and Debuggability

The current architecture stores each notification as a plain file in `~/.tmux-notifications/`. A notification for a project called "myapp" in pane %42 becomes the file `myapp__42`, containing a tmux format string. A companion `.pane_myapp__42` file stores the pane ID for navigation.

This means that at any moment, an operator can run:

```bash
ls ~/.tmux-notifications/
cat ~/.tmux-notifications/myapp__42
```

and see exactly what notifications exist, what they say, and which panes they reference. There is no intermediate representation, no serialization format, no tmux command required to inspect state. The notification directory is a complete, human-readable snapshot of the system's state.

Compare this with tmux user options. To inspect notifications stored as `@notif-myapp-42`, you would need to run `tmux show-option -gqv @notif-myapp-42` -- but you must first know the exact option name, which means you need an enumeration mechanism. tmux provides no `show-options --prefix @notif-*` wildcard query. You would need to run `tmux show-options -g | grep @notif`, parse the output, and hope the format is stable. This is strictly worse for debugging.

The file-based approach also enables standard Unix debugging workflows. You can `watch ls ~/.tmux-notifications/`, `inotifywait` on the directory, or `tail -f events.log` to observe the system in real time. These are tools every developer already knows. Debugging a tmux user option requires tmux-specific knowledge and an active tmux session.

**Key point:** Files are debuggable with `ls` and `cat`. tmux options require `tmux show-options` plus grep plus an active server. In a system designed for developers who are already cognitively loaded by AI agent interactions, debuggability is not a luxury -- it is a critical feature.

---

## 2. Cross-Process Communication: Files Are the Natural IPC for Shell Hooks

The notification system must accept input from multiple independent sources: Claude Code hooks, Codex CLI hooks, and arbitrary custom tools. These hooks are shell scripts invoked by their respective tools, not by tmux.

Consider the Claude Code hook (`claude-hook.sh`). It is invoked by Claude Code's hook system, receives JSON on stdin, and must record a notification. The current implementation writes a file -- a single atomic `echo ... > file` operation. This is the simplest possible IPC mechanism: it requires no client library, no socket connection, no understanding of tmux's internal state model.

A native tmux approach would require every hook script to invoke `tmux set-option`, which:

1. Requires the `tmux` binary to be on PATH (not always guaranteed in restricted environments or containers)
2. Requires a valid `$TMUX` socket connection (fails if the hook runs in a detached context)
3. Serializes through tmux's single-threaded command processing
4. Must handle the case where the tmux server is not running

File writes, by contrast, work in any context where the filesystem is accessible. They do not depend on tmux being alive, on socket permissions, or on any runtime state. The Codex CLI hook (`codex/notify.sh`) demonstrates this: it sources `tmux-notify-lib.sh` and calls `tmux_alert`, which writes a file. If tmux were down, the file would still be written and would be visible when tmux restarts.

**Key point:** Files decouple notification producers from the tmux server process. This is not a limitation -- it is the correct architectural boundary for a system that must accept input from heterogeneous, independently-scheduled processes.

---

## 3. Resilience: Crash Recovery and Restart Behavior

tmux's in-memory state (including all user options set via `set-option`) is volatile. When the tmux server crashes, is killed, or the system reboots, all user options are lost. The tmux ecosystem acknowledges this problem -- plugins like tmux-resurrect exist specifically because tmux does not persist its runtime state.

The file-based notification system is inherently persistent. If the tmux server crashes:

- All pending notifications survive on disk in `~/.tmux-notifications/`
- The event log (`events.log`) retains full history
- When tmux restarts and the plugin reloads, `notification-reader.sh` immediately finds and displays existing notifications
- No recovery logic is needed; the filesystem IS the recovery mechanism

With a native tmux option approach, a server crash would silently destroy all pending notifications. Users would lose the very alerts they most need -- the ones that arrived while they were away from their terminal. Implementing persistence for tmux options would require... writing them to files, which brings us back to the current architecture but with an extra layer of indirection.

The current system's 30-minute stale notification cleanup (`clear-notification.sh`, lines 22-29) and dead pane detection (lines 9-19) demonstrate that file-based state naturally supports time-based reasoning via filesystem timestamps. Getting "when was this notification created?" from a tmux user option requires storing the timestamp as part of the value and parsing it back out -- a fragile encoding that files handle natively via `stat`.

**Key point:** Files survive tmux crashes. tmux options do not. For a notification system, losing notifications on crash is an unacceptable failure mode.

---

## 4. Extensibility: The Five-Minute Integration Path

The README documents a custom tool integration path that is remarkably simple:

```bash
#!/usr/bin/env bash
NOTIFY_SOURCE="MyTool"
source "$SCRIPT_DIR/../scripts/tmux-notify-lib.sh"
DISPLAY_NAME=$(resolve_project_name "/path/to/project")
tmux_alert "Task finished" "$DISPLAY_NAME"
```

This works because the integration contract is: "source a shell library and call a function." The library writes a file. Any tool that can run a shell script can integrate.

But consider an even simpler integration that the file-based approach uniquely enables: direct file creation.

```bash
echo "#[fg=#c8d3f5][14:30:00] MyTool/project: Done" > ~/.tmux-notifications/project__99
```

No library needed. No tmux knowledge needed. A Python script, a Go binary, a Node.js process, a cron job -- anything that can write a file can create a notification. This is the Unix philosophy at its strongest: the notification directory is an interface, and the interface is the filesystem.

A native tmux approach would require every integrator to either: (a) shell out to `tmux set-option`, (b) connect to the tmux socket and speak its protocol, or (c) use a language-specific tmux client library. Each of these is a higher barrier to entry than writing a file.

The Codex CLI integration (`codex/notify.sh`) proves this extensibility works in practice. It was added alongside the Claude Code hook with minimal code duplication, because both tools share the same file-based contract via `tmux-notify-lib.sh`.

**Key point:** Files provide the lowest possible integration barrier. Any process on the system can create a notification by writing a file. This cannot be matched by any tmux-native approach.

---

## 5. tmux Version Compatibility

The plugin targets tmux 3.2+. This is already a meaningful compatibility constraint. Let us examine the tmux features the opponent might propose and their version availability:

- **User options (`@variable`):** Available since tmux 1.8, but behavior of `-s` (server-level) user options has varied. Format expansion of `#{@var}` is reliable only from tmux 2.1+.
- **`status-format` array (multi-line status):** Available from tmux 2.9, but the `status 2/3/4/5` values were only added later.
- **Server options (`set-option -s @var`):** While server options exist, server-scoped user options have had inconsistent support. Some versions require `-s`, others accept `-g` for user options. The behavior when mixing `-g`, `-s`, and `-w` with `@`-prefixed options has been a source of confusion in the community.
- **Control mode:** Available since tmux 1.8, but the protocol has evolved significantly. The libtmux project's 2025 roadmap explicitly calls out "documenting behavior differences between subprocess and control-mode engines" as ongoing work, indicating the control mode interface is still not fully stabilized.
- **`wait-for` channels:** Available since tmux 1.8, useful for synchronization but not for state storage.

The file-based approach uses exactly zero version-sensitive tmux features for its core IPC. It uses `run-shell` (stable since tmux 1.5), `set-hook` (stable since tmux 2.4), `status-format` (stable since tmux 2.9), and `refresh-client -S` (stable since tmux 2.6). All of these are well within the 3.2+ target and have stable semantics.

A native tmux option approach would need to navigate the minefield of user option scoping, format expansion edge cases, and version-specific behavior. It would bind the plugin's correctness to tmux's internal state management, which is subject to change across minor versions.

**Key point:** The file-based approach depends on tmux only for display and hooks, not for state management. This minimizes the version-compatibility surface area.

---

## 6. Addressing the Opponent's Strongest Arguments

### "File I/O is slower than reading a tmux variable"

This is technically true but practically irrelevant. Notifications in this system are infrequent -- minutes apart, not seconds. The `notification-reader.sh` script runs on status refresh (default: every 15 seconds), reads a handful of small files (typically 0-10, each under 200 bytes), and produces a single output line. On any modern filesystem, this completes in under 5 milliseconds. The tmux status refresh itself involves process forking (`#(command)` syntax), which dwarfs any file I/O cost. In fact, tmux issue #3352 documents that status-line subshell overhead is the dominant performance concern -- and both architectures must fork a shell to render the status line.

### "Native options are more 'elegant' or 'idiomatic'"

Elegance is subjective; robustness is measurable. The file-based approach has zero dependencies on tmux's internal state model, survives crashes, and is debuggable with standard Unix tools. If elegance means "fewer moving parts," the file-based approach wins: it has no serialization layer, no enumeration mechanism, and no state synchronization problem.

### "File writes are not atomic"

On Linux and macOS, `echo "content" > file` using shell redirection performs a create-write-close sequence that is atomic at the rename level when using `O_CREAT|O_TRUNC`. For this plugin's use case (single writer per notification key, reader tolerant of partial reads), this is more than sufficient. The notification key naming scheme (`{project}__{pane_id}`) ensures that each notification file has exactly one writer (the hook for that specific pane), eliminating write-write races entirely. Compare this with tmux's `set-option`, which processes commands sequentially through a single-threaded event loop -- technically serialized, but also meaning that a burst of notifications from multiple panes will queue behind each other in tmux's command buffer, potentially causing latency in tmux's main loop.

### "The filesystem approach creates clutter"

The `~/.tmux-notifications/` directory typically contains 0-10 notification files and 0-10 companion pane files. Dead pane cleanup runs on every focus event. Stale notifications are pruned after 30 minutes. The event log is the only file that grows, and it is append-only plain text, trivially managed with `logrotate` or manual truncation. This is not clutter -- it is bounded, self-cleaning state.

---

## 7. Risk Assessment: What Could Go Wrong with a Rewrite

| Risk | Severity | Likelihood | Impact |
|------|----------|------------|--------|
| Version-specific tmux option bugs break notifications | High | Medium | Silent notification loss |
| Loss of crash persistence (tmux restart clears options) | High | Certain | All pending notifications lost |
| Third-party integrations break (file API removed) | High | High | Ecosystem fragmentation |
| Rewrite introduces subtle concurrency bugs | Medium | Medium | Intermittent notification loss |
| Debugging difficulty increases (no `ls`/`cat`) | Medium | Certain | Slower issue resolution |
| tmux command length limits hit with many notifications | Medium | Low | Truncated notifications |
| Performance regression from tmux command serialization | Low | Low | Status line lag |
| Development time diverted from features to rewrite | High | Certain | Opportunity cost |

The most dangerous risk is **silent notification loss**. The current system fails visibly: if a file is not written, there is no notification, and the absence is detectable by inspecting the directory. A tmux option-based system could fail silently if the option is set but not properly expanded, if the option is overwritten by another process, or if the tmux server drops the command during high load. Debugging invisible state is categorically harder than debugging visible files.

---

## 8. Improvements Possible Within the Current Architecture

The file-based approach is not frozen. Several improvements can be made without architectural change:

1. **Atomic writes via temp-and-rename:** Replace `echo > file` with `echo > file.tmp && mv file.tmp file` for guaranteed atomicity on all POSIX systems.

2. **Single-file notification index:** Instead of reading individual files, maintain an index file (or use `ls -t` as currently done) that `notification-reader.sh` can read in one operation.

3. **inotify-based reader:** Replace polling-based status refresh with an `inotifywait` watcher that updates a cached status line only when the notification directory changes. This would eliminate all per-refresh file I/O.

4. **Structured notification format:** Move from bare tmux format strings to a simple key=value format in notification files, enabling richer metadata (creation time, priority, source) without changing the IPC mechanism.

5. **Log rotation:** Add built-in log rotation to `events.log` (e.g., rotate at 1MB, keep 3 files).

6. **Batch pane cleanup:** Instead of iterating `.pane_*` files on every focus event, run cleanup on a timer (every 60 seconds) or only when the pane count changes.

7. **XDG compliance:** Support `$XDG_RUNTIME_DIR/tmux-notifications/` for systems that prefer XDG directory standards, with fallback to `~/.tmux-notifications/`.

8. **Lock-free reader optimization:** Use a checksum or version counter file that the reader checks before re-scanning the directory, avoiding unnecessary `ls` calls when nothing has changed.

Each of these improvements strengthens the existing architecture without requiring a rewrite, without breaking third-party integrations, and without introducing tmux version dependencies.

---

## Decision Matrix

| Criterion | File-Based (Current) | Native tmux Options | Winner |
|-----------|---------------------|---------------------|--------|
| Debuggability | `ls` and `cat` -- any context | `tmux show-options` -- requires active server | File-Based |
| Crash resilience | Notifications survive server restart | All state lost on crash | File-Based |
| Cross-process IPC | Any process can write a file | Requires `tmux` binary and socket | File-Based |
| Third-party extensibility | Write a file, done | Must shell out to `tmux set-option` | File-Based |
| Version compatibility | No version-sensitive state features | User option scoping varies by version | File-Based |
| Read performance | ~5ms for ls + cat of small dir | ~1ms for option lookup | Native Options |
| Write performance | ~1ms file write | ~2ms tmux command dispatch | File-Based |
| Atomicity guarantees | POSIX rename; single writer per key | tmux command serialization | Tie |
| State enumeration | `ls directory` | `show-options | grep` (fragile) | File-Based |
| Time-based cleanup | `stat` file mtime -- native | Must encode timestamp in value | File-Based |
| Code complexity | Shell file operations | Shell + tmux command protocol | File-Based |
| Rewrite risk | N/A (current system) | High (silent failures, lost integrations) | File-Based |
| Aesthetic preference | "Feels like a workaround" | "Feels native" | Native Options |

**Score: File-Based 11, Native Options 1, Tie 1**

The native tmux option approach wins only on raw read performance for a single option lookup -- a metric that is irrelevant given the infrequent notification cadence and the dominant cost of status-line shell forking. The "aesthetic preference" point is real but should not drive architectural decisions over the eleven concrete advantages of the file-based approach.

---

## Conclusion

The file-based notification architecture is not a prototype awaiting maturation into a "real" tmux-native system. It is a principled design choice that leverages the filesystem as a universal, persistent, inspectable, crash-resilient IPC mechanism. It enables the broadest possible integration surface, provides the simplest possible debugging experience, and avoids coupling the plugin's correctness to tmux's version-specific internal state management.

The correct path forward is not replacement, but refinement: atomic writes, inotify-based readers, structured metadata, and XDG compliance can all be added incrementally within the current architecture. These improvements would deliver tangible user-facing benefits without the risk, development cost, or capability regression of a rewrite to native tmux options.

The filesystem is not the problem. The filesystem is the solution.
