# FAQ

## Is agenthive "self-hosted"?

No, and the language matters. "Self-hosted" implies you stand up a daemon on a server somewhere. agenthive runs **peer-to-peer** on devices you already own. There is no server to deploy. It runs on your laptop, your dev server, your phone — and they talk to each other directly via libp2p.

## Does my data ever leave my devices?

Only if you route it through one of *your own* peers that has a public address. Specifically: when DCUtR hole-punching fails (~30% of the time across the public internet), agenthive falls back to Circuit Relay v2 on a peer with a reachable address. That peer is yours, runs the agenthive binary, and sees only Noise-encrypted ciphertext — it cannot decrypt the relayed traffic.

No third-party server is ever in the path.

## Why libp2p and not (Tailscale / WireGuard / mTLS over QUIC / WebRTC)?

See [[Design Decisions]] and the four advocate papers in `docs/rfcs/`. The short version: libp2p makes identity = pubkey, ships Noise XX, ships GossipSub for state diffusion, ships DCUtR + AutoRelay for NAT, all under one well-vendored library. It's the architectural match for "small symmetric mesh of identified peers" and uniquely satisfies the no-third-party-infrastructure constraint without writing a coordination plane ourselves.

## Do I need to open ports on my router?

No. agenthive enables UPnP / NAT-PMP automatically (silent no-op if the router refuses). If UPnP doesn't work, DCUtR hole-punching covers ~70% of cases. The remaining ~30% relay through one of *your own* peers with a public address — typically a cloud VPS, a dev server with a static IP, or a home box with port-forward.

If you literally have no peer with a reachable address on the planet, the WAN paths fail. mDNS still works on a LAN.

## How many devices can I have in one mesh?

Tested at 3-5. Designed for 10+. GossipSub scales well into the hundreds for small message rates; LWW-CRDTs are O(n) per merge where n is the number of mutated keys. For agenthive's traffic pattern (handful of routes, peer set rarely changes), 50+ peers is plausible without intervention.

## What happens if my phone is offline for hours?

When it comes back online:
1. The libp2p connection re-establishes (typically ~1-2s).
2. GossipSub catch-up gossips delivers any messages missed during the absence (within the GossipSub mesh's retention window).
3. If you missed more than that — say, multi-hour Doze — the phone broadcasts `PeerAnnounce` and any reachable peer sends a state snapshot.

State on the phone re-converges with whatever happened across the mesh. No data is lost, no ordering invariant is violated.

## Can I edit routes while offline?

Yes. CRDT writes stamp with the local HLC and queue on the local StateStore. When connectivity returns, the changes broadcast via GossipSub. If anyone else mutated the same key during your offline window, LWW resolves based on HLC timestamp — typically the offline edit wins (HLC carries wall clock + counter + peer ID; an offline phone's wall clock keeps ticking).

## Why isn't there a Windows binary?

go-libp2p builds on Windows, but agenthive's hooks plumbing assumes Unix sockets (`<config-dir>/agenthive.sock`). Adding TCP loopback or named pipes for Windows is on the roadmap but not in v0.1.0. PRs welcome.

## Does it work with Codex CLI?

The transport, state sync, and action gate are agent-agnostic. The hook subcommand parses Claude Code's PreToolUse JSON in v0.1.0; Codex CLI's notify callback is a smaller payload that the same machinery can handle. Codex parity is on the v0.2.0 roadmap.

## Can I run multiple Claude Code sessions and not flood my phone?

Yes via routing — direct only certain selectors to your phone. See [[Routing]]:

```bash
agenthive routes add focus-only 'priority:critical' 'phone'
agenthive routes add everything-else '*' 'laptop'
```

Now only critical notifications reach the phone; everything else logs to the laptop.

## Why no DND mode?

v0.1.0 keeps the surface minimal. The closest approximation today: delete the `phone-*` routes during DND windows, re-add when done. A real DND flag is on the v0.2.0 roadmap.

## What about Slack / Discord / ntfy / a PWA?

Surface adapters for all of these are designed in `docs/rfcs/action-buttons-research.md` and slated for v0.2.0+. v0.1.0 ships the log surface, tmux surface, and desktop notification surface as proof-of-life.

## Is the code audited?

No formal third-party audit yet. The libp2p stack below it has been independently reviewed and is securing $100B+ in Ethereum/Filecoin value. agenthive's own code (~9k lines including tests) is small enough to read end-to-end; security review pull requests are welcome.

## Is there a Docker image?

Not yet. The release binary is a static ELF on Linux, so `FROM scratch` containerization is one-line trivial — but we don't ship a published image in v0.1.0. On the roadmap.

## License?

[MIT](https://github.com/shaiknoorullah/agenthive/blob/main/LICENSE).

## How can I help?

- File issues with reproductions
- Pick up a v0.2.0 roadmap item and open a PR
- Try the install on a platform we haven't tested (BSDs, exotic Linux distros)
- Improve the wiki — these pages are MIT-licensed too
