# Configuration

agenthive is configured by command-line flags and by files in a single directory. There is no daemon config file you have to edit by hand for most use cases.

## Config directory

Default: `$XDG_CONFIG_HOME/agenthive`, falling back to `~/.config/agenthive`.

Override per-command with `--config-dir`:

```bash
agenthive --config-dir /opt/agenthive start
```

Override per-shell with the environment variable:

```bash
export AGENTHIVE_CONFIG_DIR=/opt/agenthive
```

The directory is created with mode `0700` on first use.

## Files

```
~/.config/agenthive/
├── identity.key         # Ed25519 private key (mode 0600). NEVER commit this.
├── state.json           # Persisted CRDT state (peers, routes, config). Mode 0600.
├── agenthive.sock       # Unix-domain socket the hook subcommand and TUI use. Mode 0600.
└── actions/             # Action gate file queue
    ├── <id>.pending     # Pending PreToolUse request, written by the gate
    └── <id>.response    # Response, written atomically via O_CREAT|O_EXCL
```

`identity.key` and `state.json` are the only files you'd ever back up. `agenthive.sock` is recreated on daemon start; `actions/` is transient.

## Environment variables

| Variable | Effect |
|---|---|
| `AGENTHIVE_CONFIG_DIR` | Override the config directory |
| `GOLOG_LOG_LEVEL` | libp2p log verbosity: `error` (default), `info`, `debug` |
| `GOLOG_LOG_FMT` | `color` (default) or `json` |
| `CI` | When set, integration tests that require external state (real tmux, real mDNS) auto-skip |

## libp2p networking

agenthive listens on ephemeral ports by default. The `agenthive id` command prints exactly what multiaddrs the local Host is reachable on after AutoNAT and Identify have run.

In a future release, you'll be able to pin specific ports via `--listen` flags. For now: let it auto-allocate.

### Routers and UPnP

agenthive enables `NATPortMap` by default — it'll silently ask your home router for a port forward via UPnP / NAT-PMP. Most consumer routers cooperate. If yours doesn't, the request is a no-op and no error is raised; the relay path still works (see [[NAT Traversal]]).

## CRDT state

`state.json` holds three LWW-Maps: `peers`, `routes`, `config`. Each entry stamps with an HLC timestamp `{Wall, Counter, PeerID}`. Reading it directly is safe; writing is not — let the daemon do it through `peers add`, `routes add`, etc.

The daemon saves state on shutdown (clean SIGINT/SIGTERM) and periodically while running.

## Hook timeout

When you wire `agenthive hook PreToolUse` into Claude Code (see [[Claude Code Integration]]), the recommended hook-level timeout is **310 seconds**, ten seconds beyond agenthive's internal 300s default. This is so Claude doesn't kill the hook process before agenthive can return the gate response. Destructive actions (e.g., `rm -rf`) use a tighter 30s internal timeout but inherit the same outer 310s ceiling.

## TUI configuration

The TUI reads from the local daemon socket. No separate config required. Keybindings are documented in [[CLI Reference]] — keybinding remapping is on the roadmap (v0.2.0+).

## Logging

Local notifications go to a JSON-lines log file under the config directory unless you override the path. The log surface is the default fallback when no other surface is configured.

## Backup

Two-file backup covers everything:

```bash
tar -czf agenthive-backup.tar.gz \
  ~/.config/agenthive/identity.key \
  ~/.config/agenthive/state.json
```

Restoring to a new machine: untar to `~/.config/agenthive/`, start the daemon, and the mesh recognizes the same PeerID. `peers.json` propagates back through GossipSub from any reachable peer; you don't need to back it up to recover the peer list.

**If `identity.key` leaks**, the holder can impersonate your node. Remove the compromised PeerID from the CRDT peer set on a healthy peer (see [[Security Model]]) and generate a new identity with `agenthive init`.
