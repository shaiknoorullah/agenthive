# Position Paper: QUIC + mTLS (via quic-go) as the Transport Layer for agenthive

**Author:** QUIC + mTLS Advocate
**Date:** 2026-06-04
**Status:** RFC / Technical Debate Position
**Target:** agenthive transport-and-membership layer (R1-R7), atop the already-decided LWW-Register + HLC + delta gossip coordination layer.

---

## 1. Executive Summary

agenthive must shuttle approval requests between a dev server, a laptop, and a phone-in-Termux while the phone is the most aggressively moving, most aggressively NATed, most battery-managed peer in the mesh. The status-quo proposal — `autossh -R` subprocesses, newline-JSON over stdio, a hand-rolled `LinkManager`, Noise XX bolted on for LAN — optimises for the case where exactly one machine moves rarely. agenthive’s defining case is the opposite: **a phone whose IP changes mid-approval-request, four times a day, every day.**

QUIC, via `quic-go`, is the only transport on the table that treats this case as a wire-level primitive instead of a teardown event. RFC 9000 §9 specifies connection migration: a QUIC connection survives a change of client IP and port because it is keyed on a 64-bit Connection ID, not the 4-tuple.[^1] The wifi→cellular handoff that destroys an `autossh` tunnel and triggers a 5–15 s reconnect storm is, on QUIC, a single `PATH_CHALLENGE` / `PATH_RESPONSE` round-trip — the in-flight approval keeps going.[^2] This is the single feature most aligned with R2 (<1 s round-trip), R5 (resilience), and R1 (Termux phone as first-class peer).

mTLS gives us the membership story the SSH proposal hand-rolls badly. Self-signed peer certs pinned by SPKI fingerprint mean the peer ID *is* the cryptographic identity — no CA, no PKI, no key-distribution beyond `ssh-copy-id`-equivalent UX, with a battle-tested TLS 1.3 handshake underneath. The honest cost is that QUIC ships no NAT-traversal of its own; we address that in §4. **Thesis: QUIC + mTLS is the only transport whose unique feature is the exact failure mode agenthive is built to handle.**

---

## 2. Why connection migration is the whole ballgame

The flow that matters:

> Claude Code on the dev server asks: "run `rm -rf node_modules`?" → the phone on home wifi buzzes → the user walks out the door → the phone has dropped wifi and re-associated to LTE before they tap *Approve*.

On autossh+TCP that is two TCP RSTs and a reconnect. The phone's outbound tunnel dies when the radio switches; `autossh` notices via missed `ServerAliveInterval` (default 30 s × 3), tears down, reconnects, re-runs SSH handshake, re-establishes reverse forward. In the worst case the *approval response itself* is lost mid-tunnel. R2 (<1 s) is unsalvageable on the seam.

On QUIC this is one `PATH_CHALLENGE` exchange. Connection ID unchanged, TLS 1.3 keys unchanged, in-flight streams unchanged. `quic-go` exposes it directly: `conn.AddPath(transport)` → `path.Probe(ctx)` → `path.Switch()`. The application never sees a disconnect.[^3]

A 2024 SIGCOMM CCR study found server-side migration support on the public internet is uneven — notably, **Google and Cloudflare don't implement it**.[^4] **That finding is irrelevant to agenthive: we control both endpoints.** We are not riding a CDN's policy; we run our own quic-go listener on each peer. The capability internet providers haven't bothered with is the one we turn on by default.

This is the central thesis. Every other axis exists to support it.

---

## 3. quic-go in 2026: production-grade, pre-1.0, not actually unstable

The honest attack: "quic-go is still 0.x and breaks API every release." Facts:

- Latest release **v0.59.1 (May 2025)**, with v0.59.0 in Jan 2025 and v0.58.x in Dec 2024.[^5]
- v0.53.0 (June 2024) did a "massive overhaul" converting interfaces to concrete structs — a real, one-time break.[^5]
- Production users: **Cloudflare (cloudflared), libp2p (powering IPFS Kubo and Filecoin Lotus), Syncthing, Caddy, Traefik, AdGuard Home, Hysteria, frp, v2ray-core.**[^6]

Cloudflare's edge tunnel daemon, Filecoin's storage network, and Syncthing's file sync all carry real user traffic on this library. The library implements RFC 9000, 9001, 9002, 9114 (HTTP/3), 9204 (QPACK), 9297 (HTTP Datagrams) and tracks current Go (1.24/1.25).[^6] The pre-1.0 number is a versioning convention, not an engineering fact.

Compare the status quo: **`autossh` is a shell-script wrapper around OpenSSH whose state machine is "restart the subprocess on EOF."** No in-process integration, no structured errors, observability via log scraping. The dependency footprint is a separate binary on `PATH`, a subprocess tree, and a hand-written stdio framer. Calling quic-go "immature" against that is a category error.

---

## 4. NAT traversal: the honest concession and the architectural answer

**Concession.** QUIC has no built-in NAT traversal. `draft-seemann-quic-nat-traversal` expired in March 2024 at v02 as an individual submission, never adopted by the QUIC WG.[^7] If you want hole-punching, you bring your own.

**Answer.** agenthive's topology — 3–5 peers in v1, 10+ in v2, all owned by one user under one trust root — is the easy case. The hard case is libp2p's: millions of strangers, no shared trust. libp2p solved it with DCUtR, and the 2025 measurement campaign reports **~70% ± 7.1% hole-punching success across 4.4M attempts on 85k networks, with TCP and QUIC statistically indistinguishable**.[^8] That is the worst-case number for a fully decentralised mesh. agenthive has every advantage libp2p lacks, plus R6 explicitly permits self-hosted relays.

Concrete architecture:

1. **LAN-direct:** mDNS / link-local discovery → direct QUIC dial. Zero punching.
2. **WAN with one self-hosted STUN-lite + relay fallback:** ~200 LOC Go binary on a cheap VPS. Each peer registers its UDP 4-tuple; for A→B, both peers simultaneously send QUIC Initials at each other and quic-go's path validation handles the NAT bindings naturally.
3. **Relay fallback:** if punching fails (symmetric NAT both ends), the same VPS forwards QUIC packets transparently — end-to-end encrypted, relay sees ciphertext only.

The SSH proposal needs *every peer* to expose an inbound sshd, *or* a central tunnel host that violates R3 (symmetry). We need **one** optional, untrusted VPS for the rare punch-fail. Smaller infrastructure footprint, not larger.

---

## 5. The UDP-blocking objection

"UDP gets blocked on corporate / hotel / cellular networks." Real but smaller in 2026 than its reputation. Cloudflare's 2025 Radar data has HTTP/3 (= QUIC over UDP) at **~21% of global web traffic mid-year, climbing toward 35% by October**.[^9] One in three web requests already traverses these middleboxes. Carriers do not measurably throttle UDP/443; if they did, HTTP/3 would not be at a third of traffic. Targeted DPI exists (the 2024 IRBlock work found it in Iran), but is not the developer-on-hotel-wifi case.[^10]

For the locked-down-network edge: QUIC sits on UDP/443, the same port Google Meet and Zoom use. If UDP/443 is filtered, outbound SSH/22 is also blocked, and the SSH proposal needs its own fallback. Our relay (§4) doubles as that fallback, capable of TCP/443 if required.

---

## 6. mTLS as the membership layer

SSH advocate's flow: generate Ed25519 keypair, copy to peer's `authorized_keys`, trust on first use, glue in a hand-rolled `LinkManager`. mTLS equivalent: generate self-signed Ed25519 cert (TLS 1.3 supports it natively), SPKI fingerprint *is* the peer ID, each peer's trust store is a list of fingerprints. Pairing UX identical — QR code, scan, fingerprints exchanged. Underneath, **TLS 1.3 has had a decade of adversarial analysis; the SSH + Noise XX + hand-rolled glue stack has not.**[^11]

"mTLS needs a CA" is a deployment myth. RFC 8705's `self_signed_tls_client_auth` exists precisely so endpoints can pin client certs directly without a CA path. The trust anchor *is* the fingerprint, configured at pairing time, no CA in the chain.

Key rotation is *simpler* than SSH's: gossip the new fingerprint via the already-decided CRDT layer, peers update their accept-list at the next anti-entropy round, old cert is rejected once all peers have observed the update. With SSH, rotating a key means editing files on every peer.

---

## 7. 0-RTT and Android Doze: honest scoping

**0-RTT.** RFC 9001 §9.2: 0-RTT data is replayable; applications doing stateful work on 0-RTT requests get burned.[^12] An *approval* for `rm -rf` is the textbook unsafe-to-replay payload. **We disable 0-RTT entirely.** The 1-RTT handshake is still faster than SSH's multi-stage kex+auth+channel (TLS 1.3+QUIC: 1 RTT to data; SSH: 2–3 RTT minimum). Correct security posture at zero cost.

**Android Doze.** Doze suspends all app-initiated network I/O during sleep, on UDP and TCP equally.[^13] Concession: migration does not save you when the phone is in Doze for 8 hours. It saves the **30-second seam** — the wifi→cellular handoff during an active approval, the cellular→wifi reassociation when you walk in the door, the CGNAT mapping refresh mid-session. For background wakeup we use FCM as a *doorbell only* (not a payload channel — R6 holds): FCM wakes Termux, Termux re-establishes the QUIC connection. **Doze is an OS problem, not a transport problem; no transport choice fixes it, and we do not pretend ours does.**

---

## 8. Concrete migration sketch: `internal/transport/`

```
internal/
  transport/
    transport.go        // type Transport interface — Dial, Listen, Close
    quic/
      listener.go       // wraps *quic.Listener, mTLS-authenticated accept
      dialer.go         // holds *quic.Transport (one shared UDP socket)
      conn.go           // wraps quic.Connection; exposes Streams + Migrate(addr)
      stream.go         // per-message framing (length-prefixed CBOR)
      migration.go      // AddPath / Probe / Switch logic
      mtls.go           // self-signed cert gen, fingerprint pin, peer trust store
    relay/
      client.go         // VPS relay client (STUN-lite register + fallback forward)
      server.go         // 200-LOC relay binary (cmd/agenthive-relay)
  membership/
    peer.go             // type Peer: ID = SPKI fingerprint, Addrs []netip.AddrPort
    pairing.go          // QR pairing ceremony, fingerprint exchange
    trust.go            // PeerTrustStore: map[FingerprintID]*x509.Certificate
```

Key types and where they sit:

- `transport.Conn` wraps a `quic.Connection`, exposes `OpenStream() (Stream, error)` for the gossip layer and `Migrate(newLocalAddr netip.AddrPort) error` for the network-change handler.
- `transport.quic.Dialer` holds **one** `*quic.Transport` per process (one UDP socket, many connections — quic-go's recommended pattern). When the OS reports a network change (Android `ConnectivityManager` via gomobile, Linux netlink, macOS `nw_path_monitor`), the dialer calls `Migrate` on every active connection.
- mTLS configured via `tls.Config{Certificates: [...], VerifyPeerCertificate: pinByFingerprint}`. The pin check rejects any cert whose SPKI hash is not in the trust store.
- Stream multiplexing replaces the SSH advocate's hand-rolled JSON-over-stdio framer. Each logical message gets its own QUIC stream; a dropped packet stalls only *that* stream, not the gossip channel.[^1]

Total surface: ~800 LOC Go, no subprocess management, no PATH dependency, one optional relay binary. The autossh-subprocess design is ~1500 LOC of Go just to *manage subprocesses* plus an external `autossh` and `ssh` on every host including Termux.

---

## 9. Scoring axes

| Axis | This proposal | Status quo (autossh) |
|---|---|---|
| NAT traversal | ~70% direct via punch[^8], 100% via relay | tunnel dies at every network change; needs reachable sshd |
| Mobile battery / Doze | UDP under Doze = TCP under Doze; migration handles the seam[^13] | TCP dies at the seam, reconnect storm |
| Topology symmetry (R3) | every peer = same listener+dialer | inherently asymmetric: someone needs reachable sshd, or central host |
| Infra footprint | one optional VPS relay (~200 LOC) | central tunnel host (violates R3), or sshd on every peer |
| Ops complexity | cert = ID, pinned by fingerprint, rotation via CRDT | hand-rolled key distribution, `authorized_keys` editing |
| Security maturity | TLS 1.3 + QUIC, decade of analysis[^1][^11] | SSH + Noise XX + hand-rolled glue (untested) |
| Library maturity (2026) | quic-go: Cloudflare, libp2p, Syncthing, Caddy[^6] | autossh = OpenSSH wrapper, hand-framed JSON |
| Dep-tree | `golang.org/x/*`, quic-go | OpenSSH binary, autossh, hand-rolled framer |
| Scalability at 10+ peers | one UDP socket multiplexing all peers, native streams | N subprocesses, N tunnels, N×N full-mesh |
| Handshake | TLS 1.3, 1-RTT, 0-RTT disabled | SSH kex/auth/channel: 2–3 RTT |
| **Connection migration** | **Native, RFC 9000 §9** | **None. Tunnel dies on IP change.** |

---

## 10. Weakest claims (honest)

- **The relay still exists.** R6 permits it; honest cost is some ops surface. Mitigation: ~200 LOC, stateless, untrusted (E2E encrypted).
- **quic-go is still 0.x.** v1.0 unannounced as of mid-2025. Most of the "API breakage" reputation is the v0.53 overhaul; changes since are incremental. Pin a minor version, accept the ~1-day upgrade cost twice a year. Cheaper than maintaining a subprocess supervisor.
- **Server-side migration support is patchy on the public internet.**[^4] Irrelevant — we own both ends — but a reviewer will Google it.
- **Doze still wins for long sleeps.** Migration is the wifi→cellular case, not airplane-mode-overnight. FCM is the doorbell. No transport fixes Doze.
- **Termux gomobile/JNI glue isn't free.** Bridging OS network-change callbacks into the Go transport is ~100 LOC of Termux-specific code. Real engineering.

---

## 11. Sources

[^1]: RFC 9000 — QUIC: A UDP-Based Multiplexed and Secure Transport. <https://www.rfc-editor.org/rfc/rfc9000.html>
[^2]: quic-go documentation, *Connection Migration*. <https://quic-go.net/docs/quic/connection-migration/>
[^3]: Same as [^2] — documents the `AddPath` / `Probe` / `Switch` API and unsolicited-rebinding behaviour.
[^4]: *An Analysis of QUIC Connection Migration in the Wild*, ACM SIGCOMM CCR / arXiv 2410.06066. <https://arxiv.org/html/2410.06066v1>
[^5]: quic-go GitHub releases (v0.59.1 May 2025; v0.53 overhaul June 2024). <https://github.com/quic-go/quic-go/releases>
[^6]: quic-go README — production users list. <https://github.com/quic-go/quic-go>
[^7]: *draft-seemann-quic-nat-traversal-02* (Seemann, Kinnear, March 2024, expired). <https://datatracker.ietf.org/doc/draft-seemann-quic-nat-traversal/>
[^8]: *Challenging Tribal Knowledge — Large Scale Measurement Campaign on Decentralized NAT Traversal* (arXiv 2510.27500): 70% ± 7.1% across 4.4M attempts; TCP and QUIC statistically indistinguishable. <https://arxiv.org/html/2510.27500v1>
[^9]: Cloudflare Radar 2025 Year in Review summary (HTTP/3 ~21% → 35% over 2025). <https://blog.cloudflare.com/radar-2025-year-in-review/>; adoption trend page <https://radar.cloudflare.com/adoption-and-usage>
[^10]: *Now There's a Way to Measure QUIC Targeting by Middleboxes* (Internet Society Pulse / IRBlock). <https://pulse.internetsociety.org/blog/now-theres-a-way-to-measure-quic-targeting-by-middleboxes>
[^11]: RFC 9001 — Using TLS to Secure QUIC. <https://www.rfc-editor.org/rfc/rfc9001.html>
[^12]: RFC 9001 §9.2 — replay properties of 0-RTT data. <https://www.rfc-editor.org/rfc/rfc9001.html#section-9.2>
[^13]: Android Developers — *Optimize for Doze and App Standby*. <https://developer.android.com/training/monitoring-device-state/doze-standby>
