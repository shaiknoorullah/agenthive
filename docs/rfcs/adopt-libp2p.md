# RFC: Adopt go-libp2p as agenthive's transport, identity, and discovery layer

**Status:** Accepted
**Date:** 2026-06-04
**Supersedes:**
- `docs/superpowers/plans/2026-03-26-transport.md` (entirety)
- `internal/identity/` section of `docs/superpowers/plans/2026-03-26-core-daemon.md`
- `internal/protocol/messages.go` envelope/framing portions of the same
- The `internal/daemon/router.go` link-management portions of the same
- The "transport" verdict in `docs/rfcs/debate-transport-judgment.md`

**Untouched:**
- `internal/crdt/` (shipped, correct, transport-agnostic)
- `docs/superpowers/plans/2026-03-26-dispatch.md`
- `docs/superpowers/plans/2026-03-26-hooks.md`
- `docs/superpowers/plans/2026-03-26-tui.md`

---

## 1. Decision

agenthive adopts **go-libp2p** as its single transport-and-membership layer. The hand-rolled SSH-tunnel transport, the autossh-subprocess link, the bespoke pairing ceremony, the bespoke `LinkManager`, the bespoke message-envelope framing, and the bespoke peer identity module are **deleted from the plan before they are built**. They never reach `main`.

Concretely, the agenthive daemon is a libp2p **Host** configured with:

| Subsystem | Module / option | Role |
|---|---|---|
| Transports | `go-libp2p/p2p/transport/tcp`, `go-libp2p/p2p/transport/quic` | TCP and QUIC, both v4 and v6 |
| Security | `go-libp2p/p2p/security/noise` (XX) | Mutual auth + encryption |
| Stream multiplexing | Yamux on TCP; native on QUIC | Independent streams per request |
| Identity | Ed25519, `PeerID = multihash(pubkey)` | No CA, no fingerprint dance |
| LAN discovery | `go-libp2p/p2p/discovery/mdns` | Zero-config same-LAN find |
| NAT detection | `EnableNATService` + AutoNAT v2 | Tell each peer whether it's reachable |
| Hole punching | `EnableHolePunching` (DCUtR) | Direct dial through symmetric NAT |
| Router port mapping | `NATPortMap` (UPnP / NAT-PMP) | Optional, silent no-op on hostile routers |
| Reachability fallback | `EnableRelayService` + `EnableAutoRelayWithPeerSource` (Circuit Relay v2) | Every agenthive node IS a relay; nobody self-hosts a relay |
| State diffusion | `go-libp2p-pubsub` (GossipSub v1.2) on topic `/agenthive/state/v1` | CRDT delta broadcast |
| Directed messages | libp2p streams per registered protocol ID | Action requests, action responses, peer announce |

That is the complete transport stack. The Go file count for `internal/transport/` drops from the eight files in the original plan to one (`host.go`) plus protocol handlers in a sibling package.

## 2. The non-negotiable constraint

**agenthive does not depend on anything that is not agenthive itself.** Explicitly forbidden:

- No third-party libp2p public bootstrap nodes (Protocol Labs, IPFS, Filecoin, etc.)
- No third-party libp2p public Circuit Relays
- No third-party rendezvous, signaling, or DHT seed
- No public STUN, no public TURN
- No coordination server, no control plane (this rules out Tailscale-the-product and rules in Headscale only if the user explicitly opts in — but agenthive does not assume it)
- No requirement on sshd, tmux, Tailscale, Yggdrasil, NATS, MQTT, or any other technology being installed on the machine

The only external dependencies tolerated are the host OS, its network stack, and (optionally, queried but never required) UPnP/NAT-PMP on the user's home router.

How this constraint is satisfied: **every agenthive node runs `EnableRelayService()`**. Whichever of your own peers happens to have a publicly reachable address — typically a dev server with a public IP, a cloud VPS, an IPv6-enabled cellular handset, or a home machine with a forwarded port — automatically serves as the relay for the other peers' hole-punch coordination and as the data-path fallback when DCUtR can't punch. You are not "running a relay." You are running agenthive, which happens to embed a relay protocol that activates when reachable.

If literally no peer in your mesh has any reachable address — every device behind hostile NAT, no IPv6 anywhere, UPnP disabled, no shared LAN — peers cannot connect over the WAN. mDNS still finds them on the LAN. This degradation mode is honest and documented; it is not a workaround we owe the user.

## 3. Why go-libp2p (in eight words)

Identity-as-pubkey, GossipSub, Noise XX, DCUtR, embedded relay.

The longer form: every architectural problem the agenthive transport plan was about to solve from scratch — peer identity, pairing trust, multiplexing, encryption framing, message routing across multiple peers, NAT traversal, intermittent reconnection, mesh discovery — is solved inside libp2p by interlocking protocols that have been deployed in IPFS (since 2015), Filecoin (since 2020), Ethereum's consensus layer (since the Merge, 2022), Optimism, and the public IPFS gossipsub mesh which moves billions of messages per day. The 2025 ProbeLab measurement campaign across 4.4M hole-punch attempts pinned DCUtR's direct-success rate at 70% ± 7%; with Circuit Relay v2 absorbing the rest, end-to-end reachability is effectively 100% conditional on at least one publicly-reachable peer existing.

## 4. How agenthive uses each piece

### 4.1 The Host

On startup the daemon constructs exactly one `host.Host`:

```go
priv := loadOrGenerateIdentity()    // Ed25519, persisted to ~/.config/agenthive/identity.key, 0600
peers := loadPeerstoreFromState()   // from internal/crdt/StateStore.peers

h, err := libp2p.New(
    libp2p.Identity(priv),
    libp2p.UserAgent("agenthive/" + version),

    libp2p.ListenAddrStrings(
        "/ip4/0.0.0.0/tcp/0",
        "/ip4/0.0.0.0/udp/0/quic-v1",
        "/ip6/::/tcp/0",
        "/ip6/::/udp/0/quic-v1",
    ),

    libp2p.Security(noise.ID, noise.New),
    libp2p.Transport(tcp.NewTCPTransport),
    libp2p.Transport(libp2pquic.NewTransport),
    libp2p.DefaultMuxers,                       // Yamux on TCP

    libp2p.EnableNATService(),                  // help others detect their NAT
    libp2p.NATPortMap(),                        // ask the home router for a port
    libp2p.EnableHolePunching(),                // DCUtR
    libp2p.EnableRelayService(),                // BE a relay if reachable
    libp2p.EnableAutoRelayWithPeerSource(       // USE a relay (one of our own peers)
        peerSourceFromStateStore(store),
    ),

    libp2p.ConnectionManager(connMgr),
    libp2p.ResourceManager(resMgr),
)
```

The whole thing is one function call. There is no `Link` interface, no `LinkManager`, no pairing module, no Noise XX implementation we wrote, no SSH subprocess wrapper. They were never built.

### 4.2 Identity replaces the planned `internal/identity/`

The planned package (`Ed25519 keypair generation, peer ID, public key fingerprint, key rotation`) collapses to four lines:

```go
priv, _, _ := crypto.GenerateEd25519Key(rand.Reader)
b, _ := crypto.MarshalPrivateKey(priv)
_ = os.WriteFile("identity.key", b, 0600)
// PeerID is derived: peer.IDFromPublicKey(priv.GetPublic())
```

PeerIDs are `12D3KooW…` strings — multihash of the marshaled public key. They are deterministic from the keypair; there is no allocation step, no namespace conflict, no fingerprint UX to design. Two peers cannot have the same PeerID unless they share a private key.

The planned `internal/identity/` package is deleted from the core-daemon plan.

### 4.3 Pairing — one command, no ceremony

The planned multi-step pairing ceremony (`docs/superpowers/plans/2026-03-26-transport.md` task 13–14) is deleted. Pairing in libp2p is exchanging multiaddrs:

```
$ agenthive id
/ip4/203.0.113.5/udp/9001/quic-v1/p2p/12D3KooWAbCdEf…
/ip4/203.0.113.5/tcp/9001/p2p/12D3KooWAbCdEf…
/ip6/2001:db8::5/udp/9001/quic-v1/p2p/12D3KooWAbCdEf…
```

On any other peer:

```
$ agenthive peers add /ip4/203.0.113.5/udp/9001/quic-v1/p2p/12D3KooWAbCdEf…
peer 12D3KooWAbCdEf… added (LWW timestamp 2026-06-04T10:14:22.183…/0/laptop)
```

The new peer entry propagates through the CRDT gossip (already shipped, untouched) to every other agenthive node within a single GossipSub round trip. Once one peer knows the new peer, all peers know it.

Wrong PeerID → Noise XX handshake fails (the dialed peer cannot prove the private key for the embedded PeerID). There is no "have you trusted this fingerprint?" prompt because there is no fingerprint — the address *is* the identity.

### 4.4 The four registered protocols

```go
const (
    ProtoActionRequest  = "/agenthive/action/request/1"
    ProtoActionResponse = "/agenthive/action/response/1"
    ProtoNotification   = "/agenthive/notification/1"
    ProtoPeerAnnounce   = "/agenthive/peer/announce/1"
)

h.SetStreamHandler(ProtoActionRequest,  handleActionRequest)
h.SetStreamHandler(ProtoActionResponse, handleActionResponse)
h.SetStreamHandler(ProtoNotification,   handleNotification)
h.SetStreamHandler(ProtoPeerAnnounce,   handlePeerAnnounce)
```

Each handler reads a length-prefixed protobuf or CBOR message from the stream, applies it to the daemon state, and either writes a response or closes. Streams are independent — an action response on one stream cannot stall a notification on another. This replaces the planned `MsgNotification`, `MsgActionRequest`, `MsgActionResponse`, `MsgConfigSync`, `MsgHeartbeat`, `MsgPeerAnnounce` envelope-and-router system. There is no envelope: the protocol ID *is* the message type.

### 4.5 CRDT state diffusion via GossipSub

The CRDT layer is already shipped and is the right abstraction. What changes is how deltas move:

```go
ps, _ := pubsub.NewGossipSub(ctx, h)
topic, _ := ps.Join("/agenthive/state/v1")
sub, _ := topic.Subscribe()

// publish on every local mutation (debounced 100ms)
go publishDeltas(ctx, store, topic, hlc)

// receive and merge
go func() {
    for {
        msg, err := sub.Next(ctx)
        if err != nil { return }
        if msg.GetFrom() == h.ID() { continue }   // skip own messages
        var delta crdt.Delta
        _ = delta.UnmarshalCBOR(msg.Data)
        store.MergeMaps(delta.Peers, delta.Routes, delta.Config)
    }
}()
```

The planned disk-backed offline queue (`docs/superpowers/plans/2026-03-26-core-daemon.md` tasks 8–9) is deleted. GossipSub already handles message deduplication and partial-mesh fan-out; the CRDT semantics already guarantee that out-of-order, replayed, or duplicate deltas converge to the same state. When a peer reconnects after being offline, it requests a full snapshot via `ProtoPeerAnnounce` rather than replaying a queue.

### 4.6 mDNS for LAN

```go
mdns.NewMdnsService(h, "agenthive", &mdnsNotifee{h: h, store: store})
```

Same-LAN agenthive nodes find each other in <500ms with zero config. New peers discovered via mDNS are added to the StateStore with an LWW timestamp; they propagate to other peers via GossipSub the moment any single peer can reach the wider mesh.

### 4.7 NAT traversal — five paths, fail-soft

When daemon A wants to talk to daemon B, libp2p attempts these in parallel and races them:

1. **Direct LAN dial** if mDNS discovered B locally
2. **Direct WAN dial** if B advertises a public address in its multiaddrs (via AutoNAT-confirmed self-reported address, propagated through the CRDT)
3. **UPnP-mapped port dial** if B sits behind a cooperative home router
4. **DCUtR hole-punch** if both A and B can reach a common relay (one of the user's own publicly-reachable agenthive peers)
5. **Relayed dial** through the same common relay, as fallback

First success wins. The relay used in steps 4 and 5 is *not external infrastructure*. It is whichever peer in the user's mesh happens to have a public address — agenthive's `EnableRelayService()` makes every node a candidate. If the user has any of {a dev server, a cloud VPS, an IPv6-enabled phone, a home box with port-forward}, the relay path exists. If none of those, the user gets LAN-only operation.

## 5. What gets deleted (aggressively)

These plan components are removed before construction:

| Plan file / section | Action | Reason |
|---|---|---|
| `docs/superpowers/plans/2026-03-26-transport.md` Tasks 1–2 (Link interface, Envelope) | DELETE | libp2p stream + protocol ID replaces both |
| Same, Tasks 3–4 (PipeLink) | DELETE | libp2p has its own test harnesses (`p2p/net/mock`) |
| Same, Tasks 5–6 (Noise XX from scratch) | DELETE | `go-libp2p/p2p/security/noise` is the same protocol, audited and battle-tested |
| Same, Tasks 7–8 (TCP link) | DELETE | `p2p/transport/tcp` does this |
| Same, Tasks 9–10 (SSH link, autossh subprocess) | DELETE | We are not shelling out to ssh |
| Same, Tasks 11–12 (LinkManager multiplexing) | DELETE | libp2p `Host` *is* the link manager |
| Same, Tasks 13–14 (pairing ceremony) | DELETE | Multiaddr exchange replaces it |
| `docs/superpowers/plans/2026-03-26-core-daemon.md` Tasks 2–3 (Protocol messages) | REWRITE | One protobuf per protocol ID, not one envelope-with-type-tag |
| Same, Tasks 4–5 (Ed25519 identity) | DELETE | `crypto.GenerateEd25519Key` + persist; libp2p owns the rest |
| Same, Tasks 6–7 (Message router) | REWRITE as 1 file | `Router.Route(msg)` becomes `h.NewStream(ctx, peerID, protocolID)` |
| Same, Tasks 8–9 (Disk-backed offline queue) | DELETE | GossipSub + CRDT subsume it |
| `docs/rfcs/debate-transport-judgment.md` §7 "Custom SSH + gossip" verdict | SUPERSEDE | Reopened and overturned by `docs/rfcs/debate-libp2p-advocate.md` |

Net code count for the transport/identity/protocol/routing layer: the plan was ~25 files across four packages. After this RFC: 1 file for the Host (`internal/transport/host.go`), 4 files for the protocol handlers (`internal/protocols/{action,notification,peer,state}.go`), 1 file for mDNS notifee (`internal/discovery/mdns.go`). Six files, roughly 600 lines, replacing the ~3000 lines the plan budgeted.

## 6. What survives

- **`internal/crdt/`** is untouched. The CRDT is the payload of GossipSub messages and the state model for the daemon. It does not care what wire it rides.
- **The dispatch plan** (`docs/superpowers/plans/2026-03-26-dispatch.md`) is unchanged. Surfaces (tmux, desktop, Termux, audio) sit above the daemon and have no transport opinion.
- **The hooks plan** (`docs/superpowers/plans/2026-03-26-hooks.md`) is unchanged in shape. The action gate writes a `.pending` file, dispatches an `ProtoActionRequest` stream to target peers, blocks until any peer writes `.response`, returns the Claude Code hook JSON. The libp2p change is invisible to the gate logic; only the dispatcher swaps wire calls.
- **The TUI plan** (`docs/superpowers/plans/2026-03-26-tui.md`) is unchanged. It reads the StateStore and submits mutations through the local daemon — no transport awareness at all.

## 7. The threat model after this change

What the user trusts:
- The `~/.config/agenthive/identity.key` file (Ed25519 private key, mode 0600)
- The multiaddrs they've added to their peer list (these are their other devices)
- The Noise XX handshake (mutual auth — the dialing peer proves the PeerID it claims; the accepting peer proves its own)

What an attacker cannot do without one of the above secrets:
- Impersonate a peer (handshake requires the private key matching the PeerID in the multiaddr)
- Join the GossipSub topic invisibly (publication on `/agenthive/state/v1` is open by default; we restrict via a `pubsub.WithMessageSigning` + a signed peer allow-list message that GossipSub validates before forwarding)
- Inject CRDT state (deltas signed by the originating peer's libp2p key; non-allow-list peers get their messages dropped at the GossipSub validator)
- Read traffic in transit (Noise XX encrypts all bytes including stream framing)

What an attacker *can* do without secrets:
- See that two PeerIDs are exchanging encrypted bytes between two IPs (libp2p does not hide that; no onion routing)
- DoS a peer by exhausting its connection budget (the `ResourceManager` caps inbound; eviction is graceful)
- Spam GossipSub from an allow-listed-but-compromised peer (we mitigate with per-peer rate limits in the validator)

If the identity.key file is exfiltrated, the attacker becomes that peer. The mitigation is the user revokes the PeerID by removing it from the CRDT peer set, which propagates and causes other peers' GossipSub validators to reject subsequent messages from that key.

## 8. Failure modes the RFC accepts

We are honest about these. They are consequences of the no-external-dependency constraint.

1. **All-NATed mesh with no IPv6, no UPnP, no shared LAN.** WAN paths fail. mDNS still works on a LAN. We do not promise WAN connectivity in this case.
2. **DCUtR hole-punch fails ~30% of the time** per ProbeLab measurements. Fallback is the embedded Circuit Relay v2 on any peer with a public address. If no such peer exists, see (1).
3. **Phone in Doze for 8 hours.** libp2p does not solve this; no transport solves this. Android kills our process. Same problem all four prior advocates faced. The plan is: when the phone wakes (user opens an app, FCM doorbell arrives), agenthive reconnects within ~2s via QUIC 0-RTT-style fast reopen (libp2p reuses the Noise session if both sides still hold state).
4. **Behind a corporate firewall that blocks UDP and non-443 TCP.** QUIC dies; TCP dial to ports other than 443 dies. Mitigation: listen TCP on 443 alongside the random ephemeral ports, advertise both. Beyond that, see (1).
5. **AutoRelay v2 reservation churn** on small meshes. If the publicly-reachable peer reboots, the rest of the mesh re-discovers via PEX (peer exchange runs on the Identify protocol) within seconds. Brief outage during the rediscovery.

## 9. Library dependency cost — measured, not feared

The current `go.mod` has 2 direct dependencies (testify, rapid). After adoption:

```
go-libp2p                      v0.45.x  (Nov 2025, post-Shipyard handoff)
go-libp2p-pubsub              v0.13.x
go-libp2p/p2p/transport/quic  (in the same module)
go-libp2p/p2p/security/noise  (in the same module)
go-libp2p/p2p/discovery/mdns  (in the same module)
go-libp2p/p2p/host/autorelay  (in the same module)
go-libp2p/p2p/host/holepunch  (in the same module)
```

That's 2 direct deps added (libp2p, pubsub). Transitively ~80 modules pull in. They are well-vendored, no native code, no CGO, total binary size impact ~12-18MB stripped on linux/amd64. Comparable to embedding bbolt + cobra + bubbletea (which the other plans already do). This is not an npm tarpit.

## 10. Open questions

The following are not blocking the adoption decision but need to be resolved during implementation:

1. **GossipSub message validator design.** Do we sign deltas at the CRDT layer or at the GossipSub envelope layer? Leaning envelope-layer (cleaner separation), but the CRDT layer already has `Timestamp.PeerID` baked in — there's an option to use that as the validator's trust anchor.
2. **Allow-list propagation race.** When a new peer is added on device A, the CRDT delta has to propagate to device C before C will accept GossipSub messages from the new peer. If the new peer publishes before C learns of it, C drops the message and GossipSub will re-gossip it; convergence is eventual. Need to verify the lag is bounded in practice.
3. **Phone Doze + AutoRelay reservation expiry.** Default Circuit Relay v2 reservations are ~1h. If the phone Dozes longer than that and the publicly-reachable peer evicts the reservation, the phone needs to re-reserve on wake. Acceptable, but adds ~1s to wake latency.
4. **Replacing the action-gate `.pending`/`.response` file pattern.** The hook plan uses file-based atomicity (`O_CREAT|O_EXCL`) for first-response-wins across surfaces. This still works — `O_CREAT|O_EXCL` is local to the device. The libp2p change does not affect it.
5. **Bootstrap-free first connection.** First peer pairing requires manual multiaddr exchange (`agenthive id` → `agenthive peers add`). For subsequent peers, PEX over Identify discovers them automatically once any one of them is in the mesh. We do not implement DHT or rendezvous because we don't need global discovery.

## 11. Acceptance checklist

Implementation lands when these are all true:

- [ ] `internal/transport/host.go` constructs a libp2p Host with all options above
- [ ] `internal/protocols/{action,notification,peer,state}.go` register their stream handlers
- [ ] `internal/discovery/mdns.go` wires the mdns notifee
- [ ] GossipSub message validator enforces the CRDT peer allow-list
- [ ] `agenthive id` prints all multiaddrs
- [ ] `agenthive peers add <multiaddr>` adds the peer to the CRDT, propagates via GossipSub
- [ ] An integration test creates 3 in-process Hosts (via `p2p/net/mock`), exchanges CRDT deltas, asserts convergence
- [ ] An integration test with one Host listening on `127.0.0.1` and one Host behind a fake symmetric NAT (using `tc netem` or a userspace NAT mock) exercises DCUtR and asserts direct connection
- [ ] An integration test asserts a node with `EnableRelayService` correctly forwards a relayed connection between two NATed peers
- [ ] CI runs all of the above with `-race`

## 12. One-line verdict

> agenthive does not own a transport stack. agenthive owns its CRDT, its routing rules, and its surfaces. libp2p owns the wire. We're not building five years of distributed-systems infrastructure to ship a notification daemon.
