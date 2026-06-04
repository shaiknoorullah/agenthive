# Troubleshooting

Common problems and how to diagnose them.

## Daemon won't start

### `agenthive start` exits immediately with "identity not found"

You haven't run `agenthive init` yet. Run it once per device.

### "permission denied" reading `identity.key`

The file is mode 0600 and owned by the user that ran `agenthive init`. Make sure you're running the daemon as the same user, or `chmod` and `chown` it appropriately.

### "address already in use"

Another agenthive instance is still running, or another process is bound to one of libp2p's ephemeral ports. Find it:

```bash
ps aux | grep agenthive
ss -ltnp 2>/dev/null | head
```

Kill the stale process and retry. The PID file at `<config-dir>/agenthive.pid` should help.

### "socket file already exists"

A previous daemon crashed without cleaning up `<config-dir>/agenthive.sock`. Safe to delete:

```bash
rm ~/.config/agenthive/agenthive.sock
agenthive start
```

## Peers can't connect

### `agenthive peers list` shows the peer but `status` is `offline`

Run:

```bash
GOLOG_LOG_LEVEL=debug agenthive start 2>&1 | head -100
```

Look for lines like `dial attempt failed:` to see which path libp2p tried and what error was returned. Typical causes:

- The multiaddr you added points to a stale IP. The peer's actual address changed (DHCP, mobile). Have the peer run `agenthive id` again and re-add with the current multiaddr.
- The peer is behind a firewall blocking all inbound. AutoRelay should kick in; check that at least one of your peers has a publicly reachable address.
- IPv6-only addresses but one peer has no IPv6. Add an IPv4 multiaddr explicitly.

### mDNS doesn't auto-discover same-LAN peers

mDNS requires UDP 5353 multicast. Things that block it:

- Some VPN clients (WireGuard, Tailscale) capture multicast traffic
- Some Docker networking modes
- Some routers with "client isolation" on the wifi network

Try without the VPN, or fall back to manual `agenthive peers add` with an explicit multiaddr.

### "no route to host" between a phone (Termux) and your home server

Cellular carriers often NAT outbound. Your home server is behind your router's NAT. Neither can dial the other directly.

Solutions:
- Use a peer with a public address (cloud VPS) as a relay
- IPv6 on both ends (cellular often gives a public IPv6)
- Set up port forward on the home router (UPnP usually handles this automatically)

## Action gate doesn't fire

### Claude Code's PreToolUse runs but `agenthive tui` → Actions shows nothing

Check the hook command resolves correctly:

```bash
which agenthive
```

Use the absolute path in `~/.claude/settings.json` if needed:

```json
{ "command": "/usr/local/bin/agenthive hook PreToolUse" }
```

Check the daemon's socket is reachable:

```bash
ls -l ~/.config/agenthive/agenthive.sock
# should be a socket (s) with mode 0600, owned by you
```

Try a manual hook invocation:

```bash
echo '{"hook_event":"PreToolUse","tool":"Bash","input":"ls","session_id":"x","tool_use_id":"y"}' \
  | agenthive hook PreToolUse
```

If this hangs (waiting for a response), the action gate is fine; you just need to respond. Use `agenthive tui` or `agenthive respond` on the action ID.

### Hook prints nothing and exits 0 immediately

That's the fail-open path. Either the daemon socket is missing, or the gate timed out. Run with the daemon stdout visible (`agenthive start` in a separate terminal) and look for the corresponding action_request log line.

## TUI shows nothing or "daemon not running"

The TUI connects to `<config-dir>/agenthive.sock`. Confirm:

```bash
ls -l ~/.config/agenthive/agenthive.sock
agenthive start    # in another terminal
agenthive tui      # in this terminal
```

If the socket exists but the TUI exits anyway, check that the daemon is the same version. Mixed-version socket protocol mismatches are possible across major versions. Either upgrade or downgrade consistently.

## CRDT state doesn't converge

### Adding a route on peer A doesn't appear on peer B

1. Confirm both peers are connected (`agenthive peers list` on both shows the other as `online`).
2. Confirm both peers are on the same GossipSub topic (they will be — there's only one topic, `/agenthive/state/v1`).
3. Check the GossipSub validator. If peer B doesn't have peer A in its allow-list, B's validator drops A's messages. This shouldn't happen if you `peers add`-ed both directions, but worth checking with `agenthive peers list` on B.

### `routes list` shows a route that I deleted

Deletes are tombstones. They propagate exactly like writes. If `peers list` on the remote side shows a peer that didn't see the delete (because they were offline), they'll catch up via `PeerAnnounce` on reconnect. If you're seeing this on the same node that issued the delete, something is wrong — file an issue with a minimal reproduction.

## Releases / install

### `go install` fails with "module not found"

The module path is `github.com/shaiknoorullah/agenthive`. The binary path is `github.com/shaiknoorullah/agenthive/cmd/agenthive`. Note the `cmd/agenthive` suffix:

```bash
go install github.com/shaiknoorullah/agenthive/cmd/agenthive@v0.1.0
```

### Checksum mismatch on the release tarball

Re-download both the tarball and `checksums.txt`. CDN edge corruption is rare but possible. If it persists, file an issue.

### `agenthive --version` shows `dev (commit none, built unknown)` after `go install`

Expected. `go install` doesn't pass the ldflags GoReleaser uses to inject version metadata. Use the pre-built tarball if you need a stamped binary.

## Performance / battery

### Phone (Termux) battery draining quickly

agenthive maintains a long-lived libp2p connection. UDP keepalive is ~30s by default for QUIC. To minimize wakeups:

- Use `pkg install termux-tools` and enable `termux-wake-lock` to keep the process alive during screen-off (preferred over fighting Doze mode).
- Schedule `agenthive start` only during work hours via Termux's `cronie` package if you don't need 24/7 connectivity.

A native Android service for push-while-doze is on the roadmap. v0.1.0 is honest about not handling extended Doze well.

### High CPU usage

Check `GOLOG_LOG_LEVEL`. `debug` is verbose. Set it to `info` or unset.

```bash
unset GOLOG_LOG_LEVEL
```

Profile with `go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30` (the daemon doesn't expose pprof endpoints by default in v0.1.0 — patch + rebuild if you need to investigate).

## Still stuck?

- Check the daemon log file (default: `<config-dir>/events.log` or wherever your log surface is configured)
- Run with `GOLOG_LOG_LEVEL=debug`
- Open a GitHub issue with the steps to reproduce + the debug output
