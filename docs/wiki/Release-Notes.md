# Release Notes

Newest release on top. Each entry mirrors the GitHub release page; links go to the original PRs for full context.

---

## [v0.1.0](https://github.com/shaiknoorullah/agenthive/releases/tag/v0.1.0) — 2026-06-04

**First user-installable agenthive release.** Lands the surface layer on top of the libp2p substrate.

### Highlights

- TUI with bubbletea — peers / routes / actions / logs tabs, golden-file tested
- tmux per-pane surface + TPM-compatible plugin
- Desktop notifications (notify-send / osascript)
- Route matcher with selector grammar
- AutoRelay v2 peer source now fed by the CRDT peer set (closes the no-op closure gap from the libp2p RFC §10)
- GoReleaser-published binaries for `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`
- `agenthive routes add|list|del`, `agenthive tui` subcommands
- `--version` flag with stamped commit + build date

### Artifacts

| Asset | SHA-256 |
|---|---|
| `agenthive_0.1.0_linux_amd64.tar.gz` | `01e3eb687ac60b421b7d68d6cbe5834c1180193e8126e6e9cd655ef859948bfa` |
| `agenthive_0.1.0_linux_arm64.tar.gz` | `f66b1b305da00c228f47eeb6d38093bed132f92cc8d010f1b6bf5283972fe46e` |
| `agenthive_0.1.0_darwin_amd64.tar.gz` | `9f3120d9c714430e7eb8929f387789c190b0a153c9b965294d96f358bb62eaca` |
| `agenthive_0.1.0_darwin_arm64.tar.gz` | `8c5960429f4e91446336237b534695d2261499bc68557ef985a5645fb339e899` |

Each tarball includes the static binary, `LICENSE`, `README.md`, `SECURITY.md`, and the `tmux/` plugin directory.

### PRs

- [#14](https://github.com/shaiknoorullah/agenthive/pull/14) — v0.1.0 implementation
- [#13](https://github.com/shaiknoorullah/agenthive/pull/13) — README tagline correction (`self-hosted` → `peer-to-peer`)
- [#12](https://github.com/shaiknoorullah/agenthive/pull/12) — README / CONTRIBUTING / SECURITY refresh for the libp2p stack
- [#11](https://github.com/shaiknoorullah/agenthive/pull/11) — libp2p-based transport, daemon, hooks, dispatch

### Known limitations

- No Termux push surface yet; manual response via `agenthive respond` or the TUI
- No ntfy / Slack / Discord / audio surfaces
- No DND mode (workaround: delete `phone-*` routes during focus windows)
- Per-message GossipSub signature validation deferred to v0.2.0
- Windows binary not built — Unix socket IPC needs replacing first
- `go install` builds report `dev (commit none, built unknown)` — use the pre-built tarball for stamped binaries

### Upgrade notes

This is the first installable release; there's nothing to upgrade from.

### Installation

```bash
# Pre-built binary
curl -L "https://github.com/shaiknoorullah/agenthive/releases/download/v0.1.0/agenthive_0.1.0_linux_amd64.tar.gz" | tar -xzC /tmp
sudo install -m755 /tmp/agenthive /usr/local/bin/

# Or via go install
go install github.com/shaiknoorullah/agenthive/cmd/agenthive@v0.1.0
```

See [[Installation]] for all install methods.

---

## Pre-v0.1.0 commits (no tagged release)

The libp2p adoption itself shipped on `main` as [#11](https://github.com/shaiknoorullah/agenthive/pull/11). Before that, only the CRDT data layer and the original (now-superseded) implementation plans were on `main`.

Branches representing the original pre-libp2p design (`develop`, `feat/transport`, `feat/hooks`, `feat/dispatch`) were closed and deleted from the remote after libp2p adoption. PR [#9](https://github.com/shaiknoorullah/agenthive/pull/9) (feat/tui) remains open as **draft** — its bubbletea code informed the v0.1.0 TUI even though the data plumbing was rewritten against the new architecture.
