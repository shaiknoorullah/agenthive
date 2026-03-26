# Judgment: SSH Reverse Tunnel vs. P2P/WebRTC Transport for Distributed Agent Notification Mesh

**Judge:** Impartial technical evaluation
**Date:** 2026-03-26
**Status:** Final verdict

---

## 1. Fact-Check Results

Key claims from both position papers were verified against actual documentation, web research, and technical specifications. Claims are evaluated as TRUE, PARTIALLY TRUE, MISLEADING, or FALSE.

| # | Claim | Source | Verdict | Evidence |
|---|-------|--------|---------|----------|
| 1 | "SSH reverse tunnels guarantee NAT traversal. 100% reliability." | SSH, Section 1.3, 7.1 | **PARTIALLY TRUE** | SSH reverse tunnels guarantee traversal *when the remote server can make an outbound TCP connection to the local relay machine*. But this requires the local relay to have a reachable IP (public IP, port forward, or VPN). If BOTH peers are behind NAT and neither has a reachable address, SSH cannot establish a connection in either direction without a third-party intermediary. The "100%" figure in the decision matrix is only true when the local relay is reachable -- which is an assumption, not a guarantee. |
| 2 | "STUN only works for cone NAT. Symmetric NAT defeats STUN." | SSH, Section 7.1 | **TRUE** | STUN reflects the external IP:port mapping, but symmetric NAT creates different mappings for different destinations. The mapping learned from STUN cannot be used for peer connections. Research confirms STUN has approximately 80% overall success rate. Approximately 10-20% of connections require TURN fallback. |
| 3 | "WebRTC requires STUN servers minimum, TURN for reliability." | SSH, Section 1.1 | **TRUE** | WebRTC's ICE framework requires at minimum a STUN server for NAT traversal beyond the LAN. For symmetric NAT (common on corporate/mobile networks), TURN relay is required. Without TURN, approximately 10-20% of users cannot connect. |
| 4 | "autossh has a 20+ year track record of production use." | SSH, Section 1.4 | **TRUE** | autossh was authored by Carson Harding and has been maintained since 2004. It is widely used in IoT, monitoring, and infrastructure automation. Multiple production deployment guides and systemd integration patterns confirm active production use. |
| 5 | "An idle SSH tunnel produces ~24 KB/hour of keepalive traffic." | SSH, Section 5.1 | **TRUE** | With ServerAliveInterval 30, a keepalive packet (~200 bytes) is sent every 30 seconds. 200 bytes * 120 packets/hour = 24,000 bytes/hour. Math checks out. |
| 6 | "WebRTC memory footprint: 30-50 MB for data-only." | SSH, Section 9 | **MISLEADING** | The SSH advocate inflates this. The WebRTC advocate cites 15-25 MB for a pion-based peer daemon, which is more accurate. pion/webrtc is a pure Go implementation with no CGo dependencies, compiling to a single binary. 15-25 MB RSS is realistic for a Go process with the pion stack. 30-50 MB is an upper bound that would apply to a browser-based WebRTC stack, not a purpose-built Go daemon. |
| 7 | "A PWA on the phone can open WebRTC data channels using the browser's built-in WebRTC stack. No native app installation required." | P2P, Section 5.1 | **PARTIALLY TRUE** | WebRTC data channels work in mobile browsers when the tab is in the foreground. However, iOS Safari severely limits background WebRTC: backgrounding pauses media streams, Service Workers have restricted functionality, and push notification action buttons are limited or absent on iOS. The PWA must rely on Web Push API for backgrounded delivery, and iOS Web Push (available since iOS 16.4) has documented reliability issues, especially after device restarts. The claim overstates the seamlessness of the mobile browser experience. |
| 8 | "SSH has no native mobile story." | P2P, Section 5.1 | **MISLEADING (in light of R1)** | The P2P advocate assumes mobile means "phone browser." Requirement R1 introduces Termux as the mobile peer. Termux can run sshd on port 8022 without root, can run SSH clients, and supports the full OpenSSH toolchain. Termux transforms the "no mobile SSH" argument because the phone IS a terminal peer, not a browser client. |
| 9 | "WebRTC data channels use DTLS 1.3 for encryption." | P2P, Section 1.3 | **MISLEADING** | As of 2026, WebRTC implementations predominantly use DTLS 1.2, not 1.3. The WebRTC spec references DTLS 1.2. DTLS 1.3 is still under development (RFC 9147 published but adoption is gradual). pion/webrtc uses DTLS 1.2. The security is still strong, but the "1.3" claim is inaccurate for current implementations. |
| 10 | "SSH tunnel + push notification (ntfy/Pushover) delivers to mobile natively via the OS push notification system." | SSH, Section 7.3 | **TRUE, but contradicts R6** | This is technically correct, but the user's R6 requirement explicitly states "No ntfy.sh, no Pushover, no AWS, no third-party relay." The SSH advocate's mobile story depends on exactly the services R6 forbids. With R1 (Termux as peer), this dependency is eliminated -- the phone runs the same relay daemon directly. |
| 11 | "The peer daemon is a single Go binary, approximately 10-15 MB." | P2P, Section 6 | **TRUE** | pion/webrtc compiles to static binaries with no runtime dependencies. A minimal Go binary using pion for data channels would be 10-15 MB. Go cross-compilation to linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 is trivial. |
| 12 | "ICE restart handles IP address changes automatically." | P2P, Section 3.3 | **PARTIALLY TRUE** | ICE restart is a defined mechanism in the WebRTC spec, and pion supports it. However, ICE restart requires re-signaling (exchanging new ICE candidates), which means the signaling channel must survive the IP change. If signaling is over the same data channel that broke, a separate signaling fallback is needed. The claim oversimplifies the reconnection story. |
| 13 | "mDNS provides zero-configuration LAN discovery." | P2P, Section 3.2 | **TRUE on Linux/macOS, PROBLEMATIC on Android/Termux** | mDNS works well on Linux (Avahi) and macOS (Bonjour). However, research reveals that Android/Termux has significant mDNS limitations: Avahi is not available in default Termux repos, and Android's multicast support is inconsistent. mDNS advertisement from Termux "never successfully advertises" according to community reports. This directly impacts R1 (Termux as peer). |
| 14 | "60% of residential NAT devices deploy symmetric NAT." | Search results | **UNVERIFIED, LIKELY OVERSTATED** | One search result cited this figure, but it conflicts with other data showing STUN succeeds ~80% of the time and only 10-20% of connections need TURN. If 60% of residential NATs were symmetric, STUN success rates would be much lower. The actual figure is likely lower -- symmetric NAT is more common on mobile carriers and corporate networks than residential. |
| 15 | "WebRTC's data channel is massively overengineered for 200-byte JSON messages." | SSH, Section 7.2 | **TRUE** | WebRTC's ICE/STUN/TURN/DTLS/SCTP stack is designed for real-time media with strict latency requirements. For infrequent small JSON messages, the protocol overhead (ICE candidate gathering, STUN binding, DTLS handshake, SCTP association) is disproportionate to the payload. However, "overengineered" does not mean "wrong" -- it means there may be a simpler alternative that provides the same guarantees. |

---

## 2. Requirements Alignment

The advocates wrote their papers WITHOUT knowledge of requirements R1-R6. This section evaluates how well each approach supports the actual requirements.

### R1: Termux as First-Class Peer (Android)

| Aspect | SSH Approach | P2P/WebRTC Approach |
|--------|-------------|-------------------|
| Can Termux run the daemon? | **Yes.** Termux has full OpenSSH (client and sshd on port 8022). autossh can be installed via `pkg install autossh`. The relay-sender/receiver bash scripts run natively in Termux. | **Partially.** Go cross-compiles to linux/arm64, and Termux supports Go binaries. pion/webrtc can run in Termux. However, mDNS (Avahi) is NOT available in Termux repos, and Android multicast is unreliable. LAN discovery breaks. |
| termux-notification integration | **Excellent.** The relay daemon dispatches to `termux-notification --button1 "Allow" --button1-action "relay-send allow:req-42"`. Action buttons execute shell commands directly. No browser, no PWA. | **Possible but indirect.** The peer daemon would need to invoke `termux-notification` for local display and parse button actions. This is not part of the WebRTC stack -- it is custom integration. |
| Same daemon as server/laptop? | **Yes.** The relay-sender bash script runs identically on any POSIX system with bash, socat, and jq. Termux provides all of these. | **Yes.** A single Go binary compiled for linux/arm64 runs in Termux. |
| NAT traversal from phone | **Requires the phone to reach the relay OR the relay to reach the phone.** If the phone is on cellular (behind carrier-grade NAT), it cannot host an sshd reachable from the server. It must initiate outbound SSH to a reachable relay. | **ICE/STUN handles this** -- the phone's peer daemon can traverse most NATs via STUN. Falls back to TURN for carrier-grade NAT. |
| Battery impact | **Moderate.** An SSH connection with 30s keepalive maintains a persistent TCP connection. `termux-wake-lock` prevents Android from killing it, but this keeps the CPU awake. | **Moderate.** UDP keepalives are slightly more battery-friendly than TCP keepalives, but the difference is marginal for 30s intervals. |

**R1 Verdict: SSH has a significant advantage** because Termux is fundamentally a terminal environment. SSH is Termux's native language. The relay-sender/receiver scripts, autossh, socat, jq -- all run out of the box. The P2P approach requires compiling a Go binary for arm64 (feasible but adds a build step) and loses mDNS discovery on Android. The `termux-notification` integration is equally achievable from both approaches, but SSH requires less custom code.

### R2: Granular Notification Routing Rules

| Aspect | SSH Approach | P2P/WebRTC Approach |
|--------|-------------|-------------------|
| Routing at source | The relay-sender can filter before transmission based on local rules. | The peer daemon can filter before sending on data channels. |
| Routing at destination | The relay-receiver can route based on JSON fields (agent, project, session, priority). | Each peer can subscribe to specific labeled data channels or filter incoming messages. |
| Rule granularity | JSON protocol already includes `server`, `pane`, `project`, `source`, `priority` fields. Routing rules match these fields. | Same -- JSON messages carry the same metadata for routing. |
| Configuration distribution | **Not addressed.** The SSH paper does not discuss how routing rules are synced between devices. The relay-receiver holds the routing config locally. | **Not addressed.** The P2P paper does not discuss routing config sync. Each peer would need a local copy. |

**R2 Verdict: Tie.** Both approaches can implement granular routing. Neither paper addresses distributed routing config (R4). The routing engine is an application-layer concern, independent of transport.

### R3: Symmetric Distributed Device Management

| Aspect | SSH Approach | P2P/WebRTC Approach |
|--------|-------------|-------------------|
| Topology | **Star/hub-spoke.** The SSH approach designates one machine as the "local relay" that all remote servers tunnel to. This is inherently asymmetric. | **Mesh.** Every peer is equal. No designated hub. This is inherently symmetric. |
| Any device can manage? | **No, not natively.** The relay-receiver is the central dispatch point. Managing from the phone would require the phone to have access to the relay-receiver's configuration. | **Yes, architecturally.** Each peer can broadcast management commands (add peer, edit route, view metrics) on a dedicated data channel. All peers receive and apply. |
| TUI on every device | **Yes.** A TUI can run in tmux (server/laptop) and Termux (phone). | **Yes.** Same TUI binary runs everywhere. |
| Adding/removing peers | Adding a remote server requires configuring a new SSH tunnel to the relay. Adding a phone requires... a third-party push service (pre-R1). With R1 (Termux), adding a phone requires SSH tunnel setup from Termux to the relay. | Adding any peer requires a pairing ceremony (QR/secret/SDP). After pairing, peers discover each other automatically. |

**R3 Verdict: P2P has a strong architectural advantage.** The SSH approach's hub-spoke topology fundamentally conflicts with R3's "no privileged hub node" requirement. Making SSH symmetric would require every peer to run sshd and every other peer to tunnel to it -- creating N*(N-1) tunnels in a mesh, which is impractical. The P2P mesh topology naturally expresses symmetric management.

### R4: Distributed Configuration State

| Aspect | SSH Approach | P2P/WebRTC Approach |
|--------|-------------|-------------------|
| State distribution mechanism | **Not addressed.** The SSH paper stores config locally on the relay-receiver. No sync mechanism. Adding one would require a separate protocol (file sync, rsync, custom IPC). | **Not directly addressed, but architecturally aligned.** Data channels can carry config sync messages. A CRDT or last-writer-wins register on the "config" data channel would provide eventual consistency. |
| Conflict resolution | Not discussed. | Not discussed, but CRDTs are a natural fit for the P2P model. Libraries like go-ds-crdt (IPFS) or custom LWW-registers are available. |
| Propagation latency | Would require push notification or polling. Not real-time without additional infrastructure. | Real-time over existing data channels. Config changes propagate in milliseconds. |

**R4 Verdict: P2P has a clear advantage.** Distributed state is a natural extension of the P2P mesh. Over SSH tunnels, distributing config requires building a separate sync mechanism on top of an architecture that was not designed for it. The hub-spoke model means config lives in one place and must be pulled/pushed to satellites -- exactly the centralization R4 prohibits.

### R5: PreToolUse Hook Discovery

| Aspect | SSH Approach | P2P/WebRTC Approach |
|--------|-------------|-------------------|
| Hook integration | PreToolUse hook writes pending action to filesystem, dispatches via relay-sender, polls for response file. The transport is irrelevant -- the hook always communicates via local files/sockets. | Identical. The PreToolUse hook writes to a local socket. The peer daemon handles transport. |
| Response pathway | Response arrives via SSH tunnel, written to response file by relay-receiver. Hook picks it up. | Response arrives via data channel, written to response file by peer daemon. Hook picks it up. |

**R5 Verdict: Tie.** The PreToolUse hook mechanism is entirely transport-agnostic. Both approaches use the same local IPC (file polling or Unix socket) between the hook and the transport daemon. R5 is an application-layer concern that does not favor either transport.

### R6: No Cloud Services

| Aspect | SSH Approach | P2P/WebRTC Approach |
|--------|-------------|-------------------|
| Message routing | **Fully self-hosted** -- SSH tunnel carries messages directly. No third party involved. | **Fully self-hosted for LAN** (mDNS + direct data channel). **Requires self-hosted STUN/TURN for WAN** -- coturn is self-hostable but is additional infrastructure. |
| Phone notifications | **The SSH paper relies on ntfy.sh or Pushover** -- both are cloud services. This VIOLATES R6. However, with R1 (Termux), the phone runs its own relay daemon and needs no push service. | **The P2P paper relies on a PWA with Web Push** -- which uses browser vendor push infrastructure (Google FCM, Apple APNs). This also arguably violates R6. With R1 (Termux), no push service needed. |
| Authentication | SSH keys (self-managed). | Ed25519 keys + DTLS certificates (self-managed). |
| Discovery | No external service needed (manual SSH config). | Requires STUN for WAN (can be self-hosted but is additional infrastructure). |

**R6 Verdict: SSH has an edge, but both have issues.** The SSH approach's core tunnel is fully self-hosted. The P2P approach requires a STUN/TURN server even if self-hosted -- that is still infrastructure to deploy and maintain. HOWEVER, both approaches originally relied on cloud push services for mobile (ntfy/Pushover for SSH, Web Push for P2P), and both are saved by R1 (Termux eliminates the need for push services entirely). With R1 in play, SSH is closer to zero-infrastructure, while P2P still needs STUN/TURN for WAN traversal.

### Requirements Alignment Summary

| Requirement | SSH | P2P/WebRTC | Winner |
|------------|-----|-----------|--------|
| R1: Termux as first-class peer | Native fit (SSH is Termux's home) | Feasible but loses mDNS, needs arm64 binary | **SSH** |
| R2: Granular routing rules | Application-layer concern | Application-layer concern | **Tie** |
| R3: Symmetric distributed management | Hub-spoke conflicts with symmetry | Mesh is inherently symmetric | **P2P** |
| R4: Distributed config state | Requires separate sync mechanism | Natural extension of data channels + CRDT | **P2P** |
| R5: PreToolUse hook integration | Transport-agnostic | Transport-agnostic | **Tie** |
| R6: No cloud services | Core is self-hosted; STUN not needed | Requires self-hosted STUN/TURN for WAN | **SSH** |

**Score: SSH 2, P2P 2, Tie 2.** The requirements split evenly between the approaches, but the NATURE of the wins differs. SSH wins on pragmatic deployment concerns (R1, R6). P2P wins on architectural fitness (R3, R4). This split is the key tension that drives the verdict.

---

## 3. Strongest Arguments

### SSH Tunnel: Top 3

**S1. Zero new infrastructure for the server-to-laptop path.**
Every developer who works on remote servers already has SSH configured: keys, `~/.ssh/config`, `sshd` running. The reverse tunnel requires zero new software, zero new accounts, and zero new trust relationships. This is not a theoretical advantage -- it is a deployment reality. The 15-line bash reconnection loop is genuinely sufficient for production use. autossh adds polish but is not required. Strength: very high.

**S2. Termux nativity (amplified by R1).**
Requirement R1 transforms the debate. The SSH advocate's weakest point (mobile support requiring ntfy/Pushover) is eliminated because the phone IS a terminal peer running the same relay daemon. SSH is Termux's native protocol. `pkg install openssh autossh socat jq` gives the phone everything it needs. No Go binary compilation, no arm64 cross-compilation, no mDNS workarounds. The phone runs `autossh -R` to the relay, receives notifications via the tunnel, and dispatches to `termux-notification` with action buttons. This is the simplest possible mobile integration. Strength: very high.

**S3. Proven reliability under adversity.**
SSH tunnels with autossh handle network outages, laptop sleep/wake, Wi-Fi switches, and VPN reconnections with a 20+ year track record. The reconnection behavior is well-understood: detect failure via ServerAliveInterval, exit, restart, re-authenticate, re-establish port forward. Queued messages on disk survive the outage. WebRTC ICE restart is newer, less battle-tested in non-browser contexts, and requires signaling channel availability for re-negotiation. For a system where reliability is more important than architecture purity, SSH's track record is decisive. Strength: high.

### P2P/WebRTC: Top 3

**P1. Mesh topology matches the symmetric management requirement (R3).**
R3 demands that "EVERY connected peer can see all peers, add/remove devices, edit routing rules." This is a mesh requirement. SSH's hub-spoke topology means one machine is the relay, and all others connect to it. Making the phone manage the server's routing would require the phone to SSH into the relay and modify its config -- possible but architecturally backwards. A P2P mesh naturally distributes management because every peer is equal. Strength: very high.

**P2. Distributed state is a natural extension of the mesh (R4).**
R4 requires routing rules, device registry, and mesh config to be distributed state with automatic propagation and conflict resolution. Over P2P data channels, this is straightforward: broadcast config changes on a "config" channel, use CRDTs for conflict resolution, and every peer converges. Over SSH tunnels, distributing config requires building a custom sync protocol on top of a point-to-point transport -- essentially re-inventing parts of the P2P layer. Strength: high.

**P3. NAT traversal for the double-NAT case.**
When both the remote server AND the user's laptop are behind NAT (e.g., corporate network + home network), SSH cannot establish a tunnel in either direction without a publicly reachable intermediary. WebRTC's ICE framework handles this via STUN hole-punching (succeeding ~80% of the time) or TURN relay (self-hosted). While R6 prefers no additional infrastructure, a self-hosted coturn is less invasive than a publicly reachable bastion host. Strength: moderate (diminished by R1's Termux approach, which assumes the phone can reach the relay).

### P2P/WebRTC: Weakest Claims

**W-P1. "The browser is the universal mobile runtime."**
This was the P2P paper's strongest argument before R1 was introduced. With Termux as the mobile peer, the browser is irrelevant. The phone runs a terminal daemon, not a PWA. iOS Safari's WebRTC limitations, Service Worker restrictions, and push notification unreliability are all moot. The entire Section 5 (Mobile Device Support) of the P2P paper is invalidated by R1.

**W-P2. "SSH reverse tunnels are fragile across network changes."**
The paper states: "There is no equivalent of 'my SSH tunnel died and I did not notice for 2 hours.'" But autossh with ServerAliveInterval 30 and ServerAliveCountMax 3 detects failure within 90 seconds and reconnects automatically. The "fragility" claim is overstated -- autossh is specifically designed to solve this problem and has been doing so for 20+ years.

**W-P3. The decision matrix scoring "P2P 9, SSH 3" is biased.**
Several categories are framed to favor P2P. "No cloud dependency" gives P2P the win, but P2P requires STUN/TURN (even if self-hosted), while SSH requires only sshd (already running). "Bidirectional messaging: Native data channels vs. requires custom protocol over tunnel" awards P2P, but newline-delimited JSON over a TCP socket is as "native" as JSON over SCTP -- both work fine for this workload. "Single point of failure: The SSH tunnel itself vs. No single point; mesh topology" awards P2P, but ICE connectivity to each peer is equally a point of failure -- if the STUN server is down and NAT mappings change, peers cannot reconnect.

### SSH Tunnel: Weakest Claims

**W-S1. "NAT traversal reliability: 100%."**
The decision matrix assigns SSH "100% NAT traversal reliability." This is only true if the local relay has a reachable IP address. If the developer's laptop is behind NAT and they have no home server with port forwarding, the reverse tunnel cannot be established without a cloud bastion -- violating R6. The 100% figure assumes ideal conditions.

**W-S2. "WebRTC memory: 30-50 MB."**
The paper cites 30-50 MB for "WebRTC stack, even for data-only." This applies to browser WebRTC runtimes, not to pion/webrtc in Go. A pion-based peer daemon uses 15-25 MB, comparable to SSH + autossh (10-12 MB). The inflated figure is misleading.

**W-S3. Mobile support via ntfy/Pushover violates R6.**
The SSH paper's entire mobile strategy (Section 4) depends on cloud push services: ntfy.sh, Pushover. R6 explicitly prohibits these. The SSH advocate did not know about R6, but the strategy collapses without it. R1 (Termux) rescues the SSH approach, but the paper itself does not propose the Termux solution.

---

## 4. Weakest Arguments (Misleading or Exaggerated)

### From SSH Advocate

1. **"WebRTC is a browser/VoIP technology" (Section 7.2).** This frames WebRTC as categorically wrong for server-side use. pion/webrtc is a production-grade server-side WebRTC implementation used by LiveKit, Janus, and other non-browser systems. WebRTC data channels are general-purpose, not browser-specific.

2. **"Score: SSH 11, WebRTC 1, Tie 5" (Section 9).** An 11-1 score in a debate between two viable approaches indicates scoring bias, not overwhelming superiority. Several "SSH wins" are marginal (bandwidth efficiency for 200-byte messages) or category-padding (separating "encryption" and "security track record" into two categories that both award SSH).

3. **"WebRTC requires a signaling server" (Section 7.2).** True for initial connection, but the paper implies this is an ongoing dependency. Signaling is one-time per peer pair (or per reconnect). Pre-shared peer configs eliminate signaling infrastructure entirely after initial pairing.

### From P2P/WebRTC Advocate

1. **"SSH was designed as a client-initiated, session-oriented protocol for interactive terminal access" (Executive Summary).** This frames SSH as fundamentally unsuited for persistent tunnels. SSH port forwarding (`-L`, `-R`) was designed specifically for persistent tunnels and has been used for this purpose since the 1990s. It is not a misuse of the protocol.

2. **"SSH has no built-in NAT traversal" (Section 1.2).** True in the strict sense, but SSH reverse tunnels (-R) ARE the NAT traversal mechanism. The remote server initiates outbound SSH (traversing its NAT), and the tunnel carries traffic back. This is not "no NAT traversal" -- it is a different NAT traversal strategy (outbound TCP initiation vs. UDP hole-punching).

3. **"The phone in your pocket becomes a first-class participant through a browser-based progressive web app" (Executive Summary).** As documented in the fact-check, iOS Safari has severe limitations for backgrounded WebRTC: paused connections, unreliable push events, limited action buttons. A PWA is NOT a first-class participant on iOS -- it is a second-class citizen with workarounds. R1's Termux approach is strictly superior for mobile participation.

---

## 5. Missed Arguments

Things neither paper raised:

### 5.1 The Termux + SSH combination eliminates the mobile gap entirely

Neither paper considered that the phone could run the SAME daemon as the server/laptop. The SSH paper assumed mobile = push notification service. The P2P paper assumed mobile = browser PWA. Requirement R1 changes everything: the phone runs Termux, which has full SSH, bash, socat, jq. The phone is not a consumer of notifications via a cloud service -- it is a full peer running the relay daemon. This eliminates the SSH approach's only real weakness (mobile support) and eliminates the P2P approach's strongest argument (browser-native mobile).

### 5.2 The CRDT question is architecture-critical

Requirement R4 (distributed config state) is not just a feature -- it determines the entire mesh coordination model. Neither paper discusses CRDTs, vector clocks, or conflict resolution for distributed routing rules. In a mesh of 3-5 peers where any peer can edit routes, concurrent edits will happen (user edits routes on phone while laptop auto-updates peer status). Without a conflict resolution strategy, the system will have split-brain routing. This is a hard distributed systems problem that neither transport layer solves on its own.

### 5.3 The "relay" machine could be any long-lived peer, not a dedicated server

The SSH paper assumes a dedicated "local relay machine" (laptop, Raspberry Pi, NAS). But in the Termux model, the phone itself could be the relay -- it is the device most likely to be always-on and always-connected. An Android phone with Termux running `sshd` on port 8022, accessible via Tailscale or a similar mesh VPN, could serve as the always-reachable relay that remote servers tunnel to. This inverts the typical laptop-as-relay model.

### 5.4 Tailscale/WireGuard as the transport layer

Neither paper seriously evaluated WireGuard userspace or Tailscale as the mesh transport. Tailscale provides: encrypted P2P connectivity with NAT traversal (DERP relays as fallback), stable IP addresses per device (regardless of physical network), automatic key management, and an API for device management. It runs on Linux, macOS, Android (including Termux), and iOS. If the user is willing to run Tailscale (self-hosted Headscale for R6 compliance), it provides the mesh connectivity layer without building custom transport. The notification daemon would simply use TCP/Unix sockets over the Tailscale network. This was not considered by either advocate.

### 5.5 The number of concurrent peers is small

Both papers implicitly optimize for scalability (many remote servers, many devices). In practice, the typical deployment is 1-3 remote servers, 1 laptop, 1 phone = 3-5 total peers. At this scale, N*(N-1) = 6-20 connections. Even the "impractical" full-mesh SSH approach (every peer tunnels to every other peer) is only 6-20 SSH connections, which is entirely manageable. The scalability arguments from both sides are largely theoretical for the actual use case.

### 5.6 Process supervision and lifecycle on Termux

Android aggressively kills background processes. Termux mitigates this with `termux-wake-lock`, but this keeps the CPU wake lock active and impacts battery. Neither paper discusses the Android-specific lifecycle challenges: Doze mode, battery optimization whitelisting, notification channel requirements for foreground services. The relay daemon on Termux needs to be a foreground service (via `termux-notification` with ongoing notification) to survive Android's process killer.

---

## 6. The Hybrid Question

### Can a hybrid approach work?

Yes, and the requirements DEMAND it. No single transport satisfies all six requirements. Here is why:

- **SSH excels at R1 (Termux), R6 (no cloud), and point-to-point reliability** -- but fails at R3 (symmetric management) and R4 (distributed state) because its topology is inherently hub-spoke.

- **P2P/WebRTC excels at R3 (symmetric mesh) and R4 (distributed state)** -- but adds infrastructure burden (STUN/TURN) that conflicts with R6, and its mobile story (PWA) is weaker than Termux's terminal story.

The insight is that **the transport layer and the coordination layer are separable concerns**:

1. **Transport** (how bytes move between peers): This can vary per link. SSH tunnel for server-to-relay. Direct TCP for LAN peers. WireGuard/Tailscale for phone-to-server over VPN.

2. **Coordination** (how peers discover each other, sync config, and manage the mesh): This must be symmetric and distributed, regardless of transport.

3. **Application protocol** (JSON messages for notifications, actions, config sync): This is transport-agnostic.

### The hybrid that satisfies all requirements

**Use the Noise Protocol Framework for transport encryption, with pluggable link transports, and a CRDT-based distributed state layer for coordination.**

But this is overengineered for 3-5 peers. A simpler hybrid:

**SSH tunnels as the primary link transport + a gossip-based config sync protocol running over those tunnels.**

- Every peer runs a daemon that speaks a simple JSON protocol.
- Links between peers use SSH tunnels (or direct TCP on LAN, or Tailscale for VPN).
- The daemon maintains a CRDT-based routing table and device registry.
- Config changes propagate via the existing message channels.
- Any peer can manage any aspect of the mesh because the config is replicated everywhere.

This preserves SSH's deployment simplicity and Termux nativity while adding the symmetric coordination layer that SSH alone cannot provide.

---

## 7. Verdict

### Recommendation: Custom Lightweight Mesh Daemon with SSH as Primary Link Transport

Neither pure SSH tunnels nor pure P2P/WebRTC is the correct answer. The correct answer is a **purpose-built mesh daemon** that:

1. Uses **SSH tunnels as the default link transport** for WAN connectivity (leveraging existing infrastructure, keys, and Termux compatibility).
2. Uses **direct TCP connections** for LAN peers (no SSH overhead needed when peers are on the same network).
3. Implements a **symmetric gossip protocol** for config sync, peer discovery, and mesh management.
4. Runs the **same binary/script on every peer** (server, laptop, phone/Termux).
5. Speaks a **JSON-over-newline-delimited-stream protocol** that is transport-agnostic.

This is not the SSH approach with patches. It is not the P2P approach with compromises. It is a third option that takes SSH's transport pragmatism and combines it with P2P's architectural symmetry.

### Top 3 Deciding Factors

1. **R1 (Termux as first-class peer) eliminates the mobile transport debate.** The phone is not a browser client -- it is a terminal peer. SSH is Termux's native protocol. This collapses the P2P advocate's strongest argument (browser-native mobile) and strengthens the SSH advocate's position on transport. But it does NOT resolve R3/R4, which require symmetric coordination.

2. **R3 + R4 (symmetric management + distributed config) require a coordination layer that SSH alone cannot provide.** SSH tunnels are point-to-point. Making every peer equal requires a protocol on top of the tunnels that handles config sync, peer registry, and management commands. This coordination layer is the core novelty -- the transport under it is secondary.

3. **R6 (no cloud services) favors SSH transport over WebRTC.** SSH tunnels require zero external infrastructure (assuming at least one reachable endpoint). WebRTC requires STUN/TURN even if self-hosted. For a tool that "maintains all connections" without third-party services, SSH's self-contained transport model is simpler.

### How the Winning Approach Handles Termux Peers

The Termux peer runs the identical mesh daemon as server and laptop peers. The setup process:

1. **Install dependencies:** `pkg install openssh autossh socat jq`
2. **Generate peer identity:** `mesh-daemon init` creates an Ed25519 key pair and a peer ID.
3. **Pair with another peer:** `mesh-daemon pair --remote user@server` exchanges peer IDs over an existing SSH connection.
4. **Establish link:** `mesh-daemon link --to server --via ssh` creates an autossh reverse tunnel from Termux to the server (or vice versa).
5. **Start daemon:** `mesh-daemon start` begins listening for messages and syncing config.
6. **Notification display:** Incoming action requests invoke `termux-notification --title "Claude/api" --content "Allow rm -rf?" --button1 "Allow" --button1-action "mesh-daemon respond allow:req-42"`.
7. **Foreground service:** The daemon creates a persistent Termux notification to survive Android's process killer.

The TUI management interface runs identically in Termux and tmux:

```
mesh-daemon tui
```

This opens a curses/terminal-based interface showing all peers, their status, routing rules, and pending actions. Edits propagate to all peers via the gossip protocol.

### How Distributed Config Sync Works

The mesh daemon maintains a local replicated state store using **Last-Writer-Wins Register (LWW-Register)** CRDTs:

1. **State components:**
   - `peers`: Map of peer_id -> {name, status, last_seen, address, link_type}
   - `routes`: Map of route_id -> {match: {agent, project, session, window, pane, priority}, target: [peer_ids], action: "notify"|"notify+action"}
   - `config`: Map of config_key -> {value, updated_at, updated_by}

2. **Conflict resolution:** Each state entry has a Hybrid Logical Clock (HLC) timestamp. On concurrent writes, the entry with the higher HLC wins. Ties are broken by peer_id (lexicographic). This is deterministic -- all peers converge to the same state without consensus rounds.

3. **Propagation:** When a peer updates its local state, it sends the delta to all connected peers as a JSON message on the "config_sync" channel. Peers merge the incoming delta with their local state using the LWW merge function.

4. **Full sync on reconnection:** When a link recovers after an outage, peers exchange full state snapshots and merge. The LWW merge is idempotent -- applying the same update twice is harmless.

5. **No leader election.** No consensus protocol. No Raft/Paxos. LWW-CRDTs provide eventual consistency without coordination, which is appropriate for configuration state where the latest edit should win.

### Conditions That Would Change the Verdict

- **If the user requires browser-based mobile access** (no Termux): P2P/WebRTC becomes necessary for the phone peer. The hybrid would add WebRTC data channels as an optional link transport for browser clients.
- **If the mesh grows beyond 10+ peers**: The simple gossip + LWW-CRDT model may need optimization (e.g., Merkle tree-based anti-entropy for efficient state sync). But for 3-5 peers, the simple approach is sufficient.
- **If Tailscale/Headscale is acceptable**: The entire transport layer becomes trivial. Every peer gets a stable Tailscale IP, and the daemon uses plain TCP sockets. No SSH tunnels, no STUN/TURN, no NAT traversal code. The daemon focuses purely on the application protocol and CRDT state sync.
- **If the user is on a network where SSH port 22 is blocked** (e.g., restrictive corporate firewall that only allows HTTPS): WebRTC's ability to tunnel over TCP 443 via TURN becomes valuable. SSH can also run on port 443, but this requires controlling the sshd configuration on the relay.
- **If Android battery life is a hard constraint**: The persistent SSH connection from Termux will drain battery. A hybrid that uses push notifications (ntfy self-hosted within the local network, NOT cloud) for phone-to-server wakeup could reduce battery drain. This would be an optional optimization, not a core architecture change.

### Concessions to the P2P/WebRTC Side

The P2P/WebRTC advocate got several important things right:

1. **The problem IS architecturally peer-to-peer.** R3 and R4 confirm this. A hub-spoke SSH topology is wrong for symmetric management. The verdict's coordination layer borrows the P2P paper's insight that every peer must be equal.

2. **NAT traversal is a real concern.** The SSH paper's "100% NAT traversal" claim assumes a reachable relay. For the double-NAT case (both peers behind NAT with no reachable endpoint), SSH offers no solution without a bastion. The P2P paper correctly identifies this gap.

3. **ICE/STUN is genuinely useful.** For users who cannot or will not configure port forwarding, STUN-assisted hole-punching is valuable. The recommended architecture should support STUN as an OPTIONAL link transport for environments where SSH tunnels are impractical.

4. **The single Go binary model is sound.** A compiled daemon with no runtime dependencies (vs. a collection of bash scripts) is more robust for cross-platform deployment. The verdict's daemon should be a single binary (Go or Rust) even though it uses SSH for transport.

5. **Ed25519 peer identity is the right abstraction.** Peers should have cryptographic identities independent of SSH keys. This allows the same identity across different link transports.

### Concessions to the SSH Tunnel Side

The SSH tunnel advocate got several important things right:

1. **SSH is the pragmatically correct transport for the primary use case.** Developer has SSH keys, sshd is running, autossh handles reconnection. No new infrastructure. For the server-to-laptop link, SSH is unbeatable.

2. **The "already there" argument is not just convenience -- it is security.** SSH's key management, host verification, and restricted key capabilities (no shell, no forwarding except specific ports) provide a security model that would take significant effort to replicate with custom crypto.

3. **autossh's reliability track record is genuine.** 20+ years of production use in IoT and infrastructure monitoring is not hand-waving. ICE restart in pion is newer and less battle-tested in non-browser production deployments.

4. **The message protocol design is correct.** Newline-delimited JSON with type/id/server/pane/project/source/priority/actions fields is the right application protocol. The verdict adopts this protocol verbatim.

5. **Disk-based message queuing for offline resilience** is important. When the link is down, notifications must not be lost. The SSH paper's queue-to-disk approach is simple and correct.

### Technology Stack Recommendation

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| **Daemon language** | Go | Single static binary, cross-compiles to linux/amd64, linux/arm64, darwin/amd64, darwin/arm64. Runs in Termux. Good concurrency primitives for managing multiple links. |
| **Primary link transport** | SSH (via `os/exec` calling `ssh`/`autossh`) | Leverage existing SSH infrastructure. No need to embed an SSH library -- fork the system SSH binary. |
| **LAN link transport** | Direct TCP (net.Listener / net.Dial) | No SSH overhead for same-network peers. |
| **Optional WAN transport** | STUN via pion/ice (optional module) | For environments where SSH is impractical. Not required for core functionality. |
| **Peer identity** | Ed25519 key pair (crypto/ed25519) | Same algorithm as SSH keys, but independent. Used for peer authentication regardless of link transport. |
| **Transport encryption (non-SSH links)** | Noise Protocol (flynn/noise or go-noise) | Lightweight, formally verified, used by WireGuard. Encrypts direct TCP links. SSH links are already encrypted. |
| **Config state sync** | LWW-Register CRDT with Hybrid Logical Clocks | Simple, well-understood, no coordination needed. Sufficient for config-sized state (kilobytes). |
| **Message serialization** | Newline-delimited JSON (encoding/json) | Human-readable, debuggable, sufficient for the message rates involved. |
| **TUI management** | bubbletea (charmbracelet/bubbletea) | Mature Go TUI framework. Renders identically in tmux and Termux. |
| **Termux notifications** | termux-notification (via os/exec) | Native Android notifications with action buttons. |
| **Desktop notifications** | notify-send (Linux), osascript (macOS) via os/exec | Standard platform notification tools. |
| **Process supervision** | systemd (Linux), launchd (macOS), Termux foreground service (Android) | Platform-appropriate process management. |
| **Hook integration** | PreToolUse bash hook -> Unix socket -> daemon | Transport-agnostic. Hook writes to local socket, daemon handles routing. |

---

## 8. Recommended Architecture

### 8.1 System Overview

```
                    THE MESH DAEMON: "relay-mesh"
                    Runs on EVERY peer (same binary)

  +============================================================+
  |                    REMOTE SERVER A                           |
  |  +------------------+  +------------------+                 |
  |  | Claude Code      |  | Codex CLI        |                 |
  |  | (tmux pane %42)  |  | (tmux pane %58)  |                 |
  |  +--------+---------+  +--------+---------+                 |
  |           |                      |                          |
  |           v                      v                          |
  |  +--------------------------------------------+            |
  |  | PreToolUse Hook (action-gate.sh)            |            |
  |  | Writes pending action, dispatches to daemon |            |
  |  +--------------------+-----------------------+             |
  |                       |                                     |
  |                       v (Unix socket)                       |
  |  +--------------------------------------------+            |
  |  |           relay-mesh daemon                 |            |
  |  |  - Peer ID: ed25519:abc123...               |            |
  |  |  - Links: SSH tunnel to laptop              |            |
  |  |           SSH tunnel to phone               |            |
  |  |  - CRDT state: routes, peers, config        |            |
  |  |  - Message queue (disk-backed)              |            |
  |  +----------+-----------------+---------------+             |
  |             |                 |                              |
  +============ | =============== | ============================+
                |                 |
     SSH tunnel |                 | SSH tunnel
    (autossh)   |                 | (autossh)
                |                 |
  +============ | ====+    +===== | ============================+
  |   LAPTOP    |     |    |     PHONE (Termux)                 |
  |             v     |    |      v                              |
  |  +------------+   |    |  +----------------------------------+
  |  | relay-mesh |   |    |  | relay-mesh daemon                |
  |  |  daemon    |   |    |  |  - Peer ID: ed25519:def456...    |
  |  |            |   |    |  |  - Links: SSH to server A        |
  |  |  Dispatches|   |    |  |  - Dispatches to:                |
  |  |  to:       |   |    |  |    termux-notification           |
  |  |  - tmux    |   |    |  |    (native Android push          |
  |  |    status  |   |    |  |     with action buttons)         |
  |  |  - desktop |   |    |  |                                  |
  |  |    notif   |   |    |  |  - TUI management (same as       |
  |  |  - TUI mgmt|   |    |  |    laptop, in Termux terminal)   |
  |  +------------+   |    |  +----------------------------------+
  +====================+    +====================================+


  LINK TYPES:
  ========  SSH reverse tunnel (encrypted, key-authenticated)
  --------  Direct TCP + Noise (LAN peers, no SSH overhead)
  ........  Optional STUN/pion (for environments without SSH)
```

### 8.2 Daemon Architecture

```
  +===============================================================+
  |                     relay-mesh daemon                           |
  |                                                                |
  |  +-----------------+    +------------------+                   |
  |  | Link Manager    |    | State Store      |                   |
  |  |                 |    | (CRDT)           |                   |
  |  | - SSH links     |    |                  |                   |
  |  | - TCP links     |    | peers: LWW-Map   |                   |
  |  | - (opt) STUN    |    | routes: LWW-Map  |                   |
  |  |                 |    | config: LWW-Map  |                   |
  |  | Multiplexes all |    |                  |                   |
  |  | links into one  |    | HLC timestamps   |                   |
  |  | message stream  |    | Merge on receive |                   |
  |  +---------+-------+    +--------+---------+                   |
  |            |                      |                            |
  |            v                      v                            |
  |  +--------------------------------------------+               |
  |  |            Message Router                   |               |
  |  |                                             |               |
  |  |  Inbound:                                   |               |
  |  |    notification -> local dispatch           |               |
  |  |    action_request -> local dispatch         |               |
  |  |    action_response -> write response file   |               |
  |  |    config_sync -> merge into State Store    |               |
  |  |    peer_announce -> update State Store      |               |
  |  |    heartbeat -> update peer liveness        |               |
  |  |                                             |               |
  |  |  Outbound:                                  |               |
  |  |    Apply routing rules from State Store     |               |
  |  |    Forward to matching peer links           |               |
  |  |    Queue to disk if link is down            |               |
  |  +-----+------------+------------+-------------+               |
  |        |            |            |                              |
  |        v            v            v                              |
  |  +---------+  +-----------+  +------------------+              |
  |  | Local   |  | Hook IPC  |  | Notification     |              |
  |  | Unix    |  | (action   |  | Dispatch         |              |
  |  | Socket  |  |  gate     |  |                  |              |
  |  | (input) |  |  files)   |  | - tmux status    |              |
  |  |         |  |           |  | - termux-notif   |              |
  |  | Receives|  | Pending/  |  | - notify-send    |              |
  |  | notifs  |  | response  |  | - osascript      |              |
  |  | from    |  | files for |  | - display-menu   |              |
  |  | hooks   |  | PreTool   |  | - display-popup  |              |
  |  |         |  | Use hook  |  |                  |              |
  |  +---------+  +-----------+  +------------------+              |
  |                                                                |
  |  +-------------------------------------------------+           |
  |  |              TUI Manager                        |           |
  |  |  (bubbletea)                                    |           |
  |  |                                                 |           |
  |  |  Tabs: [Peers] [Routes] [Actions] [Logs]        |           |
  |  |                                                 |           |
  |  |  Peers:                                         |           |
  |  |    server-A    online   12ms   3 agents         |           |
  |  |    laptop      online   <1ms   0 agents         |           |
  |  |    phone       online   45ms   0 agents         |           |
  |  |                                                 |           |
  |  |  Routes:                                        |           |
  |  |    priority:critical -> ALL                     |           |
  |  |    project:api       -> phone, laptop           |           |
  |  |    agent:*           -> laptop                  |           |
  |  |                                                 |           |
  |  |  [a]dd peer  [e]dit route  [d]elete  [q]uit    |           |
  |  +-------------------------------------------------+           |
  +================================================================+
```

### 8.3 Message Protocol

All messages are newline-delimited JSON with a `type` field. The protocol runs over any link transport.

```
Message Types:
  notification      - Agent finished/waiting notification
  action_request    - Permission request with action buttons
  action_response   - User's decision (allow/deny)
  config_sync       - CRDT state delta (routes, peers, config)
  peer_announce     - Peer identity and capability advertisement
  heartbeat         - Liveness signal with metrics
  peer_query        - Request full state from a peer (reconnection)
  peer_state        - Full state snapshot response
```

#### Config Sync Message

```json
{
  "type": "config_sync",
  "from_peer": "ed25519:abc123...",
  "hlc_timestamp": "2026-03-26T14:32:01.000Z:0001:abc123",
  "deltas": [
    {
      "collection": "routes",
      "key": "route-17",
      "value": {
        "match": {"project": "api-server", "priority": "critical"},
        "targets": ["phone", "laptop"],
        "action": "notify+action"
      },
      "hlc": "2026-03-26T14:32:01.000Z:0001:abc123",
      "deleted": false
    }
  ]
}
```

### 8.4 Link Establishment Flow (SSH)

```
  SERVER                                   LAPTOP
    |                                        |
    |  1. mesh-daemon pair --remote          |
    |     user@laptop                        |
    |  ---------------------------------->   |
    |     (SSH connection, exchanges         |
    |      peer IDs and capabilities)        |
    |                                        |
    |  2. Peer IDs stored in                 |
    |     ~/.config/relay-mesh/peers.json    |
    |                                        |
    |  3. mesh-daemon link --to laptop       |
    |     --via ssh                          |
    |                                        |
    |  4. autossh -M 0 -N                    |
    |     -R 19222:localhost:19222           |
    |     user@laptop                        |
    |  =================================>    |
    |     (persistent reverse tunnel)        |
    |                                        |
    |  5. JSON messages flow                 |
    |     bidirectionally over               |
    |     the tunnel                         |
    |  <================================>    |
    |                                        |
    |  6. Config sync: laptop sends          |
    |     its full CRDT state               |
    |  <----------------------------------   |
    |                                        |
    |  7. Server merges, sends deltas        |
    |  ---------------------------------->   |
    |                                        |
    |  Both peers now have identical          |
    |  routing tables, peer registries,       |
    |  and configuration.                     |
```

### 8.5 Notification Flow: Server to Phone (Termux)

```
  SERVER (tmux pane %42)          PHONE (Termux)
    |                               |
    | 1. Claude Code fires          |
    |    PreToolUse hook            |
    |                               |
    | 2. action-gate.sh writes      |
    |    pending action to          |
    |    Unix socket                |
    |                               |
    | 3. relay-mesh daemon          |
    |    evaluates routing rules    |
    |    from CRDT state:           |
    |    "priority:critical -> ALL" |
    |    matches -> forward to      |
    |    phone peer                 |
    |                               |
    | 4. Send via SSH tunnel        |
    |    to phone                   |
    | =============================>|
    |                               |
    |                       5. relay-mesh daemon
    |                          on phone receives
    |                          action_request
    |                               |
    |                       6. Dispatches to
    |                          termux-notification:
    |                               |
    |                          termux-notification \
    |                            --title "Claude/api" \
    |                            --content "rm -rf build/?" \
    |                            --button1 "Allow" \
    |                            --button1-action \
    |                              "relay-mesh respond \
    |                               allow:req-42" \
    |                            --button2 "Deny" \
    |                            --button2-action \
    |                              "relay-mesh respond \
    |                               deny:req-42"
    |                               |
    |                       7. User taps "Allow"
    |                          on Android notification
    |                               |
    |                       8. relay-mesh respond
    |                          writes action_response
    |                          to daemon socket
    |                               |
    | 9. action_response arrives   |
    |    via SSH tunnel            |
    |<============================  |
    |                               |
    | 10. Daemon writes             |
    |     response file:            |
    |     {action_id}.response      |
    |                               |
    | 11. action-gate.sh            |
    |     picks up response,        |
    |     returns JSON to           |
    |     Claude Code:              |
    |     {"permissionDecision":    |
    |      "allow"}                 |
    |                               |
    | 12. Claude Code proceeds      |
```

### 8.6 Distributed Config Edit Flow

```
  PHONE (Termux)              SERVER A              LAPTOP
    |                           |                     |
    | 1. User opens TUI:        |                     |
    |    relay-mesh tui          |                     |
    |                           |                     |
    | 2. User edits route:      |                     |
    |    "project:api ->        |                     |
    |     phone ONLY"           |                     |
    |                           |                     |
    | 3. Phone daemon updates   |                     |
    |    local CRDT:            |                     |
    |    routes["route-17"] =   |                     |
    |    {match:{project:api},  |                     |
    |     targets:[phone],      |                     |
    |     hlc: T1:phone}        |                     |
    |                           |                     |
    | 4. Phone sends            |                     |
    |    config_sync delta      |                     |
    |    to all connected       |                     |
    |    peers                  |                     |
    | =========================>|                     |
    | =============================================>  |
    |                           |                     |
    |                   5. Server merges       6. Laptop merges
    |                      delta into             delta into
    |                      local CRDT             local CRDT
    |                           |                     |
    |                   7. Both now route              |
    |                      project:api                |
    |                      notifications              |
    |                      to phone only              |
    |                           |                     |
    |  CONVERGENCE: All 3 peers have identical        |
    |  routing tables within milliseconds              |
```

### 8.7 File/Directory Layout

```
~/.config/relay-mesh/
  identity.json          # This peer's Ed25519 key pair + peer ID
  peers.json             # Known peers: {peer_id: {name, pubkey, link_config}}
  state.json             # Full CRDT state snapshot (routes, config, peer status)
  links/
    server-a.link        # Link config: {type: "ssh", remote: "user@server-a", port: 19222}
    laptop.link          # Link config: {type: "ssh", remote: "user@laptop", port: 19223}
    phone.link           # Link config: {type: "ssh", remote: "user@phone:8022", port: 19224}

~/.local/share/relay-mesh/
  queue/                 # Disk-backed message queue for offline resilience
    pending_*.json       # Queued outbound messages (delivered on reconnect)
  actions/               # PreToolUse hook IPC
    {action_id}.pending  # Pending action request
    {action_id}.response # User decision
  logs/
    events.log           # Structured event log (JSON lines)
    action-audit.log     # Action decision audit trail
```

### 8.8 CLI Interface

```
relay-mesh init                          # Generate peer identity
relay-mesh pair --remote user@host       # Pair with another peer via SSH
relay-mesh pair --qr                     # Display QR code for Termux pairing
relay-mesh pair --manual                 # Manual key exchange (copy-paste)

relay-mesh link --to <peer> --via ssh    # Establish SSH tunnel link
relay-mesh link --to <peer> --via tcp    # Establish direct TCP link (LAN)
relay-mesh link --to <peer> --via stun   # Establish STUN-assisted link (optional)

relay-mesh start                         # Start daemon (foreground)
relay-mesh start --daemon                # Start daemon (background)
relay-mesh stop                          # Stop daemon

relay-mesh status                        # Show all peers, links, routes
relay-mesh peers                         # List peers with status/latency
relay-mesh routes                        # List routing rules
relay-mesh routes add "project:api -> phone"     # Add routing rule
relay-mesh routes del route-17                    # Delete routing rule

relay-mesh respond allow:req-42          # Respond to action request
relay-mesh respond deny:req-42           # Deny action request

relay-mesh tui                           # Interactive TUI management

relay-mesh config get <key>              # Read config value
relay-mesh config set <key> <value>      # Set config value (propagates to all peers)
```

### 8.9 Implementation Phases

#### Phase 1: Core Daemon (Weeks 1-3)

- Peer identity generation and storage
- SSH link transport (using system `ssh`/`autossh` via `os/exec`)
- JSON message protocol (notification, action_request, action_response, heartbeat)
- Unix socket for hook integration
- PreToolUse hook script (action-gate.sh)
- Disk-backed message queue
- `termux-notification` dispatch for Termux peers
- `notify-send` / `osascript` dispatch for desktop peers
- `relay-mesh init`, `pair`, `link`, `start`, `status`, `respond` CLI commands

#### Phase 2: Distributed State (Weeks 4-5)

- LWW-Register CRDT implementation
- Hybrid Logical Clock
- Config sync protocol (config_sync messages)
- Route table with match/target evaluation
- `relay-mesh routes`, `config` CLI commands
- State persistence and recovery on restart
- Full state sync on link reconnection

#### Phase 3: TUI Management (Week 6)

- bubbletea-based TUI with Peers/Routes/Actions/Logs tabs
- Peer management (add, remove, view metrics)
- Route management (add, edit, delete)
- Live action request display with approve/deny
- `relay-mesh tui` command

#### Phase 4: Polish and Optional Features (Weeks 7-8)

- Direct TCP link transport for LAN peers (with Noise encryption)
- Optional STUN link transport (pion/ice, for environments without SSH)
- tmux display-menu/display-popup integration for in-terminal action buttons
- Log analytics and structured event logging
- Setup wizard: `relay-mesh setup` guides through init/pair/link
- Documentation and man pages

---

## Summary

The SSH advocate was right about transport. The P2P advocate was right about architecture. Neither was right about mobile (because neither knew about Termux).

The winning approach takes SSH's pragmatic, zero-infrastructure transport and wraps it in a symmetric mesh daemon that provides the distributed state coordination P2P promised. The phone is not a browser client receiving push notifications -- it is a full terminal peer running the same daemon, speaking the same protocol, managing the same mesh.

The transport is not glamorous. The coordination layer is not trivial. Together, they are correct.
