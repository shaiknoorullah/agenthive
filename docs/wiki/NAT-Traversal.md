# NAT Traversal

agenthive deliberately runs **no external infrastructure** for NAT punching — no STUN server, no TURN server, no rendezvous point. It uses go-libp2p's protocol stack to maximize the chance that any two of your own peers can reach each other directly. When direct fails, it relays through one of *your own* peers — never a third party.

## The five-path cascade

When peer A wants to talk to peer B, libp2p tries all of these in parallel and races them:

1. **Direct LAN dial** — mDNS-discovered, both peers on the same network. Same-LAN connections complete in milliseconds with zero config.
2. **Direct WAN dial** — B advertises a publicly reachable address. AutoNAT confirms reachability; the multiaddr propagates through the CRDT `peers` map; A dials it.
3. **UPnP / NAT-PMP-mapped port** — A or B sits behind a cooperative home router. agenthive calls `NATPortMap()` on startup; the router (silently) maps an external port to the peer's internal one. Failure is a no-op.
4. **DCUtR hole-punch** — both A and B are behind NAT. They coordinate a simultaneous TCP/QUIC dial through a common relay. 2025 ProbeLab measurements report **~70% ± 7% direct success across 4.4M attempts on 85k networks**.
5. **Circuit Relay v2 fallback** — DCUtR couldn't punch a hole. Traffic relays through a peer that has a publicly reachable address. Bytes are Noise-encrypted end-to-end; the relay sees ciphertext only.

First success wins. The connection upgrades as better paths become available.

## The "no infrastructure" trick

`EnableRelayService()` is on for every agenthive node. Whichever of *your* peers happens to have a publicly reachable address — a dev server with public IPv4, a cloud VPS, an IPv6-enabled cellular phone, a home box with port-forward — automatically picks up the relay role.

You are not "running a relay." You are running agenthive, which embeds a relay protocol that activates when reachable. No additional binary to deploy, no separate user to manage, no config to set.

The peer source for AutoRelay is fed from the CRDT `peers` map. When you add a peer, that peer is immediately a relay candidate for everyone else in the mesh.

## When NAT traversal cannot work

agenthive is honest about its limits. If *all* of these are true:

- Every peer is behind hostile NAT (CGNAT, symmetric NAT, etc.)
- No peer has a publicly reachable IPv6 address
- UPnP / NAT-PMP is disabled on every router
- No peer is on a shared LAN with any other peer
- You aren't willing to operate a single peer with a reachable address

…then peers cannot connect over the WAN. mDNS still works on a LAN.

In 2026 this case is rare — most cellular networks now expose IPv6, and a single $5/month VPS running agenthive (as a peer, not a separate "relay") solves it permanently.

## Corporate firewalls

If a firewall blocks UDP and non-443 TCP, QUIC dies and most TCP ports die. Listen on TCP 443 (in addition to the random ephemeral ports) to maximize the chance of escape. The release binary doesn't pin a specific port for you in v0.1.0; on the roadmap.

## DCUtR mechanics in 60 seconds

1. A and B both connect to relay R via Circuit Relay v2.
2. R relays a hole-punch request from A to B containing A's NAT-mapped address.
3. B does the same — relayed request back to A with B's mapped address.
4. A and B fire **simultaneous** TCP / QUIC connects at each other's reported addresses.
5. Both NATs see outbound packets first, so the inbound packet from the other side gets accepted as part of an established flow.
6. The direct connection succeeds; A and B drop the relayed path.

Symmetric NATs (where outbound source ports differ per destination) defeat step 5 — those are the 30% that fall back to relay.

## See also

- [[Architecture]] — where NAT-traversal protocols sit in the stack
- [[Configuration]] — UPnP / NAT-PMP, listen addresses, environment vars
- [[Security Model]] — relay traffic is end-to-end encrypted; the relay sees ciphertext
- [[Design Decisions]] — `docs/rfcs/adopt-libp2p.md` for the full reasoning
