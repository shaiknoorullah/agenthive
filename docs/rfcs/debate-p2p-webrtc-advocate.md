# Position Paper: P2P / WebRTC Transport for the Bidirectional Agent Control Plane

**Author:** P2P / WebRTC Approach Advocate
**Date:** 2026-03-26
**Status:** RFC / Technical Debate Position
**Target:** tmux-agent-notifications cross-machine notification relay (Feature #12)

---

## Executive Summary

The tmux-agent-notifications plugin has, until now, addressed only local notification delivery: an AI agent finishes work in a tmux pane and the status bar updates. But developers increasingly run agents on powerful remote servers while working from laptops, phones, and tablets. Feature #12 (Cross-Machine Notification Relay) must solve a hard networking problem: delivering bidirectional messages between remote servers and local devices with no cloud service dependency, strong encryption, NAT traversal, mobile device support, and resilience to intermittent connectivity.

SSH tunnels appear attractive at first glance -- SSH is ubiquitous, encrypted, and already familiar. But SSH was designed as a client-initiated, session-oriented protocol for interactive terminal access. Repurposing it as a persistent bidirectional message bus introduces fundamental architectural friction: reverse tunnel fragility, no native mobile support, single-point-of-failure topology, and the inability to operate when the user is away from their terminal without a dedicated always-on relay host.

A peer-to-peer transport based on WebRTC data channels (with libp2p as an alternative for non-browser peers) provides a purpose-built solution. Every device -- remote server, laptop, phone, tablet -- runs a lightweight peer node. Peers discover each other through a one-time pairing ceremony, establish direct encrypted connections with automatic NAT traversal, and exchange messages on a pub/sub channel. Notifications flow from remote server to all subscribed devices. Action responses (allow/deny for permission prompts) flow back from any device to the originating server. No cloud service routes traffic. No intermediary holds messages. No single point of failure exists. And the phone in your pocket becomes a first-class participant through a browser-based progressive web app that speaks WebRTC natively.

This paper argues that P2P/WebRTC is the correct transport layer for the tmux-agent-notifications control plane, and provides concrete architecture, protocol design, and a decision matrix comparing it against SSH tunnels.

---

## Architecture Diagram

```
                         ┌─────────────────────────────────────┐
                         │         Self-Hosted STUN/TURN       │
                         │    (optional, for hostile NATs)      │
                         │         coturn on your VPS           │
                         └──────────┬──────────┬───────────────┘
                                    │          │
                        ICE cand.   │          │  ICE cand.
                                    │          │
  ┌──────────────────┐              │          │           ┌──────────────────┐
  │  Remote Server A │◄─────────────┘          └──────────►│  Remote Server B │
  │  (peer node)     │                                     │  (peer node)     │
  │                  │         Direct DTLS/SCTP             │                  │
  │  tmux + agents   │◄═══════════════════════════════════►│  tmux + agents   │
  │  Claude, Codex   │         (data channels)             │  Claude, Codex   │
  └────────┬─────────┘                                     └────────┬─────────┘
           │                                                        │
           │  Direct DTLS/SCTP          Direct DTLS/SCTP            │
           │  (data channel)            (data channel)              │
           │                                                        │
           ▼                                                        ▼
  ┌──────────────────┐                                     ┌──────────────────┐
  │  Laptop          │         Direct DTLS/SCTP            │  Phone (PWA)     │
  │  (peer node)     │◄═══════════════════════════════════►│  (browser peer)  │
  │                  │         (data channel)               │                  │
  │  Desktop notifs  │                                     │  Push + action   │
  │  tmux client     │                                     │  buttons         │
  └──────────────────┘                                     └──────────────────┘

  ═══  WebRTC data channel (DTLS 1.3 encrypted, SCTP reliable/ordered)
  ───  ICE candidate exchange (one-time or reconnect signaling)

  Pairing: QR code / shared secret / manual SDP exchange
  Discovery: mDNS on LAN, pre-shared peer IDs for WAN
  Encryption: DTLS 1.3 with per-peer Ed25519 identity keys
  Protocol: JSON messages on labeled data channels ("notifications", "actions")
```

---

## 1. Why P2P Is Architecturally Correct for This Problem

### 1.1 The Problem Is Inherently Peer-to-Peer

The notification relay connects N remote servers to M user devices. There is no natural "server" in this topology. A remote build machine is a peer that produces notifications. A laptop is a peer that consumes them and produces action responses. A phone is a peer that does both. Forcing this into a client-server model (as SSH does) requires designating one side as the "server" and maintaining persistent connections from all others to it. This creates a single point of failure and an asymmetric architecture that does not match the symmetric nature of the communication.

A P2P mesh naturally expresses this topology. Each device is a peer. Each peer can both send and receive. Adding a new remote server means adding a peer. Adding a phone means adding a peer. No device is privileged. No device is a bottleneck.

### 1.2 No Cloud Dependency, No Intermediary

The constraint is explicit: no cloud services for message routing. WebRTC data channels, once established, carry traffic directly between peers. No message ever passes through a third-party server. The only infrastructure component is a STUN server for NAT traversal, which can be self-hosted (coturn is a single Docker container) and only handles the initial ICE candidate exchange -- it never sees message content. For peers on the same LAN, even STUN is unnecessary; mDNS discovery establishes connections with zero external infrastructure.

By contrast, an SSH-based approach that does not use a cloud relay must have one endpoint reachable from the other. If both the remote server and the phone are behind NAT (the common case), neither can initiate a connection to the other without a relay. SSH has no built-in NAT traversal. You must either expose a port (security risk), use a cloud-hosted bastion/jump host (violates the no-cloud constraint), or set up a VPN (a heavyweight dependency). WebRTC's ICE framework solves this with a single lightweight STUN query.

### 1.3 True End-to-End Encryption

WebRTC data channels use DTLS 1.3 for encryption, negotiated directly between peers. The STUN/TURN server never possesses the session keys. This is stronger than SSH in a relay/bastion topology, where the bastion host can inspect traffic if compromised. In the P2P model, there is no intermediate host that could be compromised. The encryption is literally end-to-end: the remote server encrypts, the phone decrypts, and nothing in between has the keys.

Each peer generates an Ed25519 identity key pair at setup time. Peer identities are verified during the pairing ceremony (see Section 3). After pairing, DTLS certificate fingerprints are pinned, preventing man-in-the-middle attacks even if a STUN server is compromised.

---

## 2. Technology Selection: WebRTC Data Channels as Primary, libp2p as Alternative

### 2.1 WebRTC Data Channels

WebRTC was designed for exactly this class of problem: establishing direct, encrypted, NAT-traversing connections between two endpoints. While most people associate WebRTC with video calling, its data channel component is a general-purpose reliable/unreliable messaging layer built on SCTP over DTLS.

**Advantages for this use case:**

- **Browser-native on phones.** A progressive web app (PWA) on the user's phone can open WebRTC data channels using the browser's built-in WebRTC stack. No native app installation required. This is a decisive advantage for mobile support.
- **NAT traversal via ICE/STUN/TURN** is built into the protocol. The ICE framework tries direct connection, then STUN-assisted connection, then TURN relay as a fallback. Self-hosted coturn provides both STUN and TURN.
- **DTLS 1.3 encryption** is mandatory in the spec. There is no unencrypted mode.
- **Reliable and ordered delivery** via SCTP. Notifications are small JSON messages; we do not need the complexity of TCP but we do need reliability. SCTP provides this over UDP.
- **Mature implementations in every language.** pion/webrtc (Go), webrtc-rs (Rust), node-webrtc / libdatachannel (Node.js/C++). The tmux plugin's peer daemon can be written in Go (pion is the most battle-tested non-browser WebRTC stack) and compiled to a single static binary for easy distribution.

**Message overhead:** A WebRTC data channel message carrying a 200-byte JSON notification payload requires approximately 80 bytes of SCTP/DTLS/UDP headers. Total: approximately 280 bytes per notification. Compare this to SSH, which has similar per-packet overhead but also requires a persistent TCP connection with keepalive traffic.

### 2.2 libp2p as an Alternative

libp2p is a modular networking stack used by IPFS, Ethereum 2.0, and Filecoin. It provides peer identity, NAT traversal (AutoNAT, circuit relay), encrypted transport (Noise protocol or TLS 1.3), and pub/sub messaging (GossipSub).

**Advantages:**

- Peer identity is a first-class primitive (Ed25519 key pair = peer ID).
- GossipSub provides pub/sub messaging out of the box.
- Circuit relay v2 provides NAT traversal without STUN/TURN (peers relay for each other).
- Implementations in Go (go-libp2p), Rust (rust-libp2p), and JavaScript (js-libp2p).

**Disadvantages compared to WebRTC for this use case:**

- No browser-native support for the full libp2p stack. js-libp2p runs in browsers but requires WebRTC transport underneath for NAT traversal, adding a layer of indirection.
- Heavier runtime footprint. The full libp2p stack includes DHT, relay, muxer, and other components not needed for a simple notification relay.
- Smaller community of non-blockchain users. The documentation and examples are heavily oriented toward decentralized storage and blockchain use cases.

**Recommendation:** Use WebRTC data channels as the primary transport with pion (Go) for the peer daemon. Use libp2p only if the project later needs multi-hop relay or DHT-based peer discovery for more complex topologies.

### 2.3 Noise Protocol Framework

The Noise protocol (used by WireGuard, Lightning Network, and libp2p) is an excellent encryption framework but is a building block, not a complete transport. It provides handshake patterns and encrypted framing but does not provide NAT traversal, connection management, or multiplexing. Using Noise directly would mean implementing a custom transport protocol from scratch. Since WebRTC's DTLS provides equivalent security with NAT traversal included, Noise is not recommended as a standalone choice. However, if libp2p is selected, its Noise transport is preferred over TLS for its simplicity and performance.

### 2.4 WireGuard Userspace

WireGuard provides encrypted point-to-point tunnels with excellent performance. However, it operates at the IP layer (Layer 3), which is more than we need for application-level messaging. It requires elevated privileges for tunnel setup on most platforms, does not work in browsers, and does not provide NAT traversal (peers must have at least one publicly reachable endpoint). WireGuard is the right tool for VPN access, not for application-level pub/sub messaging.

---

## 3. Peer Pairing and Discovery

### 3.1 Initial Pairing Ceremony

When a user sets up the notification relay for the first time, they must pair their devices. This is a one-time process per device pair. Three methods are supported, in order of convenience:

**Method 1: QR Code (Recommended for Phone)**

1. The remote server's peer daemon generates a pairing payload: `{ "peer_id": "<Ed25519 public key>", "stun": "stun:your-stun.example.com:3478", "signal": "<one-time signaling URL or direct IP:port>" }`.
2. The payload is encoded as a QR code and displayed in the terminal (`qrencode -t ANSI256`).
3. The user scans the QR code with their phone's camera. The PWA opens, decodes the payload, and initiates a WebRTC connection.
4. The DTLS handshake verifies both peers' identity keys. If the fingerprints match the pairing payload, the connection is established and the peer IDs are stored for future reconnection.

**Method 2: Shared Secret (For Headless Servers)**

1. Both peers are configured with a shared secret (a 256-bit random string, displayed as a base32 word sequence for human readability).
2. Each peer derives its signaling channel from the secret (e.g., a pre-shared rendezvous point on the self-hosted signaling server, or a direct IP:port encrypted with the shared secret).
3. The DTLS handshake uses the shared secret as a pre-shared key (PSK) for mutual authentication.

**Method 3: Manual SDP Exchange (For Fully Offline Pairing)**

1. Peer A generates an SDP offer and displays it as a base64 blob.
2. The user copies the blob to Peer B (via any channel: email, paste, file).
3. Peer B generates an SDP answer and displays it.
4. The user copies the answer back to Peer A.
5. The connection is established. Peer IDs are exchanged and stored.

After initial pairing, peers store each other's identity keys. Subsequent connections use these stored keys for authentication without repeating the pairing ceremony.

### 3.2 LAN Discovery via mDNS

For devices on the same local network (e.g., laptop and remote server on the same office network), mDNS (multicast DNS, aka Bonjour/Avahi) provides zero-configuration discovery. Each peer daemon advertises itself as `_tmux-agent._tcp.local.` with its peer ID in the TXT record. Peers that discover each other on mDNS establish direct LAN connections without any STUN/TURN involvement.

### 3.3 WAN Reconnection

When a peer's IP address changes (laptop moves between Wi-Fi networks, mobile device switches from Wi-Fi to cellular), the WebRTC ICE agent automatically performs ICE restart. The peer daemon detects the connection drop (SCTP heartbeat failure), generates new ICE candidates, and re-establishes the data channel using the stored peer identity keys for authentication. No user intervention is required.

For cases where ICE restart cannot find a path (both peers behind symmetric NAT with no STUN), the self-hosted TURN server provides a relay fallback. TURN traffic is still DTLS-encrypted end-to-end; the TURN server relays opaque encrypted packets.

---

## 4. Concrete Message Flow Examples

### 4.1 Notification: Remote tmux to Phone

```
1. Claude Code in tmux pane %42 on remote-server-A fires the Stop hook.
2. claude-hook.sh calls tmux_alert(), which:
   a. Sets per-pane tmux options (@notif-msg, @notif-project, etc.)
   b. Publishes a JSON message to the peer daemon's Unix socket:
      {
        "type": "notification",
        "server": "remote-server-A",
        "pane": "%42",
        "project": "api-server",
        "source": "Claude",
        "message": "Agent has finished",
        "timestamp": "2026-03-26T14:32:01Z",
        "priority": "info",
        "actions": []
      }
3. The peer daemon serializes the JSON and sends it on the "notifications"
   data channel to all connected peers (laptop, phone).
4. The phone's PWA receives the message via its WebRTC data channel,
   deserializes it, and:
   a. Shows a browser push notification: "Claude/api-server: Agent has finished"
   b. Updates the PWA's notification list UI.
5. Total latency: typically 50-200ms (ICE path + DTLS + SCTP).
```

### 4.2 Action Response: Phone to Remote tmux

```
1. Claude Code fires a Notification hook with a permission request:
      {
        "type": "notification",
        "server": "remote-server-A",
        "pane": "%42",
        "project": "api-server",
        "source": "Claude",
        "message": "Permission requested: delete database migration files",
        "priority": "critical",
        "actions": ["allow", "deny"]
      }
2. The phone's PWA shows a push notification with action buttons.
3. The user taps "Allow".
4. The PWA sends a JSON message on the "actions" data channel:
      {
        "type": "action_response",
        "server": "remote-server-A",
        "pane": "%42",
        "action": "allow",
        "responder": "phone-peer-id",
        "timestamp": "2026-03-26T14:32:15Z"
      }
5. The peer daemon on remote-server-A receives the message, verifies the
   responder's peer ID is authorized, and:
   a. Writes the response to a named pipe or file that the agent hook monitors.
   b. Or invokes tmux send-keys to type the response into the pane.
6. The agent proceeds with the allowed action.
```

### 4.3 Multiple Remote Servers to One User

```
  remote-server-A ──(data channel)──► laptop peer daemon
  remote-server-B ──(data channel)──►     │
  remote-server-C ──(data channel)──►     │
                                          ├──► desktop notifications
                                          └──(data channel)──► phone PWA

Each remote server is an independent peer. The laptop's peer daemon maintains
a data channel to each server. Notifications from all servers are aggregated
locally. The phone connects to the laptop peer (or directly to each server,
depending on configuration).

Adding server D: run `tmux-agent-peer pair --qr` on server D, scan QR on
laptop. One command, one scan, done. No SSH key distribution, no port
forwarding, no firewall rules.
```

---

## 5. Mobile Device Support

### 5.1 The Browser Is the Universal Mobile Runtime

SSH has no native mobile story. There is no SSH client built into iOS Safari or Android Chrome. Users must install a dedicated SSH app (Termius, Blink, JuiceSSH), configure host entries, manage keys, and maintain persistent SSH sessions that drain battery. Even then, SSH provides a terminal, not a notification interface. Receiving a notification over SSH means running a TUI on the phone, which is hostile to the mobile interaction model.

WebRTC is built into every modern mobile browser. A progressive web app (PWA) that uses WebRTC data channels runs in Safari, Chrome, and Firefox on both iOS and Android with zero installation. The PWA can:

- Receive notifications via the Push API (for when the browser is backgrounded).
- Display rich notification cards with action buttons (allow/deny).
- Maintain a persistent data channel while the app is in the foreground.
- Use a Service Worker to handle incoming messages when backgrounded.
- Be added to the home screen and behave like a native app.

This means the user's phone becomes a first-class participant in the agent control plane with zero app store installation. The entire mobile client is a static HTML/JS bundle served by the laptop's peer daemon (or hosted on any static file server).

### 5.2 Battery and Network Efficiency

WebRTC's ICE agent uses STUN keepalives at configurable intervals (default: 15-30 seconds). These are single UDP packets (approximately 28 bytes). Compare this to an SSH connection, which requires TCP keepalives (40 bytes) and typically also application-level keepalives to prevent NAT timeout. The UDP-based nature of WebRTC is kinder to mobile radios, which can batch UDP sends with other traffic.

When the PWA is backgrounded, the Service Worker can close the data channel and rely on the Push API for wake-up. The peer daemon on the remote server sends a push notification via a self-hosted web push relay (e.g., ntfy.sh self-hosted, or direct Web Push Protocol to the browser's push service). This is the one area where a minimal external service is involved, but it is the browser vendor's standard push infrastructure, not a custom cloud dependency.

---

## 6. Addressing SSH Tunnel Strengths

The SSH advocate will argue several strong points. Here are direct responses:

### "SSH is already installed everywhere"

True, but irrelevant for mobile devices (the hardest constraint). SSH is ubiquitous on servers and laptops, but the notification relay must reach phones. Installing an SSH app on a phone, configuring key authentication, and maintaining a persistent SSH session from a phone to a remote server is a poor user experience that drains battery and breaks whenever the phone sleeps or switches networks.

For the server-to-laptop path, SSH is indeed already available. But "available" is not "optimal." SSH tunnels require explicit port forwarding setup, must be restarted when they break, and do not handle IP address changes (laptop moving between networks). WebRTC handles all of these automatically via ICE.

### "SSH has battle-tested authentication"

True, and the P2P approach does not discard this strength. The pairing ceremony serves the same purpose as SSH key exchange: establishing mutual trust between two endpoints. Ed25519 keys are the same algorithm used by modern SSH. The DTLS handshake provides the same cryptographic guarantees as the SSH transport layer. The difference is that WebRTC's authentication is integrated with its NAT traversal, while SSH's authentication assumes an already-established TCP connection (which requires the server to be directly reachable).

### "SSH is simple: one command to set up a tunnel"

`ssh -R 9999:localhost:9999 user@remote` is indeed one command. But it is one command that:

1. Requires the remote server to be directly reachable from the laptop (no NAT traversal).
2. Must be restarted when the connection drops (autossh helps but adds complexity).
3. Does not work from a phone without a dedicated SSH app.
4. Creates a single point of failure (the SSH connection itself).
5. Does not support multiple remote servers without multiple tunnels.
6. Requires SSH key distribution to every device (including the phone).

The P2P pairing ceremony is also a one-time setup, but after pairing, connections are automatic and self-healing. There is no equivalent of "my SSH tunnel died and I did not notice for 2 hours."

### "SSH reverse tunnels solve the NAT problem"

SSH reverse tunnels (`ssh -R`) allow a remote server behind NAT to expose a port on the client. But this requires the client (laptop) to be reachable by the server, or requires a publicly reachable bastion host. If both the server and the laptop are behind NAT, SSH cannot establish a connection in either direction without a third-party intermediary. WebRTC's ICE framework handles this case natively through STUN-mediated hole-punching, succeeding in approximately 85-90% of NAT configurations without any relay.

### "SSH is already there; why add another daemon?"

A fair point. The P2P approach does require running a peer daemon (a single Go binary, approximately 10-15 MB). But the SSH approach also requires additional infrastructure beyond bare SSH: autossh or a systemd service for keepalive, a message broker or custom protocol on top of the tunnel, a web server or socket listener on each end, and some mechanism for the phone to connect (which SSH cannot provide). The total infrastructure footprint is comparable, but the P2P approach packages it into a single purpose-built binary rather than a collection of shell scripts wrapping SSH.

---

## 7. Risks and Mitigations

### Risk 1: Complexity of WebRTC Stack

**Concern:** WebRTC is a complex protocol suite (ICE, STUN, TURN, DTLS, SCTP). Implementing a peer daemon introduces a large dependency.

**Mitigation:** We do not implement WebRTC from scratch. The pion/webrtc library (Go) is a complete, production-grade WebRTC stack in approximately 50,000 lines of well-tested Go code. It compiles to a single static binary with zero runtime dependencies. The peer daemon is a thin layer on top of pion: accept pairing configuration, establish data channels, forward messages to/from the tmux plugin via a Unix socket. The application-level code is approximately 500-1000 lines.

### Risk 2: STUN/TURN Server Requirement

**Concern:** NAT traversal may require a STUN/TURN server, which is an infrastructure dependency.

**Mitigation:** Three tiers of NAT traversal, each with decreasing infrastructure needs:

1. **LAN (mDNS):** Zero infrastructure. Peers discover each other via multicast DNS.
2. **WAN with cooperative NAT (STUN):** A STUN server is a stateless UDP reflector. Public STUN servers exist (stun.l.google.com:19302), but for the no-cloud-dependency constraint, self-hosting coturn takes one `docker run` command. STUN handles approximately 85-90% of NAT configurations.
3. **WAN with symmetric NAT (TURN):** The same coturn instance provides TURN relay. Traffic remains DTLS-encrypted end-to-end.

For users who truly want zero external infrastructure, the manual SDP exchange pairing method (Method 3) works with no STUN/TURN at all, provided at least one peer has a publicly reachable IP.

### Risk 3: Mobile Browser Backgrounding

**Concern:** iOS Safari and Android Chrome aggressively suspend background tabs and Service Workers, which may cause missed notifications.

**Mitigation:** The PWA uses the Web Push API for backgrounded notification delivery. When the data channel detects disconnection (browser backgrounded), the peer daemon falls back to sending a push notification via the standard Web Push Protocol. The user sees a system notification and can tap it to open the PWA and respond. This is the same mechanism used by every web-based chat application (Slack, Discord, WhatsApp Web). The data channel reconnects automatically when the PWA is foregrounded.

### Risk 4: Binary Distribution

**Concern:** The peer daemon is a compiled Go binary, which is more complex to distribute than shell scripts.

**Mitigation:** Go cross-compiles trivially. The build produces static binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64. Distribution options:

- Download from GitHub releases (like fzf, ripgrep, and other tmux plugin dependencies).
- `go install github.com/kaiiserni/tmux-agent-peer@latest` for users with Go installed.
- The tmux plugin's install script can download the correct binary automatically.
- Homebrew formula for macOS users.

### Risk 5: Security of Self-Hosted STUN/TURN

**Concern:** A self-hosted STUN/TURN server is an internet-facing service that must be maintained and secured.

**Mitigation:** STUN is stateless and handles no sensitive data (it reflects public IP addresses). TURN relays opaque DTLS-encrypted blobs; even a compromised TURN server cannot read message content. coturn supports DTLS transport for its own client connections. The TURN server can be locked down to only accept connections from known peer IDs (authenticated via long-term credentials derived from the pairing secret). For users who do not want to run any server, the LAN-only mode (mDNS) and manual SDP exchange provide fully serverless alternatives.

### Risk 6: Firewall Restrictions in Corporate Environments

**Concern:** Some corporate networks block UDP traffic or restrict outbound connections to port 443 only.

**Mitigation:** WebRTC ICE candidates include TCP candidates on port 443 as a fallback. The TURN server can listen on TCP 443, appearing as a standard HTTPS connection to corporate firewalls. pion/webrtc supports ICE over TCP. This is the same technique used by WebRTC-based video conferencing tools (Google Meet, Zoom web client) that work behind corporate firewalls.

---

## 8. Resource Usage Comparison

| Metric | SSH Tunnel | WebRTC Data Channel |
|--------|-----------|-------------------|
| **Transport protocol** | TCP | UDP (SCTP over DTLS over UDP) |
| **Connection setup** | TCP 3-way handshake + SSH handshake (2-4 RTT) | ICE connectivity checks + DTLS handshake (1-3 RTT) |
| **Keepalive overhead** | TCP keepalive (40B) + SSH keepalive (60-100B) every 15-60s | STUN binding indication (28B) every 15-30s |
| **Per-message overhead** | SSH channel header (20-40B) + TCP (20B) + IP (20B) | SCTP (16B) + DTLS (13-29B) + UDP (8B) + IP (20B) |
| **NAT traversal** | None (requires port forwarding or bastion) | Built-in (ICE/STUN/TURN) |
| **Reconnection** | Manual (autossh) or systemd restart; re-authenticates | Automatic ICE restart; cached DTLS session resumption |
| **Memory footprint** | SSH client process: 5-15 MB RSS | Peer daemon: 15-25 MB RSS (includes pion stack) |
| **Mobile support** | Requires dedicated app; battery-intensive | Browser-native; PWA with push notifications |
| **CPU at idle** | Negligible (TCP keepalive) | Negligible (UDP keepalive) |
| **Connections for N servers** | N SSH tunnels, N TCP connections | N data channels, multiplexed over fewer UDP flows |

The peer daemon is slightly heavier than an SSH client in memory, but it replaces the SSH client, autossh, a message broker, and a signaling mechanism -- all of which would be needed in an SSH-based architecture.

---

## 9. Decision Matrix

| Criterion | SSH Tunnel | P2P / WebRTC | Winner |
|-----------|-----------|-------------|--------|
| **NAT traversal** | None built-in; requires port forwarding or bastion host | ICE/STUN/TURN; works through 85-90% of NATs automatically | P2P |
| **Mobile device support** | Requires dedicated SSH app; no notification UI | Browser-native PWA; push notifications; action buttons | P2P |
| **No cloud dependency** | Requires bastion host if both peers behind NAT | Self-hosted STUN or mDNS; no cloud for message routing | P2P |
| **End-to-end encryption** | Strong (SSH transport), but bastion can inspect if used | Strong (DTLS 1.3); no intermediary has keys | P2P |
| **Bidirectional messaging** | Requires custom protocol over tunnel | Native data channels with labeled message types | P2P |
| **Reconnection/resilience** | autossh + systemd; fragile across network changes | ICE restart; automatic; handles IP changes | P2P |
| **Multiple remote servers** | N separate tunnels; N configurations | N peers; uniform pairing; single daemon | P2P |
| **Setup simplicity** | One SSH command per server (if reachable) | One pairing ceremony per device (QR/secret/SDP) | Tie |
| **Familiarity/ubiquity** | Universal on servers; well-understood | Less familiar; requires learning new concepts | SSH |
| **Existing infrastructure** | SSH keys, sshd, authorized_keys already in place | New peer daemon binary; optional STUN server | SSH |
| **Auth infrastructure** | SSH keys, certificates, agent forwarding | Ed25519 keys, DTLS certificates, pairing ceremony | Tie |
| **Single point of failure** | The SSH tunnel itself; or the bastion host | No single point; mesh topology | P2P |
| **Bandwidth efficiency** | TCP + SSH framing | UDP + SCTP + DTLS (slightly more header bytes) | SSH |
| **Firewall friendliness** | TCP 22 (often allowed) | UDP (sometimes blocked); TCP 443 fallback available | Tie |
| **Battery life (mobile)** | TCP keepalive + SSH keepalive; app must stay open | UDP keepalive + Push API for background; lower drain | P2P |

**Score: P2P/WebRTC 9, SSH Tunnel 3, Tie 3**

The SSH tunnel approach wins on familiarity, existing infrastructure reuse, and raw bandwidth efficiency for small messages. These are real advantages. But the P2P approach wins on every constraint that is specific to this problem: NAT traversal, mobile support, no cloud dependency, resilience, and multi-server topology. The SSH wins are "nice to have"; the P2P wins are "must have."

---

## 10. Implementation Sketch

### 10.1 Peer Daemon (`tmux-agent-peer`)

A single Go binary using pion/webrtc. Approximately 1000 lines of application code on top of pion.

**Responsibilities:**

- Listen on a Unix socket (`/tmp/tmux-agent-peer.sock`) for messages from `tmux_alert()`.
- Maintain WebRTC data channels to all paired peers.
- Perform ICE/STUN/TURN for NAT traversal.
- Handle peer pairing (QR code generation, shared secret, SDP exchange).
- Persist peer identity keys and paired peer list in `~/.config/tmux-agent-peer/`.
- mDNS advertisement and discovery for LAN peers.
- Forward incoming notifications to a local Unix socket (for `tmux_alert()` to read) or invoke tmux commands directly.

**Lifecycle:** Started by the tmux plugin at init (`run-shell "tmux-agent-peer daemon &"`). Runs as a background process alongside the tmux server. Exits when tmux exits.

### 10.2 Integration with tmux-agent-notifications

The existing `tmux_alert()` function gains a single line:

```bash
# After setting per-pane tmux options (native approach from Phase 1):
echo "$json_payload" | socat - UNIX-CONNECT:/tmp/tmux-agent-peer.sock 2>/dev/null || true
```

This is a fire-and-forget write to the peer daemon's Unix socket. If the daemon is not running (relay not configured), the `socat` fails silently and the notification is still displayed locally via the native tmux approach. The relay is purely additive; the plugin works identically without it.

### 10.3 Phone PWA

A static HTML/JS/CSS bundle (approximately 5-10 KB gzipped) that:

1. Opens a WebRTC data channel to the laptop or remote server peer.
2. Renders incoming notifications as cards with timestamps and source labels.
3. Shows action buttons (Allow/Deny) for permission-request notifications.
4. Registers a Service Worker for background push notifications.
5. Can be served by the peer daemon itself on a local HTTPS port, or hosted on any static file server.

---

## 11. Conclusion

The notification relay problem has five hard constraints: no cloud dependency, NAT traversal, mobile device support, bidirectional messaging, and multi-server topology. SSH tunnels address one of these well (encryption) and struggle with the rest. WebRTC data channels address all five as first-class concerns.

The P2P approach requires a compiled peer daemon and a one-time pairing ceremony -- real costs that are not zero. But these costs are paid once, and the resulting system is self-healing, cloud-free, mobile-native, and architecturally matched to the problem's symmetric peer topology.

SSH is the right tool for getting a shell on a remote machine. WebRTC is the right tool for establishing a persistent, encrypted, NAT-traversing message channel between arbitrary devices. This problem calls for the latter.

The strongest argument for SSH -- that it is already there -- is the same argument that was once made for FTP, Telnet, and CGI. "Already there" is a valid consideration, not a sufficient architecture. When the constraints of the problem align with a purpose-built protocol, the purpose-built protocol wins.
