# Roadmap

The path from v0.1.0 (shipped) to whatever 1.0 looks like. Open items are tracked as GitHub issues.

## Shipped — v0.1.0

- libp2p Host: TCP + QUIC, Noise XX, Yamux, IPv4 + IPv6
- DCUtR hole-punching + AutoRelay v2 with CRDT-driven peer source
- Embedded Circuit Relay v2 on every node
- mDNS LAN discovery
- UPnP / NAT-PMP
- CRDT state sync via GossipSub on `/agenthive/state/v1`
- Action gate (atomic file-queue, first-response-wins)
- Destructive-action classifier with 30s TTL
- Bubbletea TUI with 4 tabs
- tmux per-pane surface + TPM-compatible plugin
- Desktop notifications (notify-send / osascript)
- Route matcher with selector grammar
- Cobra CLI: `init`, `id`, `peers`, `routes`, `start`, `hook`, `respond`, `tui`
- GoReleaser-published binaries for `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`

## v0.2.0 — surface diversification

- **Termux push surface** — native Android notification with action buttons via `termux-notification`
- **ntfy.sh surface** — point at your self-hosted ntfy instance (or theirs, your call)
- **Slack surface** — interactive message via Bolt + Socket Mode
- **Discord surface** — button interaction via Gateway WebSocket
- **Audio surface** — terminal bell, system sounds, custom audio files
- **Codex CLI parity** — same hook contract, smaller payload
- **DND mode** — daemon flag suppresses all surfaces except logging until cleared
- **Notification grouping** — collapse multiple same-project events into one entry
- **Per-message GossipSub signature validation** — limit compromise blast radius to the compromised peer's key
- **Bounded tombstone GC** — once a generation is observed by every active peer, sweep
- **Periodic action janitor** — sweep expired pending files on a timer, not on demand
- **TUI keybinding remapping** — `@agenthive-key-*` style options
- **Windows binary** — replace Unix socket IPC with TCP loopback or named pipes

## v0.3.0 — operations and packaging

- **Brew tap, Scoop bucket, AUR PKGBUILD** — package-manager native installs
- **Docker image** — `FROM scratch` static binary
- **`systemd` unit and `launchd` plist templates** — first-class background-service install
- **`agenthive --version` machine-readable** — `--version --json` for CI
- **Identity-key encryption at rest** — OS keychain (macOS), Linux Secret Service, Termux:API
- **Native Android background service** — survives Doze without `termux-wake-lock` hacks
- **Pprof endpoints behind a flag** — production debugging
- **Structured event log** — JSON lines with stable schema
- **Listen address pinning** — `--listen` flag instead of ephemeral ports
- **Bootstrap peers** — optional static peer list separate from CRDT (useful for first-ever connection)

## v0.4.0+ — depth and resilience

- **Encrypted state file** — `state.json` at rest
- **Per-action authorization** — destructive actions require approval from a specific device tier
- **Multi-mesh** — one binary, multiple parallel meshes with isolated identity/state
- **Audit log** — append-only signed record of action gate decisions
- **Metrics endpoint** — Prometheus-style scrape for fleet telemetry
- **Formal security audit** — by an external firm
- **WireGuard transport** — for organizations that already terminate WireGuard
- **Yggdrasil transport** — alternative substrate for users who prefer IPv6 overlay routing
- **PWA push surface** — browser as the surface (Web Push API)

## Out of scope, probably forever

- **Cloud-hosted mode** — agenthive is peer-to-peer by design. If you want a hosted service, that's a different product.
- **Multi-user / multi-tenant** — agenthive is single-user. One person, their devices, their agents.
- **Rich-content notifications** — agent notifications are short text + metadata. Markdown/images/attachments aren't planned.
- **Persistent message store** — GossipSub + CRDT delta exchange covers our use case; a queue per peer is unnecessary state.

## How priorities get set

Open issues that gather thumbs-up rise. Pain points reported by users with reproductions rise. Things that compound (better tests, cleaner abstractions) get done between feature pushes. There is no fixed release cadence.

## Want to push a specific item up?

Open an issue, describe the use case, link any related work. Or open a PR with a working implementation — that's the fastest path.
