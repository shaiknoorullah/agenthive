# Security Model

agenthive is a low-blast-radius tool: it has access to whichever AI agents you wire it into, and to whatever permissions those agents have on each device. The threat model is correspondingly scoped.

## What's trusted

1. The `~/.config/agenthive/identity.key` file on each peer. Mode 0600, owned by the user running the daemon. Loss is treated as full peer compromise.
2. The set of PeerIDs in the local `peers` map (the CRDT allow-list). Anyone in this set can publish state and request actions.
3. The local OS, kernel, and file system.
4. The Go runtime and the audited libp2p stack underneath (Noise XX, Yamux, QUIC, GossipSub).

## What's encrypted

| Channel | Mechanism |
|---|---|
| Every libp2p connection (TCP and QUIC) | Noise XX handshake with ChaCha20-Poly1305 frames |
| Relay paths (Circuit Relay v2) | End-to-end Noise — the relay sees ciphertext only |
| Identity key on disk | Filesystem perms (mode 0600); not encrypted at rest in v0.1.0 |
| State file on disk | Filesystem perms (mode 0600); not encrypted at rest in v0.1.0 |
| Unix socket IPC (hook ↔ daemon) | Filesystem perms (mode 0600); no encryption in v0.1.0 |

Disk-at-rest encryption for `identity.key` and `state.json` (via OS keychain integration on macOS / Linux Secret Service) is on the roadmap. Until then, full-disk encryption on the host (FileVault, LUKS, etc.) is the right defense.

## What's authenticated

| Operation | How |
|---|---|
| New connection between peers | Noise XX requires private-key proof for the PeerID embedded in the multiaddr. Mismatch → handshake fails. |
| GossipSub message acceptance | Sender's PeerID must be in the local CRDT `peers` map. Unknown senders → message dropped at validator. |
| Hook subcommand → daemon | Unix-domain socket with mode 0600 (only the owning user can connect) |
| TUI → daemon | Same Unix socket; same protection |

Per-message GossipSub signature verification (so a compromised peer's authority is limited to its own key) ships in v0.2.0. In v0.1.0, GossipSub's built-in libp2p signing is on but not enforced at the validator — meaning compromise of any allow-listed peer's key is full mesh compromise.

## What an attacker without secrets can do

| Capability | What they get |
|---|---|
| Sniff network traffic between two peers | Ciphertext only. Cannot read or modify messages. |
| Replay a captured ciphertext | Noise rejects replay (per-message nonces) |
| Connect to a peer's libp2p port | Handshake fails unless they hold a private key matching an allow-listed PeerID |
| Spoof multiaddrs in an mDNS response on a LAN | The peer they advertise must still pass the Noise handshake to be useful — same `identity.key` required |
| Run a parallel libp2p node on the same IP | Different PeerID; gets rejected at the GossipSub validator |
| Read agenthive's local files via another OS user | `0600` / `0700` perms block this on a sound POSIX OS |

## What an attacker *can* observe (without secrets)

- Two PeerIDs are exchanging encrypted bytes between two IPs. libp2p does not hide that — no onion routing.
- Approximate message sizes and timing (GossipSub does not pad).
- That an agenthive node is running on a given host (TCP/QUIC port open + Noise handshake start).

If you need to hide the existence of the conversation itself, route agenthive over Tor or WireGuard. That's outside agenthive's scope.

## What an attacker *with* `identity.key` can do

- Impersonate that peer to the mesh. They can publish GossipSub state, accept action gate responses, dial other peers.
- Read whatever the local user can read on the host where the key was stolen.

## Mitigations against key compromise

If you suspect a peer's identity key is exfiltrated:

1. On any healthy peer, remove the compromised PeerID from the CRDT peers map:
   ```bash
   agenthive peers del 12D3KooWCompromised...
   ```
   The tombstone propagates via GossipSub. Subsequent messages from that key get dropped at every peer's validator.

2. Generate a new identity on the affected device:
   ```bash
   mv ~/.config/agenthive/identity.key ~/.config/agenthive/identity.key.compromised
   agenthive init
   agenthive id
   ```

3. Add the new PeerID to the CRDT on a healthy peer:
   ```bash
   agenthive peers add /ip4/.../tcp/.../p2p/12D3KooWNew...
   ```

4. Rotate any external secrets the compromised host had access to. agenthive itself doesn't store secrets beyond the keypair, but the host had whatever you were running there.

## Denial of service

| Vector | Mitigation |
|---|---|
| Connection-flood from outside the allow-list | libp2p `ResourceManager` caps inbound connection budget; new connections are evicted gracefully |
| GossipSub spam from a compromised allow-listed peer | v0.1.0: no rate limit. v0.2.0: per-peer rate limits in the validator |
| Action gate flood (rapid pending file creation) | Each action consumes ~1 KB of disk per pending file; sweeper deletes after TTL |
| File-descriptor exhaustion via many open streams | libp2p `ResourceManager` caps streams per peer |

## Action gate authorization model

- Anyone in the CRDT `peers` allow-list can write `<id>.response` for any pending action.
- The action gate does **not** verify that the responder is the "right" peer (e.g., authorized for this specific tool). All allow-list peers are trusted equally.

If you need per-action authorization (e.g., destructive actions require approval from a specific device), that's not in v0.1.0. Open an issue; it's a roadmap candidate.

## Reporting a vulnerability

Email **security@shaiknoorullah.dev** or open a private security advisory on GitHub. See [SECURITY.md](https://github.com/shaiknoorullah/agenthive/blob/main/SECURITY.md) for the response SLA.

**Do not open a public GitHub issue for security vulnerabilities.**

## See also

- [[Action Gate]] — internals
- [[CRDT State Sync]] — validator boundary
- [[NAT Traversal]] — what relay traffic looks like end-to-end
