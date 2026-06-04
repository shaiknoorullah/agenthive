# agenthive Wiki

**Encrypted, peer-to-peer mesh for AI agent notification and control.**

agenthive turns every device you own — dev server, laptop, phone in Termux — into a coordination node for your AI coding agents. Claude Code's `PreToolUse` hook routes through your own libp2p mesh to whichever device is in your hand. First responder wins, atomic file-create as the race primitive. No cloud, no broker, no server to deploy.

---

## Start here

| You want to… | Page |
|---|---|
| Install the binary | [[Installation]] |
| Get two peers talking in 5 minutes | [[Quick Start]] |
| Wire it into Claude Code | [[Claude Code Integration]] |
| Set up the tmux plugin | [[tmux Plugin]] |
| Look up a CLI subcommand | [[CLI Reference]] |
| Understand the architecture | [[Architecture]] |
| Configure routing | [[Routing]] |
| Diagnose a problem | [[Troubleshooting]] |

## Reference

- [[Configuration]] — config directory layout, env vars, file formats
- [[Security Model]] — what's encrypted, what's authenticated, what's exposed
- [[Design Decisions]] — index of RFCs and the adversarial debates behind them
- [[FAQ]]
- [[Release Notes]]

## Status

| Subsystem | Status |
|---|---|
| libp2p Host: TCP + QUIC, Noise XX, Yamux, IPv4 + IPv6 | shipped (v0.1.0) |
| NAT traversal: DCUtR, AutoRelay, UPnP / NAT-PMP | shipped |
| Embedded Circuit Relay v2 on every node | shipped |
| mDNS LAN discovery | shipped |
| CRDT state sync via GossipSub | shipped |
| Action gate via Claude Code `PreToolUse` hook | shipped |
| Bubbletea TUI: peers / routes / actions / logs | shipped |
| tmux per-pane surface + TPM plugin | shipped |
| Desktop notifications (notify-send / osascript) | shipped |
| CRDT-driven AutoRelay peer source | shipped |
| Termux push surface | planned (v0.2.0) |
| ntfy / Slack / Discord / audio surfaces | planned |
| Per-message GossipSub signature validation | planned |
| Brew / Scoop / AUR packaging | planned |

Current release: **[v0.1.0](https://github.com/shaiknoorullah/agenthive/releases/tag/v0.1.0)** — pre-built binaries for `linux/{amd64,arm64}` and `darwin/{amd64,arm64}`.
