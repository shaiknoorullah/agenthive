# Glossary

| Term | Meaning |
|---|---|
| **Action gate** | The PreToolUse hook flow inside agenthive: writes `<id>.pending`, dispatches to surfaces, polls for `<id>.response`, returns the decision to Claude. See [[Action Gate]]. |
| **AutoNAT** | libp2p protocol where peers ask each other "can you reach me?" to determine NAT status. |
| **AutoRelay** | libp2p subsystem that finds Circuit Relay v2 hosts and reserves slots so a peer behind NAT becomes reachable via a relayed multiaddr. |
| **Circuit Relay v2** | A libp2p protocol where one peer forwards bytes between two others. End-to-end encrypted (the relay sees ciphertext). Every agenthive node ships this enabled. |
| **CRDT** | Conflict-free Replicated Data Type. Mathematical structure that converges deterministically across distributed replicas without consensus. agenthive uses LWW-Register + LWW-Map. |
| **DCUtR** | Direct Connection Upgrade through Relay. libp2p protocol where two NAT-bound peers coordinate via a relay, then race a simultaneous direct dial that punches their NATs. |
| **GossipSub** | libp2p's pubsub protocol. Maintains a partial mesh per topic; messages reach every subscriber even when individual links flap. agenthive uses topic `/agenthive/state/v1`. |
| **HLC** | Hybrid Logical Clock. Timestamp `{Wall, Counter, PeerID}` that orders events across the mesh without trusting wall clocks. |
| **Host** | The central object in go-libp2p. Owns identity, listening addresses, peerstore, protocol handlers. |
| **Identify** | libp2p protocol where peers exchange observed addresses on every new connection. |
| **LWW** | Last-Writer-Wins. Conflict resolution: the mutation with the higher timestamp wins per key. Ties broken by HLC.PeerID lexicographically. |
| **LWW-Map** | A map of keys to LWW-Registers. Tombstones for deletes. agenthive has three: `peers`, `routes`, `config`. |
| **LWW-Register** | Single-value LWW cell. Holds value + HLC timestamp + tombstone flag. |
| **mDNS** | Multicast DNS. Zero-config LAN service discovery on UDP 5353. agenthive uses it to auto-discover same-LAN peers. |
| **Multiaddr** | Self-describing address format: `/ip4/192.168.1.10/tcp/9123/p2p/12D3KooW...`. Includes the protocol stack and the PeerID. |
| **Noise XX** | Mutual-auth handshake from the Noise Protocol framework. Runs on every libp2p connection. Provides ChaCha20-Poly1305 encryption. |
| **PeerID** | SHA-256 multihash of an Ed25519 public key, base58-encoded. `12D3KooW…`. Identity, not assigned. |
| **Peerstore** | libp2p's local DB of known peers, addresses, and pubkeys. |
| **PreToolUse hook** | Claude Code's hook event before tool execution. Returns `{"hookSpecificOutput": {"permissionDecision": "allow"\|"deny"}}` JSON to allow or deny without showing the built-in prompt. |
| **Selector** | Comma-separated `key:value` clauses that match notification metadata. See [[Routing]]. |
| **StateStore** | The peer-scoped wrapper around three LWW-Maps (`peers`, `routes`, `config`). Lives in `internal/crdt/`. |
| **Surface** | An output adapter: log file, tmux status line, desktop notification, phone push, Slack message, etc. agenthive ships log, tmux, and desktop in v0.1.0. |
| **Tombstone** | LWW write of a `Deleted: true` marker. Propagates like a value; loses to later live writes via timestamp. |
| **TPM** | Tmux Plugin Manager — `github.com/tmux-plugins/tpm`. Installs tmux plugins from `~/.tmux.conf` declarations. |
| **UPnP / NAT-PMP** | Two protocols (one IETF, one Apple) for asking a home router to forward an external port. Both queried by libp2p's `NATPortMap` option. |
| **Yamux** | Stream multiplexer used inside libp2p over TCP. Lets one connection carry many independent logical streams. QUIC has native multiplexing and doesn't need Yamux. |
