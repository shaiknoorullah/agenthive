# Feature Research: tmux-agent-notifications

**Date:** 2026-03-26
**Status:** Proposal
**Author:** Feature research agent

---

## Executive Summary

This document proposes 14 new features for tmux-agent-notifications, informed by research into the tmux plugin ecosystem, AI coding agent workflows, terminal notification standards, and multi-agent orchestration patterns emerging in 2026.

The proposals are organized into three tiers:
- **Must-have (5):** Features that address clear gaps compared to competing plugins and common user requests.
- **Nice-to-have (5):** Features that meaningfully improve the experience for power users running multiple agents.
- **Experimental (4):** Forward-looking features that push the boundaries of what a tmux notification plugin can do.

---

## Competitive Landscape

Before proposing features, it is worth noting what exists:

| Plugin | Key Differentiators |
|--------|-------------------|
| **tmux-notify** ([rickstaa/tmux-notify](https://github.com/rickstaa/tmux-notify)) | Desktop notifications via libnotify, visual bell, urgency hints. Process-completion focused. |
| **tmux-agent-indicator** ([accessd/tmux-agent-indicator](https://github.com/accessd/tmux-agent-indicator)) | Pane border colors, window title colors, status bar icons for agent states (running/needs-input/done). |
| **tmux-agent-status** ([samleeney/tmux-agent-status](https://github.com/samleeney/tmux-agent-status)) | At-a-glance session status for Claude working vs idle. |
| **claude-code-audio-hooks** ([ChanMeng666/claude-code-audio-hooks](https://github.com/ChanMeng666/claude-code-audio-hooks)) | Audio notification system with 22 hook sounds, ElevenLabs voice recordings. |
| **Claude HUD** | Real-time monitoring of context window usage, active tools, running agents, task progress. |
| **claude_code_agent_farm** ([Dicklesworthstone/claude_code_agent_farm](https://github.com/Dicklesworthstone/claude_code_agent_farm)) | 20+ parallel agents, lock-based coordination, real-time dashboard, heartbeat tracking. |

Our plugin's strength is its focused, file-based notification model with smart per-pane clearing and dynamic status line rendering. The features below build on that foundation without bloating the core.

---

## MUST-HAVE Features

### 1. Native Desktop Notifications (notify-send / osascript / OSC 9)

**What it does:**
When an agent completes or needs attention, send a native OS desktop notification in addition to (or instead of) the tmux status bar notification. Supports `notify-send` on Linux, `osascript` on macOS, and OSC 9/777 escape sequences for terminals that support them (iTerm2, VS Code terminal, Windows Terminal).

**Why it is useful:**
The tmux status bar is invisible when a developer switches to a browser, Slack, or another application. This is the single most requested feature across Claude Code notification discussions. Users currently cobble together separate hook scripts to get desktop notifications (see [kane.mx](https://kane.mx/posts/2025/claude-code-notification-hooks/), [quemy.info](https://quemy.info/2025-08-04-notification-system-tmux-claude.html), [d12frosted.io](https://www.d12frosted.io/posts/2026-01-05-claude-code-notifications)). Integrating it into the plugin eliminates duplicated effort.

**Complexity:** Low-Medium
- Platform detection is straightforward (`uname` check)
- OSC sequences require tmux passthrough (`set -g allow-passthrough on`) or direct tty write
- Need to rate-limit to avoid notification floods

**Configuration:**
```tmux
set -g @claude-notif-desktop "on"           # off (default), on
set -g @claude-notif-desktop-sound "on"     # play system sound
set -g @claude-notif-desktop-urgency "normal"  # low, normal, critical
```

**Example interaction:**
A developer is reading docs in Firefox. Claude finishes a task in tmux pane %5. A desktop notification bubble appears: "Claude/my-project: Agent has finished". Clicking the notification can optionally raise the terminal window.

---

### 2. Audio Alerts

**What it does:**
Play a short audio cue when a notification fires. Uses `afplay` (macOS), `paplay`/`aplay` (Linux), or a terminal bell (BEL character, `\a`).

**Why it is useful:**
Audio is the fastest channel to interrupt focused work. The claude-code-audio-hooks project demonstrates significant demand for this, but it is a standalone hooks-only solution that does not integrate with tmux status bar notifications. Offering a simple bell/sound option within this plugin covers the majority use case without requiring a separate installation.

**Complexity:** Low
- Terminal bell is a single `printf '\a'`
- System sound is a single command (`afplay`, `paplay`)
- Optional: bundled default sound file

**Configuration:**
```tmux
set -g @claude-notif-sound "bell"           # off (default), bell, system, /path/to/file.wav
```

**Example interaction:**
Developer has headphones on, working in another pane. A soft chime plays when Codex finishes in a background pane. No need to visually scan the status bar.

---

### 3. Notification Priority Levels

**What it does:**
Assign priority levels to notifications: `info`, `warning`, `critical`. Each level gets its own color scheme and behavior. Critical notifications (e.g., agent errors, permission requests) are styled more urgently and are never auto-cleared by the stale cleanup timer.

**Why it is useful:**
Currently all notifications look the same. An agent finishing normally and an agent hitting an error both get the same yellow bold treatment. When running 5+ agents, the ability to visually triage at a glance matters. This also enables priority-based filtering in the notification picker and priority-based routing for desktop/audio alerts.

**Complexity:** Medium
- Extend `tmux_alert` to accept a priority parameter
- Add per-priority color options
- Modify notification-reader.sh to sort by priority
- Modify stale cleanup to respect priority (critical notifications persist longer)

**Configuration:**
```tmux
set -g @claude-notif-critical-fg "red"
set -g @claude-notif-critical-style "bold,blink"
set -g @claude-notif-warning-fg "yellow"
set -g @claude-notif-info-fg "#c8d3f5"
set -g @claude-notif-stale-timeout "1800"           # default 30min for info
set -g @claude-notif-stale-timeout-critical "7200"  # 2h for critical
```

**Example interaction:**
Status bar shows:
```
[14:30] Claude/api-server: Task failed   [14:31] Claude/frontend: Agent has finished
        ^^^^ red, bold, blink                     ^^^^ default dim
```
The developer immediately knows which pane needs attention first.

---

### 4. Notification Grouping and Deduplication

**What it does:**
When multiple notifications fire for the same project within a short window (e.g., agent teams where 3 teammates finish within seconds of each other), group them into a single status bar entry like `Claude/my-project: 3 agents finished` instead of showing 3 separate identical-looking notifications.

**Why it is useful:**
Agent teams (now shipping across all major platforms) routinely produce burst notifications. The [claude_code_agent_farm](https://github.com/Dicklesworthstone/claude_code_agent_farm) project runs 20+ agents in parallel. Without grouping, the status bar overflows instantly and the "+N more" indicator loses its usefulness because the user cannot tell whether 5 notifications represent 5 different projects or 5 agents in the same project.

Alert fatigue research from [Datadog](https://www.datadoghq.com/blog/reduce-alert-storms-datadog/) and [Netdata](https://www.netdata.cloud/academy/what-is-alert-fatigue-and-how-to-prevent-it/) consistently identifies grouping as the first mitigation strategy.

**Complexity:** Medium
- Group by project name (the part before `__` in the notification key)
- Show individual pane details in the picker popup
- Preserve jump-to-pane behavior (jump to oldest in group)

**Example interaction:**
Instead of:
```
[14:30] Claude/my-project: Agent has finished  |  [14:30] Claude/my-project: Agent has finished  |  [14:30] Claude/my-project: Agent has finished
```
Show:
```
[14:30] Claude/my-project: 3 agents finished
```
Pressing `prefix + S` (picker) expands the group to show individual panes.

---

### 5. Configurable Stale Timeout

**What it does:**
Make the 30-minute stale notification cleanup timeout configurable via a tmux option, rather than being hardcoded.

**Why it is useful:**
30 minutes is a reasonable default, but some workflows need different values. Long-running agent tasks (multi-hour refactors) may produce a notification that is still relevant after 30 minutes. Conversely, rapid iteration workflows may want 5-minute cleanup. This is a small change with outsized value for configurability.

**Complexity:** Very Low
- Replace the hardcoded `1800` in `clear-notification.sh` with a tmux option lookup
- Single line change plus documentation

**Configuration:**
```tmux
set -g @claude-notif-stale-timeout "3600"   # seconds, default 1800
```

**Example interaction:**
A developer running overnight agent tasks sets the timeout to 4 hours. Notifications from 2am are still visible when they check at 5am.

---

## NICE-TO-HAVE Features

### 6. Agent State Tracking (Running / Waiting / Done)

**What it does:**
Track agent lifecycle state per pane: `running` (user submitted prompt), `waiting` (agent needs input/approval), `done` (agent stopped). Surface this state via a tmux user option (`@agent-state-<pane_id>`) that themes and other plugins can consume, and optionally display a compact agent dashboard in the status bar or a popup.

**Why it is useful:**
The [tmux-agent-indicator](https://github.com/accessd/tmux-agent-indicator) plugin exists specifically for this purpose, which validates the demand. However, it is a separate plugin with its own hook system. Since tmux-agent-notifications already processes the same hook events (`UserPromptSubmit` = running, `Notification` = waiting, `Stop` = done), adding state tracking is a natural extension that avoids requiring users to install two plugins.

**Complexity:** Medium
- Store state in `$NOTIF_DIR/.state_<project>__<pane>` files
- Set tmux user options: `tmux set -p -t $PANE_ID @agent-state "running"`
- Optional: compact status bar widget showing colored dots per agent

**Configuration:**
```tmux
set -g @claude-notif-state-tracking "on"
set -g @claude-notif-state-running-icon ""
set -g @claude-notif-state-waiting-icon ""
set -g @claude-notif-state-done-icon ""
```

**Example interaction:**
Status line shows dots next to agent names:
```
 api-server   frontend   docs-gen
```
Green = running (leave it alone), yellow = needs input, blue = done (go check results).

---

### 7. Notification Dashboard Popup

**What it does:**
A `display-popup` based dashboard (bound to a configurable key, e.g., `prefix + D`) that shows a rich, interactive view of all agents: their state, last notification, project, pane ID, time since last activity, and notification history. Supports keyboard navigation to jump to any agent's pane.

**Why it is useful:**
The current fzf picker shows notification text but lacks context. When running 5-10 agents, developers need a bird's-eye view that answers: "Which agents are done? Which need me? Which are still working? How long has each been running?" The [claude_code_agent_farm](https://github.com/Dicklesworthstone/claude_code_agent_farm) project built a custom real-time dashboard for exactly this reason.

tmux `display-popup` (3.2+) is ideal for this: it floats over the workspace, dismisses with Escape, and can run any script.

**Complexity:** Medium-High
- Build a TUI script (bash + tput, or optionally a small Go/Rust binary)
- Read state files, notification files, and log history
- Keyboard navigation and pane jumping
- Auto-refresh (every 2-5 seconds)

**Configuration:**
```tmux
set -g @claude-notif-key-dashboard "D"
```

**Example interaction:**
```
+------------------------------------------------------+
|  Agent Dashboard                           [q] close  |
|------------------------------------------------------|
|  #  Project        Agent   State     Last Activity    |
|  1  api-server     Claude   done    14:30 (5m ago)   |
|  2  frontend       Claude   run     14:33 (2m ago)   |
|  3  data-pipeline  Codex    wait    14:34 (1m ago)   |
|  4  docs-gen       Claude   done    14:20 (15m ago)  |
|------------------------------------------------------|
|  [Enter] jump to pane  [d] dismiss notif  [a] all    |
+------------------------------------------------------+
```

---

### 8. Notification Callbacks / Webhook Support

**What it does:**
Allow users to define a callback script or webhook URL that fires whenever a notification is created. The callback receives structured data: project name, source tool, message, priority, pane ID, timestamp.

**Why it is useful:**
Power users want to integrate notifications with external systems: Slack channels, Discord bots, phone push notifications (via Pushover/ntfy), home automation (turn on a desk lamp when an agent finishes), or custom dashboards. Rather than building each integration, exposing a generic callback hook lets the community build what they need.

**Complexity:** Low-Medium
- Add a single `run-shell` or script invocation in `tmux_alert`
- Pass data as environment variables or JSON to stdin
- Users write their own callback scripts

**Configuration:**
```tmux
set -g @claude-notif-callback "~/.local/bin/my-notif-handler.sh"
```

**Example callback script:**
```bash
#!/usr/bin/env bash
# Receives: NOTIF_PROJECT, NOTIF_SOURCE, NOTIF_MESSAGE, NOTIF_PANE, NOTIF_TIMESTAMP
curl -s "https://ntfy.sh/my-agents" \
  -d "${NOTIF_SOURCE}/${NOTIF_PROJECT}: ${NOTIF_MESSAGE}" \
  -H "Title: Agent Notification" \
  -H "Priority: default"
```

---

### 9. Do Not Disturb Mode

**What it does:**
A toggle (keybinding or tmux option) that suppresses all visual notifications, audio alerts, and desktop notifications. Notifications are still logged and queued -- they simply are not displayed until DND mode is turned off, at which point all queued notifications appear.

**Why it is useful:**
When pair programming, screen sharing, recording a demo, or in deep focus on a specific pane, notification noise in the status bar is distracting. The status line constantly changing can pull attention. DND mode provides an explicit "I know my agents are running, leave me alone until I am ready" signal.

**Complexity:** Low
- A flag file (`$NOTIF_DIR/.dnd`) or tmux option (`@claude-notif-dnd`)
- `tmux_alert` checks the flag before writing notification files
- Still logs events
- Toggle key writes/removes the flag

**Configuration:**
```tmux
set -g @claude-notif-key-dnd "Q"   # prefix + Q to toggle DND
```

**Example interaction:**
Developer presses `prefix + Q`. Status line shows a small indicator: `[DND]`. Three agents finish while DND is active. Developer presses `prefix + Q` again. Three notifications appear simultaneously in the status bar.

---

### 10. Log Analytics and Session Summary

**What it does:**
Extend the log viewer (`prefix + key_log`) with summary statistics: total notifications today, notifications per project, average time between agent start and completion, busiest hour, error rate. Optionally, generate a markdown summary file at end of day.

**Why it is useful:**
Developers running agents all day accumulate hundreds of log entries. Raw `tail -f` of events.log is not actionable. Summary statistics help answer: "How productive were my agents today? Which project had the most errors? Am I spending too much time on approval requests?" This data informs workflow optimization -- for example, if 40% of notifications are approval requests, the developer might adjust their agent's autonomy settings.

**Complexity:** Medium
- Parse events.log with awk/grep to extract statistics
- Render in the display-popup log viewer
- Optional: structured log format (tab-separated or JSON lines) for easier parsing

**Configuration:**
```tmux
set -g @claude-notif-log-format "tsv"      # plain (default), tsv, jsonl
set -g @claude-notif-key-summary "M"       # prefix + M for summary popup
```

**Example interaction:**
```
+----------------------------------------------------+
|  Session Summary (2026-03-26)            [q] close  |
|----------------------------------------------------|
|  Total notifications:  47                           |
|  Projects:  api-server (18), frontend (15),         |
|             data-pipeline (9), docs (5)             |
|  By type:   done (31), needs attention (12),        |
|             error (4)                               |
|  Avg response time:  4m 12s                         |
|  Busiest hour:  14:00-15:00 (14 notifications)      |
+----------------------------------------------------+
```

---

## EXPERIMENTAL Features

### 11. Pane Output Monitoring (Silence Detection)

**What it does:**
Use tmux's built-in `monitor-silence` mechanism or periodic `capture-pane` to detect when an agent pane has gone silent (no output for N seconds) without firing a hook. This catches edge cases where an agent hangs, crashes, or the hook fails to fire.

**Why it is useful:**
Hook-based notification is reliable when hooks fire. But agents can crash without triggering a Stop hook, leaving the developer unaware that an agent is dead. Silence detection acts as a safety net. The tmux man page documents `monitor-silence` as a window option, and the plugin could leverage `capture-pane -p` to check for known "waiting for input" patterns in the pane content.

**Complexity:** High
- Configuring `monitor-silence` per-window is straightforward but coarse
- Pattern matching on pane output requires periodic polling (`capture-pane -p -t $pane | grep pattern`)
- False positives are a risk (agent might be legitimately silent during a long compile)
- Need careful interaction with existing hook-based notifications

**Configuration:**
```tmux
set -g @claude-notif-silence-detect "on"
set -g @claude-notif-silence-timeout "120"   # seconds
set -g @claude-notif-silence-message "Agent may be idle"
```

**Example interaction:**
An agent crashes mid-task. No Stop hook fires. After 2 minutes of silence in the pane, a notification appears: `[14:45] Claude/api-server: Agent may be idle`. The developer investigates and finds the crash.

---

### 12. Cross-Machine Notification Relay

**What it does:**
For developers who SSH into remote machines and run agents there, relay notifications back to the local machine. Uses SSH reverse port forwarding, a lightweight relay daemon, or file-based sync (e.g., `~/.tmux-notifications/` synced via a small watcher script).

**Why it is useful:**
A common workflow pattern: developer SSHes into a powerful remote machine, starts 5 Claude Code agents, then works locally on other things. Remote tmux notifications are invisible unless the developer is actively viewing the remote tmux session. The [AI Coding Agent Dashboard](https://blog.marcnuri.com/ai-coding-agent-dashboard) project was built specifically to solve cross-device agent monitoring.

**Complexity:** High
- Multiple transport options to support (SSH, HTTP, filesystem)
- Security considerations (notification content may contain project names)
- Need a local receiver component
- Platform-specific local notification integration

**Configuration:**
```tmux
set -g @claude-notif-relay "ssh"
set -g @claude-notif-relay-target "user@local-machine"
```

**Example interaction:**
Developer is on their laptop. An agent finishes on the remote beefy server. A desktop notification appears on the laptop: "remote-server: Claude/api-server: Agent has finished".

---

### 13. Notification Rules Engine

**What it does:**
A simple rules system that lets users define per-project or per-source notification behavior. Rules can suppress, redirect, escalate, or transform notifications based on pattern matching on the project name, source tool, or message content.

**Why it is useful:**
Not all projects are equally important. A developer might want critical desktop notifications for the production API server agent but quiet status-bar-only notifications for a docs-generation agent. Current behavior is global -- all notifications look and behave the same. A rules engine provides fine-grained control without complicating the default experience.

**Complexity:** High
- Rules file format design (INI, TOML, or shell-based)
- Pattern matching engine
- Rule evaluation in the hot path (notification creation)
- Documentation and examples

**Configuration file** (`~/.tmux-notifications/rules.conf`):
```ini
# Suppress notifications from docs-gen project
[docs-gen]
action = suppress

# Desktop + sound for api-server errors
[api-server]
match_message = *error*|*failed*
desktop = critical
sound = /path/to/alert.wav

# All Codex notifications are info-only
[*]
match_source = Codex
priority = info
```

**Example interaction:**
Developer configures rules once. From then on, the docs-gen agent runs silently (still logged), the api-server gets special treatment for errors, and Codex notifications are always low-priority.

---

### 14. Agent Timeline Visualization

**What it does:**
A popup (`display-popup`) that renders a Gantt-chart-style timeline of agent activity over the current session. Each row is an agent/project, each bar represents a run (from UserPromptSubmit to Stop), colored by outcome (green = success, red = error, yellow = needed input). The timeline uses Unicode block characters for rendering.

**Why it is useful:**
After a long day of multi-agent work, developers lack visibility into how their agents performed over time. Which agents were idle? Which had long runs? Were there cascading failures? A visual timeline answers these questions at a glance. This is inspired by CI/CD pipeline visualizations and the real-time dashboards in orchestration tools like [claude_code_agent_farm](https://github.com/Dicklesworthstone/claude_code_agent_farm).

**Complexity:** Very High
- Requires structured logging of start/stop events with timestamps
- Terminal-based Gantt chart rendering with Unicode
- Time-axis scaling and scrolling
- Might benefit from a compiled helper (Go/Rust) for performance

**Configuration:**
```tmux
set -g @claude-notif-key-timeline "L"
```

**Example interaction:**
```
+----------------------------------------------------------------+
|  Agent Timeline (last 2 hours)                      [q] close  |
|----------------------------------------------------------------|
|  14:00    14:15    14:30    14:45    15:00    15:15    15:30    |
|  api-srv  [===========]         [====ERR]  [=======]           |
|  fronted  [=====][===]     [================]                  |
|  datapipe      [========================WAIT===]              |
|  docs-gen [==][==][==][==][==][==][==]                          |
|                                                                |
|   [=] running   [ERR] error   [WAIT] awaiting input            |
+----------------------------------------------------------------+
```

---

## Implementation Roadmap

A suggested ordering based on impact-to-effort ratio:

| Phase | Features | Rationale |
|-------|----------|-----------|
| **Phase 1** (quick wins) | #5 Configurable stale timeout, #2 Audio alerts (bell only) | Trivial to implement, immediately useful |
| **Phase 2** (core value) | #1 Desktop notifications, #3 Priority levels | Addresses the top user pain points |
| **Phase 3** (multi-agent) | #4 Notification grouping, #6 Agent state tracking | Essential as agent teams become mainstream |
| **Phase 4** (power user) | #9 DND mode, #7 Dashboard popup, #8 Callbacks | Builds on state tracking foundation |
| **Phase 5** (analytics) | #10 Log analytics, #5 Configurable stale timeout (priority-aware) | Requires structured logging from Phase 2-3 |
| **Phase 6** (experimental) | #11 Silence detection, #13 Rules engine, #12 Cross-machine relay, #14 Timeline | High effort, validates with community interest first |

---

## Architecture Considerations

Several proposed features share infrastructure needs:

1. **Structured logging.** Features #10, #14, and #6 all benefit from a structured log format. Switching events.log from freeform text to tab-separated or JSON lines (with backward-compatible plain-text rendering) should be done early.

2. **Priority metadata.** Features #3, #4, #9, and #13 need notification metadata beyond the current "formatted tmux string in a file" approach. Consider a metadata sidecar file (`.meta_<key>`) or embedding structured data in the notification file with a separator.

3. **Callback architecture.** Feature #8 (callbacks) naturally supports #1 (desktop notifications) and #2 (audio alerts) as built-in callbacks. Implementing the callback hook first, then building desktop/audio as default callbacks, produces a cleaner architecture.

4. **State files.** Feature #6 (state tracking) creates files that features #7 (dashboard) and #14 (timeline) consume. The state file format should be designed with these consumers in mind.

5. **Rate limiting.** Features #1, #2, and #4 all touch on the same problem: burst notifications. A shared rate-limiting mechanism (e.g., minimum interval between desktop notifications per project) prevents notification floods without duplicating logic.

---

## Research Sources

- [tmux-notify](https://github.com/rickstaa/tmux-notify) - Process completion notifications via libnotify
- [tmux-agent-indicator](https://github.com/accessd/tmux-agent-indicator) - Visual agent state feedback
- [tmux-menus](https://github.com/jaclu/tmux-menus) - Popup menu system for tmux
- [claude-code-audio-hooks](https://github.com/ChanMeng666/claude-code-audio-hooks) - Audio notifications for Claude Code
- [claude_code_agent_farm](https://github.com/Dicklesworthstone/claude_code_agent_farm) - 20+ parallel agent orchestration
- [parallel-cc](https://github.com/frankbria/parallel-cc) - Parallel Claude Code with worktrees
- [Claude Code Agent Teams docs](https://code.claude.com/docs/en/agent-teams) - Official multi-agent documentation
- [Claude HUD](https://aitoolly.com/ai-news/article/2026-03-22-claude-hud-a-new-monitoring-plugin-for-claude-code-tracking-context-and-agent-activity) - Context and agent monitoring
- [AI Coding Agent Dashboard](https://blog.marcnuri.com/ai-coding-agent-dashboard) - Cross-device agent monitoring
- [tmux Advanced Use wiki](https://github.com/tmux/tmux/wiki/Advanced-Use) - Hooks, formats, popups, menus
- [tmux Formats wiki](https://github.com/tmux/tmux/wiki/Formats) - Format variables and conditionals
- [Datadog alert storm reduction](https://www.datadoghq.com/blog/reduce-alert-storms-datadog/) - Alert fatigue mitigation
- [Netdata alert fatigue guide](https://www.netdata.cloud/academy/what-is-alert-fatigue-and-how-to-prevent-it/) - Notification overload strategies
- [OSC 8 hyperlinks in tmux](https://github.com/tmux/tmux/issues/3660) - Clickable links status
- [Claude Code hooks guide](https://code.claude.com/docs/en/hooks-guide) - Official hooks documentation
- [Claude Code notifications blog](https://www.d12frosted.io/posts/2026-01-05-claude-code-notifications) - Community notification patterns
- [tmux popup usage](https://tmuxai.dev/tmux-popup/) - display-popup best practices
- [tmux-logging](https://github.com/tmux-plugins/tmux-logging) - Session logging and capture
- [Multi-agent orchestration patterns](https://www.codebridge.tech/articles/mastering-multi-agent-orchestration-coordination-is-the-new-scale-frontier) - Coordination strategies
