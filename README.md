# agenthive

A self-hosted, encrypted peer-to-peer mesh that connects your AI coding agents across devices. Get notifications, approve actions, and control agents from your terminal -- whether that's tmux on a server, your laptop, or Termux on your phone. No cloud. No intermediaries. Just your machines, talking directly.

---

Your AI agents run on a server at home. You're on the bus. Your phone buzzes:

> **Claude wants to run:** `rm -rf /tmp/build`
>
> **\[Allow\]** **\[Deny\]**

You tap Allow. Claude proceeds. No cloud service touched the message.

agenthive turns every terminal into a command center for your agents.

---

## How It Works

```
  SERVER (tmux)                LAPTOP (tmux)              PHONE (Termux)
  +-----------------+         +-----------------+        +------------------+
  | Claude Code     |         | Desktop notifs  |        | Android push     |
  | Codex CLI       |   SSH   | tmux status bar |  SSH   | termux-notif     |
  | Custom agents   |<=======>| Action popups   |<======>| Action buttons   |
  |                 | tunnel  | TUI dashboard   | tunnel | TUI dashboard    |
  +-----------------+         +-----------------+        +------------------+
        |                            |                          |
        +----------------------------+--------------------------+
                         Encrypted mesh (same daemon)
                         CRDT-synced config & routing
                         Manage from ANY device
```

Every device runs the same `agenthive` daemon. Every device is an equal peer. Change a routing rule on your phone -- the server learns instantly.

## Features

### Notifications That Reach You Anywhere

- **tmux status bar** -- native per-pane notifications, zero shell forks
- **Desktop** -- `notify-send` (Linux), `osascript` (macOS)
- **Android** -- native push via `termux-notification`, with action buttons
- **Audio** -- terminal bell, system sounds, or custom audio files

### Bidirectional Agent Control

- **Action buttons** -- Allow/Deny agent permission requests from any device
- **Remote commands** -- tell agents on remote servers what to do from your phone
- **Hook-native** -- uses Claude Code's `PreToolUse` hook for programmatic allow/deny (no keystroke injection)

### Intelligent Routing

```
agenthive routes add "project:api-server -> phone, laptop"
agenthive routes add "session:refactor -> telegram"
agenthive routes add "priority:critical -> ALL"
agenthive routes add "source:Codex -> desktop-only"
```

Route notifications per-agent, per-project, per-session, per-window, or per-pane. Rules sync across all peers automatically.

### Distributed Mesh Management

```
$ agenthive tui

+================================================================+
|  Peers                                                          |
|  * dev-server     online   12ms   5 agents   43 msgs today     |
|  * macbook-pro    online    3ms   2 agents   18 msgs today     |
|  * pixel-phone    online   45ms   0 agents    7 msgs today     |
|  o work-desktop   offline  --     last seen 2h ago              |
|                                                                 |
|  Routes                                                         |
|  api-server/*      -> phone, laptop                             |
|  session:refactor  -> telegram                                  |
|  priority:critical -> ALL                                       |
|  default           -> laptop                                    |
|                                                                 |
|  [p]eers  [r]outes  [m]etrics  [a]dd device  [q]uit            |
+=================================================================+
```

Manage peers, routes, and configuration from **any** connected device. The TUI works identically in tmux and Termux.

### Smart Local Notifications

Built on native tmux per-pane options (not files):

- **Atomic** -- no race conditions, no dual-file writes
- **Zero-fork** -- status line renders via tmux format strings, not shell commands
- **O(1) clearing** -- inline hook, no directory scanning
- **Auto-cleanup** -- pane destruction clears notifications automatically
- **Multi-agent** -- each pane gets its own notification slot
- **Worktree-aware** -- shows `project/worktree` for git worktrees

### Priority Levels

```
[14:30] Claude/api-server: Task failed        <- red, bold (critical)
[14:31] Claude/frontend: Agent has finished   <- default (info)
[14:32] Codex/docs: Needs approval            <- yellow (warning)
```

Critical notifications persist longer, route to more devices, and can trigger audio/desktop alerts.

### Agent State Tracking

```
* api-server   ? frontend   * docs-gen
```

Green = running, yellow = needs input, blue = done. Visible in status bar and dashboard.

### Notification Grouping

When 5 agents in the same project finish within seconds:

```
[14:30] Claude/my-project: 5 agents finished    <- instead of 5 separate notifications
```

Expand details in the picker or dashboard.

## Requirements

- tmux 3.2+ (for local notifications)
- [fzf](https://github.com/junegunn/fzf) (for notification picker)
- SSH keys configured (for mesh transport)
- Go 1.22+ (to build from source, or use prebuilt binaries)

### Optional

- [Termux](https://termux.dev) + [Termux:API](https://wiki.termux.com/wiki/Termux:API) (Android peer)
- [jq](https://jqlang.github.io/jq/) (enhanced JSON handling in hooks)

## Installation

### From Source

```bash
go install github.com/shaiknoorullah/agenthive@latest
```

### Prebuilt Binaries

Download from [Releases](https://github.com/shaiknoorullah/agenthive/releases) for:
- `linux/amd64`, `linux/arm64`
- `darwin/amd64`, `darwin/arm64` (macOS)
- `linux/arm64` (Termux/Android)

### Termux (Android)

```bash
pkg install openssh autossh jq
# Install agenthive binary for linux/arm64
# Install Termux:API from F-Droid for termux-notification
```

### tmux Plugin (TPM)

For local tmux notifications only (no mesh):

```tmux
set -g @plugin 'shaiknoorullah/agenthive'
```

## Quick Start

### 1. Initialize

```bash
# On every device:
agenthive init
```

Generates an Ed25519 peer identity. Run this on your server, laptop, and phone.

### 2. Pair Devices

```bash
# From your server, pair with your laptop:
agenthive pair --remote user@laptop

# From your laptop, pair with your phone (Termux):
agenthive pair --remote user@phone:8022
```

Or use QR code pairing for Termux:

```bash
# On laptop:
agenthive pair --qr

# On phone (scan the QR, or):
agenthive pair --manual
```

### 3. Establish Links

```bash
# Server -> Laptop (SSH tunnel):
agenthive link --to laptop --via ssh

# Server -> Phone (SSH tunnel to Termux):
agenthive link --to phone --via ssh

# LAN peers (direct, no SSH overhead):
agenthive link --to laptop --via tcp
```

### 4. Start the Daemon

```bash
# On every device:
agenthive start --daemon
```

### 5. Configure Claude Code Hooks

```json
{
  "hooks": {
    "PreToolUse": [{
      "hooks": [{
        "type": "command",
        "command": "agenthive hook PreToolUse"
      }]
    }],
    "Stop": [{
      "hooks": [{
        "type": "command",
        "command": "agenthive hook Stop"
      }]
    }],
    "Notification": [{
      "hooks": [{
        "type": "command",
        "command": "agenthive hook Notification"
      }]
    }]
  }
}
```

### 6. Set Up Routes

```bash
# All critical notifications go to every device:
agenthive routes add "priority:critical -> ALL"

# API project goes to phone and laptop:
agenthive routes add "project:api-server -> phone, laptop"

# Everything else just goes to laptop:
agenthive routes add "default -> laptop"
```

Done. Your agents are now connected to your mesh.

## Usage

### CLI

```bash
agenthive init                              # Generate peer identity
agenthive pair --remote user@host           # Pair with a peer via SSH
agenthive pair --qr                         # QR code pairing (Termux)

agenthive link --to <peer> --via ssh        # SSH tunnel link
agenthive link --to <peer> --via tcp        # Direct TCP link (LAN)

agenthive start [--daemon]                  # Start the mesh daemon
agenthive stop                              # Stop the daemon
agenthive status                            # Show peers, links, routes

agenthive peers                             # List peers with status/latency
agenthive routes                            # List routing rules
agenthive routes add "<selector> -> <targets>"
agenthive routes del <route-id>

agenthive respond allow:<request-id>        # Approve an agent action
agenthive respond deny:<request-id>         # Deny an agent action

agenthive tui                               # Interactive TUI dashboard

agenthive hook <event>                      # Claude Code hook handler
agenthive config get <key>                  # Read config (synced)
agenthive config set <key> <value>          # Set config (synced)
```

### tmux Keybindings

| Key | Action |
|-----|--------|
| `prefix + N` | Jump to oldest notification |
| `prefix + S` | Notification picker (fzf) |
| `prefix + D` | Dashboard popup |
| `prefix + Q` | Toggle Do Not Disturb |

All keybindings are configurable via tmux options.

### tmux Configuration

```tmux
# Keybindings
set -g @agenthive-key-next 'N'
set -g @agenthive-key-picker 'S'
set -g @agenthive-key-dashboard 'D'
set -g @agenthive-key-dnd 'Q'

# Display
set -g @agenthive-status-line 'on'
set -g @agenthive-status-bg 'default'
set -g @agenthive-fg '#c8d3f5'
set -g @agenthive-alert-fg 'yellow'
set -g @agenthive-critical-fg 'red'

# Behavior
set -g @agenthive-desktop-notifs 'on'
set -g @agenthive-sound 'bell'
set -g @agenthive-stale-timeout '1800'
```

## Architecture

### Layer Stack

```
6. Command        | Remote agent control (commit, run, query)
5. Management     | Distributed TUI: peers, routes, metrics, pairing
4. Routing        | CRDT-synced rules: per-agent/project/session/pane targeting
3. Actions        | Bidirectional allow/deny via PreToolUse hooks
2. Transport      | SSH tunnels (WAN) + TCP/Noise (LAN) + optional STUN
1. Integration    | tmux options, Termux notifs, notify-send, osascript
```

### Transport

- **WAN:** SSH reverse tunnels via autossh (zero new infrastructure)
- **LAN:** Direct TCP with Noise Protocol encryption
- **Optional:** STUN-assisted connections for SSH-blocked environments

### State Synchronization

Routing rules, peer registry, and configuration are distributed using **LWW-Register CRDTs** with Hybrid Logical Clocks. No leader election. No consensus protocol. All peers converge to the same state automatically.

### Notification Flow

```
Agent fires hook -> agenthive daemon -> evaluate routing rules
                                     -> forward to matching peers via SSH tunnel
                                     -> peer dispatches to local surface:
                                          tmux status bar (format strings)
                                          desktop notification (notify-send)
                                          Android notification (termux-notification)
                                          action buttons (allow/deny)
                                     -> response flows back through tunnel
                                     -> hook returns decision to agent
```

### Security

- **Transport:** SSH (AES-256-GCM) for WAN, Noise Protocol (ChaCha20-Poly1305) for LAN
- **Identity:** Ed25519 key pairs per peer
- **Authentication:** SSH key-based auth for tunnels, peer identity verification for direct links
- **Action security:** Cryptographic request IDs, TTL expiry, audit trail
- **No cloud:** All traffic stays between your machines

## Supported Agents

| Agent | Hook Integration | Action Buttons |
|-------|-----------------|----------------|
| Claude Code | PreToolUse, Stop, Notification hooks | Allow/Deny via hook JSON response |
| Codex CLI | notify callback | Notification only |
| Custom tools | Source the hook library or write to Unix socket | Full support |

## Comparison

| Feature | agenthive | tmux-notify | tmux-agent-indicator | tmux-agent-notifications |
|---------|-----------|-------------|---------------------|--------------------------|
| Multi-device mesh | Yes | No | No | No |
| Action buttons (allow/deny) | Yes | No | No | No |
| Remote agent control | Yes | No | No | No |
| Android (Termux) | Yes | No | No | No |
| Notification routing rules | Yes | No | No | No |
| Desktop notifications | Yes | Yes | No | No |
| Agent state tracking | Yes | No | Yes | No |
| Priority levels | Yes | No | No | No |
| Self-hosted (no cloud) | Yes | Yes | Yes | Yes |
| tmux status bar | Yes | No | Yes | Yes |
| Notification grouping | Yes | No | No | No |

## Project Status

> **Early Development** -- agenthive is under active development. The architecture is designed, the RFCs are written, and implementation is underway. Contributions and feedback are welcome.

See the [design documents](docs/rfcs/) for the full architectural rationale, including adversarial debates and judge evaluations for every major design decision.

## Contributing

Contributions are welcome. Please read the design documents in `docs/rfcs/` before proposing architectural changes -- every major decision has been debated and documented.

```bash
git clone https://github.com/shaiknoorullah/agenthive.git
cd agenthive
go build ./...
go test ./...
```

## License

MIT

## Acknowledgments

- Architecture informed by analysis of [tmux-agent-notifications](https://github.com/kaiiserni/tmux-agent-notifications), [tmux-notify](https://github.com/rickstaa/tmux-notify), [tmux-agent-indicator](https://github.com/accessd/tmux-agent-indicator), and [claude_code_agent_farm](https://github.com/Dicklesworthstone/claude_code_agent_farm)
- Transport design validated through adversarial debate between SSH tunnel and P2P/WebRTC approaches
- Local notification architecture validated through debate between file-based and native tmux approaches
