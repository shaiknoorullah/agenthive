# Quick Start

A two-peer agenthive mesh in under five minutes. We'll do a laptop ↔ dev-server pairing; the same flow works for any combination of devices including a phone in Termux.

## Prerequisites

- agenthive on both devices ([[Installation]])
- TCP reachability between them (LAN, VPN, or one peer with a public address)

## On device A (your laptop)

```bash
# 1. Generate an identity
agenthive init
# Wrote ~/.config/agenthive/identity.key (mode 0600)
# PeerID: 12D3KooWAbCdEf...

# 2. Print this peer's multiaddrs — copy the line you'll use to dial
agenthive id
# /ip4/192.168.1.10/tcp/9123/p2p/12D3KooWAbCdEf...
# /ip4/192.168.1.10/udp/9123/quic-v1/p2p/12D3KooWAbCdEf...
# /ip6/.../tcp/9123/p2p/12D3KooWAbCdEf...

# 3. Start the daemon — blocks; ^C to stop
agenthive start
```

Leave that terminal running.

## On device B (your dev server)

```bash
# 1. Generate an identity
agenthive init

# 2. Add device A as a known peer (paste any multiaddr from device A's `agenthive id`)
agenthive peers add /ip4/192.168.1.10/tcp/9123/p2p/12D3KooWAbCdEf...
# peer 12D3KooWAbCdEf... added at <HLC timestamp>

# 3. Start the daemon
agenthive start
```

The two daemons connect via libp2p, negotiate Noise XX, subscribe to GossipSub topic `/agenthive/state/v1`, and start exchanging CRDT state.

If both devices are on the same LAN, you can skip step 2 entirely — mDNS finds and adds peers automatically.

## Verify it's working

In a third terminal on either device:

```bash
agenthive peers list
# 12D3KooWAbCdEf...  online   192.168.1.10:9123  (TCP+QUIC)  last_seen 2s ago
# 12D3KooWXyZ...     self
```

Then mutate something on one side — add a route, for instance:

```bash
# On device A
agenthive routes add critical-all 'priority:critical' 'ALL'

# On device B (within a round trip)
agenthive routes list
# critical-all   priority:critical   ALL
```

State converges across the mesh under one round trip.

## Run the TUI

On either device:

```bash
agenthive tui
```

You'll see four tabs — Peers, Routes, Actions, Logs — driven by live CRDT state. Press `p / r / a / l` to switch tabs, `q` or `^C` to quit. See [[CLI Reference]] for the full key map.

## Add a third device

Repeat the device-B steps anywhere. New peer entries propagate through GossipSub to all members; no need to add every peer to every other peer. The mesh grows organically.

## What's next

→ [[Claude Code Integration]] — wire `PreToolUse` to route Claude's permission prompts through the mesh
→ [[tmux Plugin]] — install the per-pane status-line surface
→ [[Routing]] — selector grammar, examples, conflict resolution
→ [[Configuration]] — config directory layout, env vars
