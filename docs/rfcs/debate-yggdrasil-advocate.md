# Position Paper: Yggdrasil IPv6 Mesh as the Transport for agenthive

**Author:** Yggdrasil-Network Advocate
**Date:** 2026-06-04
**Status:** RFC / Technical Debate Position
**Target:** agenthive transport-and-membership layer (R1-R7)

---

## Executive Summary

The status-quo proposal builds a transport from `autossh` reverse tunnels, hand-rolled Ed25519 pairing, a custom `LinkManager`, framed JSON, plus a separate Noise-XX path for LAN. The SSH advocate's paper is internally consistent — but it spends 650 lines re-implementing, by hand and in shell, three things the kernel and a 4500-line Go library already do: **addressing**, **routing**, and **encryption**. Every bug class on the R1-R7 axes — Termux without a `sshd`, port collisions, NAT pinholes, manual pairing UX, link multiplexing — exists because the SSH design forces the application to be its own network layer.

Yggdrasil inverts the abstraction. Each agenthive peer embeds the `yggdrasil-go` core as a Go library, derives a stable `200::/7` IPv6 address from its Ed25519 public key, and joins a self-organising mesh. The agenthive daemon then becomes a plain TCP server bound to that IPv6 address. **Peer identity is the address. NAT traversal is the routing layer's problem, not ours. End-to-end encryption is below the socket.** The CRDT gossip already decided by the prior judgment runs on top, unchanged, as a normal TCP application protocol.

The architectural punchline: **the entire `internal/transport/` package collapses to roughly 100 lines of Go.** No SSH subprocess. No Noise handshake. No pairing ceremony — pairing is "exchange these two `200:...` strings out-of-band, like you already do for SSH `known_hosts`". No `LinkManager` multiplexing because the kernel's TCP stack already does it. R1-R7 are satisfied not by clever engineering but by *not engineering them at all* and inheriting them from the transport.

I will concede the weakest claims honestly in §10 — embedding is under-documented, Yggdrasil self-describes as alpha, and Termux on stock Android needs the VPNService trick. None of these are blockers; the SSH design has worse versions of each.

---

## 1. The Architectural Inversion

```
+-------------------------------------------------------------+
|  agenthive daemon (Go binary, runs on every peer)           |
|                                                             |
|  +-------------------------------------------------------+  |
|  |  internal/transport  -- ~100 LOC                      |  |
|  |    net.Listen("tcp", "[<my-ygg-v6>]:9999")            |  |
|  |    net.Dial("tcp", "[<peer-ygg-v6>]:9999")            |  |
|  |  No TLS. No Noise. No SSH. No pairing ceremony.       |  |
|  +---------------------------+---------------------------+  |
|                              |                              |
|  +---------------------------v---------------------------+  |
|  |  embedded yggdrasil-go core (Go library, in-process)  |  |
|  |    - Ed25519 node key -> 200::/7 IPv6                 |  |
|  |    - Spanning-tree + greedy routing in metric space   |  |
|  |    - RTT-aware link cost (v0.5.13)                    |  |
|  |    - End-to-end XChaCha20-Poly1305 + Curve25519       |  |
|  |  Outbound dials to known public peers OR LAN peers.   |  |
|  +---------------------------+---------------------------+  |
|                              |                              |
|  +---------------------------v---------------------------+  |
|  |  TUN device (Linux/macOS) OR VPNService (Android)     |  |
|  +-------------------------------------------------------+  |
+-------------------------------------------------------------+
                              |
              ~~ any network: LAN, WAN, CGNAT, LTE ~~
                              |
                  [ other agenthive peers ]
```

Everything above the `transport/` boundary — the CRDT engine, HLC, anti-entropy, action-request semantics — is unaffected. We are arguing exclusively about what lives below the gossip layer.

---

## 2. Why an IPv6 Mesh Is the Architecturally Correct Shape

The R1-R7 requirements describe a **flat, symmetric, NAT-oblivious, identity-bearing fabric**. That is the literal definition of an overlay mesh. SSH and WebRTC both fight this shape:

- **SSH** is hub-and-spoke at the transport level — each tunnel has a client and a server, and the "every device equal" property (R3) is faked above the transport by labelling tunnels as bidirectional. With N peers you need N*(N-1)/2 tunnels and a pairing matrix; the SSH advocate elides this by routing through a designated "local relay," which is exactly the hub-and-spoke topology R3 forbids.
- **WebRTC** is peer-to-peer but requires a signalling channel and STUN/TURN. The TURN fallback (~15-30% of real-world connections behind symmetric NAT and CGNAT [^tailscale-nat]) reintroduces a relay server, violating R6.
- **Yggdrasil** is the only option where "peer A talks to peer B" is a single concept regardless of LAN/WAN/NAT, and where the address space *itself* is the identity space. That is exactly R3 + R4 + R5 + R6 in one primitive.

---

## 3. Mapping Yggdrasil to the Scoring Axes

### 3.1 NAT traversal reliability (Yggdrasil wins outright)

Yggdrasil peers establish outbound TCP/QUIC connections to known peers (configured public peers, LAN-discovered peers via multicast, or other agenthive nodes you've pointed it at). Once any peer has any outbound link into the mesh, **the entire `200::/7` address space becomes reachable** because the spanning tree routes packets through intermediate nodes [^yggfaq]. The FAQ states explicitly: "you will be able to accept incoming connections on your Yggdrasil IPv6 addresses" even behind carrier-grade NAT, with no port forwarding required.

This is fundamentally stronger than SSH-reverse-tunnel-to-a-relay. The SSH design fails when the relay host is down; Yggdrasil's mesh has no relay host — any peer that holds any outbound connection serves the same role.

For latency, the optional `yggdrasil-jumper` companion adds STUN-based hole-punching to upgrade routed paths into direct UDP/QUIC links transparently, with reported improvements from multi-second routed latency down to ~120ms direct [^jumper]. This is opt-in and below the agenthive layer.

### 3.2 Mobile battery + Android Doze (acceptable, not best-in-class)

The official Yggdrasil Android app (`eu.neilalexander.yggdrasil`, F-Droid, current as of v0.1-021 / March 2026 [^fdroid]) runs as an Android `VpnService`. Battery cost is dominated by the persistent socket keepalive — comparable to any WireGuard/OpenVPN client. Standard mitigations apply: request battery-optimisation exemption and enable Always-On VPN [^doze]. Importantly, only one VPNService can be active at a time on stock Android, which constrains the design choice (see §5 on Termux).

For R2 (<1s round-trip), the cost is the keepalive timer, not per-message wakeups — once the tunnel is up, a TCP packet from the dev server arrives at the phone in tens to low-hundreds of milliseconds and triggers the agenthive process via a normal accepted connection.

### 3.3 Topology symmetry (perfect fit)

Every peer is a node in the mesh with an equal IPv6. There is no client vs. server, no "designated relay machine." This is R3 verbatim.

### 3.4 Infrastructure footprint (zero)

No coordination server. No DERP/TURN. No `sshd` running for tunnel-receive duty. Public Yggdrasil peers are used only for bootstrap (and only when no LAN peer is available); the project's public peer list is community-run and freely substitutable — agenthive can also be configured to use only the user's own peers, satisfying R6 in the strictest reading.

### 3.5 Ops complexity (lower, not higher)

The user-facing operation reduces to:

1. Install the agenthive binary.
2. On first run, it generates a key, prints its `200:abcd:...` address.
3. Add another peer's address to a `peers:` list (or paste it into the running daemon's CRDT-synced config — which is exactly the mechanism R4 already promises).

There is no SSH key rotation step, no `authorized_keys` editing, no `ServerAliveInterval` tuning, no port-allocation convention across N machines. The peer-identity-is-the-address property means the address book is the entire pairing system.

### 3.6 Security model maturity

Yggdrasil's stack: Ed25519 node identity, Curve25519 ECDH for session keys, ratcheting keys rotated per RTT or nonce overflow, end-to-end authenticated encryption between any two `200::/7` addresses [^yggcrypto] [^v04prep]. All primitives come from `golang.org/x/crypto` — the same library SSH-server-in-Go implementations use [^yggfaq]. The cryptographic posture is **stronger** than the SSH design because every packet is end-to-end encrypted from source IPv6 to destination IPv6, even when transiting intermediate mesh nodes — they cannot decrypt, only forward. SSH only encrypts hop-by-hop.

### 3.7 Library maturity in 2026

`yggdrasil-go` v0.5.13 shipped on **2026-02-24** [^releases], requires Go 1.24, and the v0.5 protocol has been stable and protocol-compatible across point releases since late 2023. The third-party `ygg` wrapper [^ygg] provides a clean embedded API (`New(cfgPath)`, `Close()`, `SetConnectivityHandler`); a worked tutorial [^embed-tut] shows the direct import path:

```go
import (
    "github.com/yggdrasil-network/yggdrasil-go/src/core"
    "github.com/yggdrasil-network/yggdrasil-go/src/config"
)
```

The `core` package on `pkg.go.dev` is a public, versioned Go module [^pkgdoc].

### 3.8 Dep-tree weight

One Go module, MIT-licensed, ~50 transitive deps, all already in the Go ecosystem (no C library, no CGo). Compare to the SSH proposal's runtime deps: `autossh`, `socat`, `jq`, `ssh`, plus a bash-or-Go process supervisor — all of which must exist on every peer including Termux.

### 3.9 Scalability to 10+ peers

The mesh is N-to-N at the addressing layer; agenthive opens one TCP connection per other peer it actively gossips with. The v0.5.13 routing algorithm balances tree-distance and link cost and is RTT-aware [^releases]. At 10 peers this is trivially below any threshold. The greedy-routing-in-metric-space design is what Yggdrasil is specifically engineered for at internet scale, so 10+ is a non-question.

### 3.10 Encryption + handshake quality

Curve25519 + Ed25519 + (X)ChaCha20-Poly1305-equivalent AEAD with per-RTT ratchet [^v04prep]. Modern, post-Snowden-era primitives. No TLS cert management, no certificate-authority trust roots, no Noise handshake state machine in our code.

### 3.11 Connection migration across networks

This is where Yggdrasil decisively beats both SSH and WebRTC. Because the address is bound to a key, **not** to a network interface, the laptop closing its lid and reopening on a coffee-shop Wi-Fi is invisible to agenthive: the same `200:...` address reappears, the routing layer rediscovers a path, and the TCP session above either resumes or is cleanly reopened by the gossip layer's normal reconnection logic. SSH must tear down `autossh` and re-establish; WebRTC must do ICE restart with new signalling.

---

## 4. Concrete Migration Sketch — `internal/transport/`

Today there is no `internal/transport/` (the repo currently has `internal/crdt/`). Under this proposal the entire package is roughly:

```go
// internal/transport/yggdrasil.go  -- the whole package
package transport

import (
    "net"
    "github.com/yggdrasil-network/yggdrasil-go/src/core"
    "github.com/yggdrasil-network/yggdrasil-go/src/config"
)

type Transport struct {
    node *core.Core
    ln   net.Listener
}

func Start(cfgPath string, port int) (*Transport, error) {
    cfg, err := config.GenerateConfig(), error(nil)
    if cfgPath != "" { /* load from disk */ }

    node, err := core.New(cfg.Certificate, discardLogger{}, /* opts */)
    if err != nil { return nil, err }

    addr := node.Address()                 // 200:abcd:...
    ln, err := net.Listen("tcp",
        net.JoinHostPort(addr.String(), itoa(port)))
    if err != nil { return nil, err }

    return &Transport{node: node, ln: ln}, nil
}

func (t *Transport) Accept() (net.Conn, error)        { return t.ln.Accept() }
func (t *Transport) Dial(peer string) (net.Conn, error) {
    return net.Dial("tcp", net.JoinHostPort(peer, "9999"))
}
func (t *Transport) Address() string { return t.node.Address().String() }
func (t *Transport) Close() error    { _ = t.ln.Close(); return t.node.Stop() }
```

That's it. The gossip layer above calls `Accept()` / `Dial(peer-v6)` and treats each `net.Conn` as a normal stream. No framing changes. No multiplexing. The CRDT delta exchange is just `gob` / `protobuf` over TCP. **The SSH advocate's §3 has 200 lines of shell that this 30 lines of Go subsumes.**

Pairing reduces to one line in the CRDT-synced config (R4):

```yaml
peers:
  - addr: "200:abcd:1234:.../128"     # phone
  - addr: "201:fedc:9876:.../128"     # dev-server
```

Because R4 already promises edit-on-any-device-propagates-to-all, *adding a new peer is a one-line CRDT op, fully consistent with the prior judgment.*

---

## 5. Rebuttals to the Strongest Adversarial Points

**5.1 "TUN requires root / CAP_NET_ADMIN; Termux can't open TUN."**
True on stock Android, but the official Yggdrasil Android app proves the answer: it runs as an `android.net.VpnService`, which the OS grants to any app the user approves at install time [^android-app]. agenthive's Android build is **not a Termux binary** in the v1 design — it is a sibling app that exposes its UNIX socket / a localhost TCP port that the user's Termux session can talk to. The Termux process still meets R1 ("first-class terminal-shaped peer") because the agent code runs in Termux and reaches the mesh via the sidecar VPN app, exactly the way every other tool on a non-rooted phone reaches privileged networking. The SSH design has no answer to this at all — Termux's stock `sshd` is fine *outbound* but the SSH advocate's `autossh -R` requires the phone to *receive* the reverse tunnel, which is the much harder problem.

On Linux/macOS, the agenthive daemon either runs with `CAP_NET_ADMIN` (one-line systemd capability grant) or — and this is the elegant option — uses the Yggdrasil core *without* a TUN device at all and exposes only the `net.Listener` API, keeping all packets inside the userspace overlay. The embedding tutorials show exactly this pattern [^embed-tut].

**5.2 "Embedding Yggdrasil is unusual."**
Conceded that it's unusual; rejected that it's unsupported. The `src/core` package is a public Go module on `pkg.go.dev` [^pkgdoc]. The `ygg` library [^ygg] is a published wrapper with `New() / Close() / SetConnectivityHandler` semantics. The DEV Community walkthrough [^embed-tut] gives concrete imports and a working `Listen` / `CallPeer` example. The maintainer hasn't *blessed* embedding officially [^embed-disc], but the code is structured to permit it and at least two third-party libraries demonstrate it works. We are not the first.

**5.3 "Tailscale has 100x the users — why not Tailscale?"**
Conceded on market share, rejected on architectural fit. Tailscale's control plane runs on `tailscale.com` (or a self-hosted Headscale, which is itself a single point of coordination). R6 says **zero third-party cloud infrastructure** and R3 says **every device equal, no hub/spoke**. A coordination server is a hub. Yggdrasil is coordinator-free by design — peer discovery is via multicast on LAN, configured peer URIs on WAN, or transitive routing once any peer is reachable. Tailscale solves a different problem (corporate device fleets); Yggdrasil solves *our* problem.

**5.4 "Mesh routing has worse latency than direct connections."**
For nearby peers Yggdrasil establishes direct outbound links from the config — there is no mesh hop. Multi-hop only occurs when neither end can reach the other directly, which is precisely the case where SSH would need a TURN-equivalent anyway. The v0.5.13 RTT-aware cost function actively prefers low-latency paths [^releases], and `yggdrasil-jumper` opportunistically upgrades multi-hop paths into direct holepunched links [^jumper]. Real-world numbers from the jumper README show 1-3 second iperf3 latencies collapsing to ~120ms when direct paths are established. For R2 (<1s round-trip on action-approval), a single direct mesh hop is comfortably within budget.

**5.5 "Yggdrasil v0.5 is still pre-1.0."**
Conceded the version number; rejected the implication. The v0.5 protocol shipped in 2023, has been protocol-stable across point releases for ~2.5 years, with the latest v0.5.13 in February 2026 [^releases]. The FAQ self-describes as "alpha" but immediately qualifies: "relatively stable and very rarely crashes" [^yggfaq]. Compare the SSH design's hand-rolled `LinkManager` and pairing ceremony, which have *zero* releases and *zero* production deployments. "Alpha but stable for 2.5 years and in F-Droid" beats "novel code I wrote last week."

---

## 6. Weakest Claims (Honest Concessions)

I will not pretend this proposal is risk-free.

1. **The maintainer has not officially endorsed embedding.** [^embed-disc] If the `core` package's API shifts in a future release, agenthive carries the cost of catching up. Mitigation: pin to a specific yggdrasil-go SHA; the v0.5 protocol's stability suggests breakage is unlikely.
2. **Yggdrasil self-describes as alpha.** [^yggfaq] The advisory against "life-or-death workloads" is real. agenthive isn't life-or-death (R1-R7 is a developer-tool use case), so this is an acceptable risk, but a regulated-industry user should know.
3. **The Android story requires a sidecar VPNService app, not pure Termux.** That is one more install step than "just run the binary in Termux." The SSH design avoids this — but only because it can't actually solve R1 cleanly either (autossh -R from inside Termux is awkward and battery-hostile).
4. **Linux peers need CAP_NET_ADMIN unless we go TUN-less.** The TUN-less embedded mode is the cleaner answer and is what I'd ship, but it does mean we can't accept connections from non-agenthive Yggdrasil software on the same host — which we don't want anyway.
5. **The `200::/7` namespace is shared with the global Yggdrasil network.** Anyone can dial our peers' IPv6 addresses. Mitigation: the agenthive daemon enforces a Yggdrasil-key allowlist at `Accept()` time — a 5-line check that the remote `200:...` is in the CRDT-synced peer list. This is the only piece of identity logic we keep in the application.

---

## 7. Conclusion

The SSH-tunnel proposal builds the wrong abstraction. It treats the transport as something to be assembled in the application out of shell tools, then re-implements identity, NAT traversal, encryption, and pairing on top — five problems Yggdrasil already solves below the socket. WebRTC has the right peer-to-peer mental model but needs signalling and TURN, violating R6.

Yggdrasil is the only option where the transport, the addressing, the encryption, and the membership model all collapse into one primitive: **an IPv6 address derived from a public key, routed by a self-organising mesh.** The agenthive daemon then needs roughly 100 lines of Go to participate. The CRDT layer already accepted by the prior judgment runs on top, unmodified, with peer identity = peer address.

It looks exotic because the average Go developer hasn't reached for a TUN-based overlay before. It is in fact the **most boring** of the three proposals once you commit to "every device equal, mesh-of-N, NAT just goes away" — because that's literally the design brief Yggdrasil was built to satisfy.

The dark horse is the architecturally correct horse. Ride it.

---

## Sources

[^releases]: yggdrasil-go releases — v0.5.13 released 2026-02-24, Go 1.24, RTT-aware routing. <https://github.com/yggdrasil-network/yggdrasil-go/releases>
[^yggfaq]: Yggdrasil Network FAQ — production-readiness, NAT behaviour, library origins. <https://yggdrasil-network.github.io/faq.html>
[^yggcrypto]: Yggdrasil crypto package on pkg.go.dev — wraps golang.org/x/crypto curve25519, ed25519, nacl/box. <https://pkg.go.dev/github.com/yggdrasil-network/yggdrasil-go/src/crypto>
[^v04prep]: "Preparing for Yggdrasil v0.4" — Ed25519 identity, ratcheting keys per RTT/nonce. <https://yggdrasil-network.github.io/2021/06/19/preparing-for-v0-4.html>
[^pkgdoc]: yggdrasil-go core package — public Go module documentation. <https://pkg.go.dev/github.com/yggdrasil-network/yggdrasil-go/src/core>
[^ygg]: svanichkin/ygg — third-party embedding wrapper with `New() / Close() / SetConnectivityHandler`. <https://github.com/svanichkin/ygg>
[^embed-tut]: "Yggdrasil Network as an Embedded Go Library" — DEV Community walkthrough with imports and `Listen`/`CallPeer` example. <https://dev.to/asciimoth/yggdrasil-network-as-an-embedded-go-library-9h>
[^embed-disc]: GitHub Discussion #915 — community asks about embedding; no official maintainer guidance yet. <https://github.com/yggdrasil-network/yggdrasil-go/discussions/915>
[^android-app]: yggdrasil-android — reference Android client built on VpnService. <https://github.com/yggdrasil-network/yggdrasil-android>
[^fdroid]: Yggdrasil Android app on F-Droid — v0.1-021 (2026-03-15). <https://f-droid.org/en/packages/eu.neilalexander.yggdrasil/>
[^doze]: Android Doze and App Standby developer guidance. <https://developer.android.com/training/monitoring-device-state/doze-standby>
[^jumper]: yggdrasil-jumper — STUN-based hole-punching companion; 1-3s -> 120ms latency improvement. <https://github.com/one-d-wide/yggdrasil-jumper>
[^tailscale-nat]: Tailscale, "How NAT traversal works" — symmetric NAT and CGNAT fallback rates. <https://tailscale.com/blog/how-nat-traversal-works>
