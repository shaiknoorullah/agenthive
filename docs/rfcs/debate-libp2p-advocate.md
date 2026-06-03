# Position Paper: go-libp2p as the Transport and Membership Layer for agenthive

**Author:** libp2p Advocate
**Date:** 2026-06-04
**Status:** RFC / Technical Debate Position
**Target:** agenthive transport + membership layer (R1-R7); successor to the autossh+stdio status-quo

---

## 1. Executive Summary

The CRDT coordination layer is settled. What is not settled is the lower half of the stack: the thing that opens sockets between a dev server in DigitalOcean, a laptop on a coffee-shop Wi-Fi, and a phone in Termux jumping between LTE and the home AP. The status quo proposes to solve this with `autossh -R` subprocesses, a bespoke `LinkManager`, a hand-rolled pairing ceremony, and newline-JSON over stdio. Every one of those is a solved problem that go-libp2p solves better in 2026.

This paper argues that **go-libp2p is the correct transport-and-membership substrate for agenthive** because (1) peer identity *is* the public key, which collapses pairing into key exchange; (2) NAT traversal is no longer guesswork — it is an instrumented, measured protocol with 70% ± 7.1% direct-connection success and a deterministic relay fallback for the remaining 30% [1]; (3) GossipSub gives the CRDT layer a battle-tested anti-entropy fabric used by Ethereum's L1 consensus and Filecoin [2][3]; (4) Noise XX is built in, audited, and identical to the Noise XX the status-quo plan was already going to use; and (5) the Android / Termux story, while imperfect, is *better* than autossh's, not worse — and improving (60% mobile battery savings shipped in 2025 [4]).

The status-quo plan is not a transport. It is a duct-tape harness around four transports (SSH, TCP, stdio, the LinkManager fan-out) that the agenthive maintainer will own forever. libp2p is one transport interface that absorbs all four, ships with the security primitives we'd otherwise write ourselves, and stops a 3-person side project from re-implementing a 2026-era P2P stack in Go.

---

## 2. The Axes, Scored Honestly

| Axis | autossh + LinkManager | go-libp2p |
|---|---|---|
| **NAT traversal** | Needs an SSH-reachable jump host for every pair. | 70% direct via DCUtR [1]; AutoRelay v2 covers the rest [5]. |
| **Mobile battery / Doze** | Keepalive + reconnect storms on Doze cycle. | QUIC + 2025 optimizations cut mobile battery ~60% [4]. |
| **Topology symmetry** | Hub-and-spoke. Some node must run sshd. | Symmetric mesh; every node is a Host. R3 by construction. |
| **Infrastructure footprint** | Always-on jump host. | Zero in the happy path; optional self-hosted relay for the 30% tail. R6 satisfied. |
| **Ops complexity** | Bespoke pairing + LinkManager + port alloc ≈ 1500 LoC owned. | `libp2p.New(...)` + ~200 LoC glue. Configured, not implemented. |
| **Security maturity** | SSH mature; pairing ceremony unaudited. | Noise_XX, PeerID = multihash(pubkey) [6][7]; 3 advisories in 4 years, all patched [8][9][10]. |
| **Library maturity 2026** | autossh is a supervisor, not a library. | v0.45.0 shipped 2025-11-06 [11]; 30+ chains, ~$100B secured [12]. |
| **Dep-tree weight** | 2 direct deps. | ~50-80 transitive. Larger. See §4.1. |
| **Scale 3→10+** | O(N) explicit dials + N×N pairing. | GossipSub mesh self-tunes, scales to thousands [2]. |
| **Connection migration** | Tunnel break → cold redial, drops in-flight. | QUIC migration preserves sessions across IP changes [13]; TCP+QUIC dialed in parallel [14]. |

The status-quo plan wins on dep-tree weight and only dep-tree weight.

---

## 3. The Architectural Punchline: Pairing Is Just Key Exchange

The status-quo plan describes a "hand-rolled pairing ceremony with Ed25519 keys." This is the moment a small project quietly becomes a security project. Every pairing scheme has to answer: how does Device A learn Device B's pubkey authentically? QR code? Shared secret? Out-of-band string?

libp2p makes this question go away. **A peer's identity *is* the multihash of its public key.** [6][7] A multiaddr like `/dnsaddr/laptop.tailnet/p2p/12D3KooWQ...` carries the pubkey by construction. If an attacker substitutes a different host at the same DNS name, the Noise handshake fails because the peer they reach cannot prove possession of the private key behind that PeerID. There is no separate pairing protocol — the address *is* the pairing.

Agenthive's pairing UX becomes: show a QR code containing one multiaddr. Other device dials. Done. No nonce exchange, no time-bounded windows, no re-pair on key rotation (rotate the host key, the PeerID changes, the user just re-shares the new multiaddr — same as rotating an SSH host key, except the user only ever copies one string).

This is the architectural point that should decide the debate: **the status-quo plan owns a custom security protocol it doesn't need to own.**

---

## 4. Rebuttals to the Adversarial Points

### 4.1 "libp2p is a sprawling dependency"

True, partially. go-libp2p's *own* go.mod declares ~51 direct dependencies because it bundles every transport, muxer, and security module [15]. A *consumer* import like `github.com/libp2p/go-libp2p` pulls fewer direct imports (one) but a substantial transitive closure — empirically 50-100 indirect entries in `go.sum` for a Host + GossipSub + connmgr setup.

The honest framing: we trade ~80 transitive deps for the deletion of ~1500 LoC of in-tree pairing, link multiplexing, SSH subprocess management, JSON framing, and reconnection logic. Those 80 deps are maintained by Protocol Labs successors (Shipyard handed go-libp2p off to community stewardship on 2025-09-30; v0.45.0 still shipped 2025-11-06 [11][16]) and by Ethereum, Filecoin, Optimism, Celestia in their CI loops. The 1500 LoC is maintained by one person on weekends.

R6 (no third-party cloud) is about *runtime* infrastructure, not source dependencies. libp2p satisfies R6 — runs entirely on the user's hardware.

### 4.2 "Termux can't host a full libp2p node"

The libp2p 2025 annual report documents ~60% mobile battery savings from QUIC optimizations and identifies mobile light clients as a target deployment [4]. Ethereum light clients and Portal Network nodes run libp2p on Android today.

Termux specifically is hostile because Doze kills background processes regardless of library [17] — autossh suffers identically. Under the same `termux-wake-lock` + battery-whitelist mitigation users need either way, libp2p does *better*: QUIC connection migration survives Wi-Fi↔LTE swaps that force autossh to redial [13], and AutoRelay v2 re-establishes via a relay on Doze wake rather than looping on `connection refused`.

For phones that can't sustain a node at all, an FCM bridge wakes the device for high-priority approvals. That sits *above* the transport, not inside it.

### 4.3 "Hole punching only works ~70% of the time"

Correct. The ProbeLab measurement campaign (4.4M attempts, 85K networks, 167 countries) puts DCUtR at **70% ± 7.1% success**, indistinguishable across TCP and QUIC, with 97.6% of successes on the first attempt [1]. For agenthive's 3-5 peer topology that means: ~70% of pairs get a direct connection on the first try; the other ~30% fall back to a relay.

This is a feature, not a bug. AutoRelay v2 means we ship one optional `agenthive-relay` binary that any user can run on a $4/mo VPS *or* on their own home NAS, and the mesh self-organizes [5]. Compare to the status quo: 100% of connectivity requires a jump host. libp2p's 30% relay tail is strictly less infrastructure than the status-quo's 100% jump-host requirement.

R2 (<1s round-trip) is also better satisfied. A relayed libp2p connection has a single TCP RTT through the relay; an autossh tunnel has SSH framing, tunnel framing, and a per-message JSON parse — multiple syscall hops the libp2p path avoids.

### 4.4 "libp2p has had CVEs; SSH has 30 years of hardening"

go-libp2p's published advisories [8][9][10]: GHSA-j7qp-mfxf-8xjw (Yamux DoS, High, Dec 2022, fixed in v0.18.0 via Resource Manager); GHSA-876p-8259-xjgg (large RSA key DoS, Moderate, Aug 2023); GHSA-gcq9-qqwx-rgj3 (signed peer record OOM, High, Aug 2023). Three advisories in four years, all patched, all pre-Resource-Manager. Recent GossipSub CVEs cited in adversary briefings [18] are in **rust-libp2p** and **js-libp2p** — different codebase.

The comparison isn't "SSH vs libp2p." It's "OpenSSH + autossh + our pairing + our LinkManager + our JSON framing" vs libp2p. The first three layers in the SSH stack have decades of audit; the last two have **zero**, because no one has looked at code that doesn't exist yet. go-libp2p has continuous hostile attention from Ethereum researchers and ProbeLab. Visible-and-patched beats invisible.

### 4.5 The CRDT layer needs anti-entropy gossip

The judgment that locked in CRDTs also implicitly locked in: we need a publish-subscribe substrate for delta propagation. GossipSub v1.2's IDONTWANT control message specifically optimizes the case our LWW-Register delta exchange will hit — many small updates, occasional large state syncs [19]. The status-quo plan would have to *build* this on top of the LinkManager. libp2p hands it to us as `pubsub.NewGossipSub(ctx, h).Join("agenthive/config/v1")`.

---

## 5. Weakest Claims (Things I'm Honestly Worried About)

1. **Shipyard handoff.** Protocol Labs / Interplanetary Shipyard ended dedicated go-libp2p maintenance on 2025-09-30 and is transitioning to community stewardship [16]. v0.45.0 shipped after the handoff [11], but the cadence may slow. *Mitigation:* pin a specific minor version, follow Ethereum's go-libp2p adoption (they will not let it bitrot), and budget a quarterly dependency-bump task.
2. **Dep-tree weight is real.** Going from 2 direct deps to ~80 transitive ones changes the supply-chain posture. `go mod why` will be a tool we use often. We accept this cost because the alternative is owning the same surface area in our own source tree.
3. **Termux + Doze is genuinely fragile** regardless of transport. Neither libp2p nor autossh fully solves it. Honest framing: agenthive on phone is "best-effort wake on approval," not "always-online peer." R5 (resilient to intermittent connectivity) is satisfied because libp2p reconnects automatically, but R2 (<1s latency) may degrade after a Doze cycle.
4. **AutoRelay v2 reservation logic is still maturing.** The 2025-06-23 v0.42.0 release notes explicitly say "in a future release, AutoRelay will use this reachability information" [5]. We are betting on a release timeline.
5. **Connection migration across networks** is a QUIC feature, not a libp2p feature. TCP transport in libp2p does not migrate; mid-session network changes on TCP cause a redial [13]. We must configure QUIC as the preferred transport on mobile.

These are real costs. They are smaller than the cost of writing a bespoke pairing protocol that no one will ever audit.

---

## 6. Concrete Migration Sketch — `internal/transport/`

```
internal/
  crdt/                          (unchanged — LWW-Register, HLC)
  transport/
    host.go                      // wraps libp2p.Host + lifecycle
    identity.go                  // PeerID <-> persistent Ed25519 key
    pairing.go                   // QR encoder/decoder for multiaddrs
    gossip.go                    // GossipSub topics for CRDT deltas + RPC
    rpc.go                       // request/response over libp2p Streams
    relay.go                     // optional self-hosted relay mode
  membership/
    peerstore.go                 // libp2p Peerstore + agenthive labels
    routes.go                    // CRDT route table, gossiped via gossip.go
```

Key types:

```go
// internal/transport/host.go
type Host struct {
    h        host.Host           // libp2p Host
    ps       *pubsub.PubSub      // GossipSub instance
    dht      *dht.IpfsDHT        // optional, off by default — we don't need global discovery
    connmgr  connmgr.ConnManager // low watermark 3, high watermark 32
    log      *slog.Logger
}

func New(ctx context.Context, cfg Config) (*Host, error) {
    priv := loadOrCreateIdentity(cfg.KeyPath)        // identity.go
    h, err := libp2p.New(
        libp2p.Identity(priv),
        libp2p.ListenAddrStrings(
            "/ip4/0.0.0.0/udp/0/quic-v1",            // mobile-friendly
            "/ip4/0.0.0.0/tcp/0",                    // fallback
        ),
        libp2p.Security(noise.ID, noise.New),        // Noise XX, the same primitive we were going to use anyway
        libp2p.EnableHolePunching(),                 // DCUtR
        libp2p.EnableAutoRelayWithStaticRelays(cfg.Relays),
        libp2p.EnableAutoNATv2(),
        libp2p.ConnectionManager(connmgr),
        libp2p.ResourceManager(rcmgr),
    )
    ps, _ := pubsub.NewGossipSub(ctx, h)
    return &Host{h: h, ps: ps, connmgr: connmgr}, nil
}
```

Key methods:

```go
// internal/transport/pairing.go
func (h *Host) PairingMultiaddr() string         // QR payload
func ParsePairing(qr string) (peer.AddrInfo, error)
func (h *Host) Connect(ctx context.Context, ai peer.AddrInfo) error

// internal/transport/gossip.go
func (h *Host) Publish(topic string, payload []byte) error
func (h *Host) Subscribe(topic string) (<-chan Message, error)

// internal/transport/rpc.go — for approval round-trips that need acks
func (h *Host) RegisterHandler(proto protocol.ID, fn StreamHandler)
func (h *Host) Request(ctx context.Context, peer peer.ID, proto protocol.ID, req []byte) ([]byte, error)
```

Wire-up to the existing CRDT layer:

- `internal/crdt` publishes deltas via `transport.Host.Publish("agenthive/state/v1", delta)`.
- `internal/crdt` subscribes to the same topic and applies inbound deltas.
- Approval requests (need synchronous round-trip for <1s R2) use the `rpc.go` Stream-based path with protocol ID `/agenthive/approve/1.0.0`.
- No `LinkManager`. No `autossh`. No JSON-over-stdin framing. Deleted: the entire pairing module, the SSH subprocess supervisor, the reconnection state machine. Estimated net code change: **−1500 LoC, +400 LoC**.

The status-quo plan is more code, more bugs, less measured, and ships with no Ethereum-funded reviewers watching the dependency. Pick libp2p.

---

## 7. Sources

1. *Challenging Tribal Knowledge: Large-Scale Measurement Campaign on Decentralized NAT Traversal*, arXiv 2510.27500 (Oct 2025) — 70% ± 7.1% DCUtR success, 4.4M attempts, 85K networks. https://arxiv.org/abs/2510.27500
2. libp2p specs — GossipSub v1.2 spec. https://github.com/libp2p/specs/blob/master/pubsub/gossipsub/gossipsub-v1.2.md
3. *GossipSub: Attack-Resilient Message Propagation in the Filecoin and ETH2.0 Networks*, arXiv 2007.02754. https://arxiv.org/abs/2007.02754
4. libp2p 2025 Annual Report — mobile ~60% battery savings, AutoNAT v2 / AutoTLS / WebRTC stabilization. https://libp2p.io/reports/annual-reports/2025/
5. Announcing the release of go-libp2p v0.42.0 (2025-06-23) — AutoNAT v2 per-address reachability, rate limiting, QUIC source-address verification. https://libp2p.io/releases/2025-06-23-go-libp2p/
6. libp2p Noise spec — Noise_XX_25519_ChaChaPoly_SHA256. https://github.com/libp2p/specs/blob/master/noise/README.md
7. libp2p Peer ID spec — multihash-of-pubkey identity. https://github.com/libp2p/specs/blob/master/peer-ids/peer-ids.md
8. GHSA-j7qp-mfxf-8xjw — Yamux/Resource Manager DoS, High, Dec 2022. https://github.com/libp2p/go-libp2p/security/advisories/GHSA-j7qp-mfxf-8xjw
9. GHSA-876p-8259-xjgg — Large RSA key DoS, Moderate, Aug 2023. https://github.com/libp2p/go-libp2p/security/advisories/GHSA-876p-8259-xjgg
10. GHSA-gcq9-qqwx-rgj3 — Signed peer record OOM, High, Aug 2023. https://github.com/libp2p/go-libp2p/security/advisories/GHSA-gcq9-qqwx-rgj3
11. Announcing the release of go-libp2p v0.45.0 (2025-11-06) — post-Shipyard-handoff release. https://libp2p.io/releases/2025-11-06-go-libp2p/
12. libp2p users page — Ethereum, Filecoin, Optimism, Celestia, Avail, EigenDA, Algorand, Starknet, Flow, Mina, IPFS. https://docs.libp2p.io/concepts/introduction/users/
13. quic-go documentation — Connection Migration; client-initiated path probe; preserves session across IP changes. https://quic-go.net/docs/quic/connection-migration/
14. libp2p QUIC transport docs — parallel dial of TCP + QUIC. https://pkg.go.dev/github.com/libp2p/go-libp2p
15. go-libp2p go.mod (master) — direct dependency count and contents. https://github.com/libp2p/go-libp2p/blob/master/go.mod
16. *An update about Libp2p maintenance at Shipyard* — handoff to community maintainers, 2025-09-30 cutover. https://ipshipyard.com/blog/2025-libp2p-maintenance-update/
17. Termux Doze-mode issue tracker — wake-lock and battery-optimization whitelist required for long-running processes. https://github.com/termux/termux-app/issues/377
18. CVE-2026-46679 — js-libp2p GossipSub subscription-flood memory DoS (different codebase). https://advisories.gitlab.com/npm/@libp2p/gossipsub/CVE-2026-46679/
19. libp2p specs PR #548 — GossipSub v1.2 IDONTWANT control message. https://github.com/libp2p/specs/pull/548
