# CRDT State Sync

agenthive replicates state across the mesh as **state-based LWW-CRDTs over Hybrid Logical Clocks**. No leader, no quorum, no consensus protocol — just deterministic merge semantics that converge eventually.

## What's replicated

Three LWW-Maps live in `internal/crdt/StateStore`:

| Map | Key | Value |
|---|---|---|
| `peers` | PeerID (string) | `PeerInfo{Name, Status, Addr, LinkType, LastSeen}` |
| `routes` | rule ID (string) | `RouteRule{Match, Targets, Action}` |
| `config` | key (string) | `ConfigEntry{Value, UpdatedBy}` |

Each entry inside a map is wrapped in a `LWWRegister[T]` carrying the value, an HLC timestamp, and a tombstone flag.

## Hybrid Logical Clock

Each peer keeps a clock with three fields:

```go
type Timestamp struct {
    Wall    time.Time
    Counter uint32
    PeerID  string
}
```

Ordering: `Wall` is primary; on tie, `Counter` breaks; on tie, `PeerID` lexicographic.

`Now()` produces a Timestamp that strictly dominates the previous local Timestamp. `Update(remoteTs)` merges a Timestamp received over the wire, returning one strictly after both local state and the remote.

Net effect: every mutation across the mesh has a total order, even when wall clocks drift or run backwards.

## Last-Writer-Wins semantics

When two peers mutate the same key, the one with the higher Timestamp wins. The CRDT contract:

- **Commutative**: `merge(A, B) == merge(B, A)`
- **Associative**: `(A + B) + C == A + (B + C)`
- **Idempotent**: `merge(A, A) == A`

These are property-tested using `pgregory.net/rapid`.

## Tombstones

Deletes are not removes — they are LWW writes of a `Deleted: true` marker. Tombstones propagate the same way values do; receivers that already saw a later live write keep their live value (the tombstone loses on timestamp). Receivers without a later live write apply the tombstone and stop returning the entry from `Get` and `Keys`.

Tombstones are never garbage-collected in v0.1.0. For a small mesh with bounded peer churn, the memory cost is negligible. A future release will add bounded GC once a generation has been observed by every active peer.

## Propagation: GossipSub anti-entropy

Mutations are broadcast as `StateDelta` messages on libp2p GossipSub topic `/agenthive/state/v1`:

```json
{
  "from":   "12D3KooW...",
  "peers":  "<marshaled LWWMap[PeerInfo] delta>",
  "routes": "<marshaled LWWMap[RouteRule] delta>",
  "config": "<marshaled LWWMap[ConfigEntry] delta>"
}
```

A delta is the result of `LWWMap.Delta(since)` — only entries with a timestamp after `since`. The daemon debounces emissions to 200ms after a local mutation, capped at one per 2s under sustained churn.

GossipSub maintains a partial mesh (~6 direct peers per topic, gossiping IDs to ~12 more), so messages reach every subscriber even when individual peer-to-peer links are flaky.

## Catch-up after offline

When a peer reconnects after being offline, it broadcasts a `PeerAnnounce` carrying its current Timestamp watermark. Peers with newer state respond with a snapshot via the `/agenthive/peer/announce/1` stream.

This avoids the need for a separate persistent message queue per offline peer — GossipSub handles the live case, and the announce protocol handles catch-up.

## Persistence

`StateStore.SaveToFile(path)` writes the entire store as JSON to `<config-dir>/state.json` using a temp-file + atomic rename. `LoadFromFile(path)` reads it back and merges via `MergeMaps`, preserving timestamps.

A daemon restart rejoins the mesh with its previous view; nothing is lost.

## Validation: who can publish

GossipSub messages are dropped at the validator layer if the sender PeerID isn't in the local `peers` map. This means new peers cannot inject state into your mesh until they've been explicitly added (or discovered via mDNS on a LAN).

Per-message cryptographic signature validation (so a man-in-the-middle can't replay or forge `StateDelta` from a known PeerID) ships in v0.2.0.

## Concurrency

`LWWMap` uses an `sync.RWMutex`; `HLC` uses an `sync.Mutex`. All public methods are goroutine-safe. The race detector (`go test -race`) runs across the full test suite in CI on every PR.

## Why not a real CRDT library?

In-house implementation is ~600 lines including tests. External CRDT libraries for Go (Automerge, Yjs ports) target richer data types (rich text, lists with concurrent inserts). agenthive replicates three flat maps — the right tool is the simplest one.

## See also

- [[Architecture]] — where the CRDT sits relative to the wire
- [[NAT Traversal]] — how peers reach each other for GossipSub to work at all
- [[Design Decisions]] — `docs/rfcs/adopt-libp2p.md` for the choice of GossipSub as the carrier
