# CLI Reference

Every subcommand in agenthive v0.1.0.

## Global flags

| Flag | Default | Effect |
|---|---|---|
| `--config-dir <path>` | `$XDG_CONFIG_HOME/agenthive` or `~/.config/agenthive` | Override the directory holding `identity.key`, `state.json`, `agenthive.sock` |
| `--version` | — | Print version, source commit, and build date, then exit |
| `--help` | — | Print help for the current command |

---

## `agenthive init`

Generate a fresh Ed25519 keypair and persist the private key to `<config-dir>/identity.key` with mode `0600`. The directory is created with mode `0700` if missing.

```
agenthive init
```

Idempotent **only if no identity exists**. If `identity.key` is already present, `init` refuses to overwrite — delete it manually first if you really mean to rotate identity.

Exits 0 on success.

---

## `agenthive id`

Print every multiaddr the local Host is listening on, one per line. Each line includes the PeerID. Run this on a fresh node to share a multiaddr with peers.

```
agenthive id
```

Sample output:

```
/ip4/192.168.1.10/tcp/9123/p2p/12D3KooWAbCdEf...
/ip4/192.168.1.10/udp/9123/quic-v1/p2p/12D3KooWAbCdEf...
/ip6/2001:db8::5/tcp/9123/p2p/12D3KooWAbCdEf...
```

If a peer is behind NAT, only LAN multiaddrs may be useful here until AutoNAT and AutoRelay assign a relayed multiaddr.

---

## `agenthive peers`

Manage the CRDT peer set.

### `peers add <multiaddr>`

Parse a multiaddr, derive the PeerID from its `/p2p/` suffix, and add the entry to the local `StateStore.peers` map. The change propagates to every connected peer via GossipSub.

```
agenthive peers add /ip4/192.168.1.20/tcp/9123/p2p/12D3KooW...
```

### `peers list`

Print every peer known to the local StateStore, including `self`, online status, current dialed address, and last-seen timestamp.

```
agenthive peers list
```

### `peers del <peer-id>`

Tombstone a peer in the CRDT. The deletion propagates; receivers stop accepting GossipSub messages from that PeerID.

```
agenthive peers del 12D3KooWXyZ...
```

---

## `agenthive routes`

Manage routing rules. See [[Routing]] for the full selector grammar.

### `routes add <id> <selector> <targets>`

```
agenthive routes add critical-phone 'priority:critical' 'phone,laptop'
agenthive routes add codex-only 'source:codex-cli' 'desktop'
agenthive routes add default-route '*' 'log'
```

`<selector>` is a comma-separated list of `key:value` clauses (AND-joined). `<targets>` is comma-separated peer names or the literal string `ALL`.

### `routes list`

```
agenthive routes list
# ID              SELECTOR             TARGETS
# critical-phone  priority:critical    phone,laptop
# codex-only      source:codex-cli     desktop
```

### `routes del <id>`

Tombstone a route. Propagates via GossipSub.

---

## `agenthive start`

Run the daemon in the foreground. Blocks until ^C / SIGTERM. Loads identity, opens the libp2p Host, joins GossipSub, registers stream handlers, starts the Unix-socket server for hooks and the TUI, and connects to every peer in `StateStore.peers`.

```
agenthive start
```

The daemon is a single process. There is no detach / daemonize mode in v0.1.0 — use `systemd`, `tmux`, `screen`, or `nohup` if you want background operation.

---

## `agenthive hook <event>`

Invoked by Claude Code's hook system. Reads the hook JSON payload from stdin, dispatches via the local daemon's Unix socket, prints the hook response JSON to stdout.

```
agenthive hook PreToolUse < /tmp/event.json
```

Currently supported events: `PreToolUse`. Others are no-ops (exit 0, no output).

If the daemon is unreachable, prints nothing and exits 0 — Claude falls back to its built-in permission prompt. Never fail-closed.

See [[Claude Code Integration]] for the settings.json snippet.

---

## `agenthive respond <action-id> <decision>`

Manually write a response file for a pending action gate request. Useful while real surfaces (phone, Slack, etc.) are still being built.

```
agenthive respond a1b2c3d4 allow
agenthive respond a1b2c3d4 deny
```

Writes `<config-dir>/actions/<action-id>.response` atomically via `O_CREAT|O_EXCL`. If the file already exists (another surface beat you), exits 1 with `already responded`.

---

## `agenthive tui`

Launch the bubbletea TUI. Connects to the local daemon's Unix socket, queries for snapshot data, renders four tabs:

- **Peers** — live peer list with status icons
- **Routes** — current routing rules
- **Actions** — pending action gate requests
- **Logs** — recent event log

Keybindings:

| Key | Effect |
|---|---|
| `p` | Switch to Peers tab |
| `r` | Switch to Routes tab |
| `a` | Switch to Actions tab |
| `l` | Switch to Logs tab |
| `↑` / `k` | Cursor up |
| `↓` / `j` | Cursor down |
| `y` (Actions tab) | Approve the highlighted action |
| `n` (Actions tab) | Deny the highlighted action |
| `1` / `2` / `3` (Logs tab) | Filter by level (info / warn / crit) |
| `q`, `^C` | Quit |

If the daemon is not running, the TUI prints `agenthive daemon is not running. Start it with: agenthive start` and exits 1.

---

## `agenthive --version`

Print the version, source commit, and build date.

```
agenthive 0.1.0 (commit ce7fbc1db6c07fed6146c3d7ef0377a06d6180c6, built 2026-06-04T01:08:15Z)
```

`go install`-built binaries report `dev (commit none, built unknown)` since the ldflag vars aren't injected by that build path.

---

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Operation-specific error (e.g., already responded, daemon unreachable for TUI) |
| Non-zero from `hook` | Should not happen — hooks are deliberately fail-open and exit 0 even on internal error |
