package router

import (
	"sort"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/identity"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// newPeerID returns a fresh, deterministically-formatted libp2p peer ID for
// use as a target/self in tests. Each call produces a distinct ID.
func newPeerID(t *testing.T) peer.ID {
	t.Helper()
	priv, _, err := identity.Generate()
	require.NoError(t, err)
	pid, err := peer.IDFromPrivateKey(priv)
	require.NoError(t, err)
	return pid
}

// sortIDs returns ids sorted by their string form, so test assertions can
// compare against ordered expectations regardless of internal map ordering.
func sortIDs(ids []peer.ID) []peer.ID {
	out := make([]peer.ID, len(ids))
	copy(out, ids)
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// registerPeer records a peer in the store with the canonical encoded ID as
// the map key (matches how the daemon writes peer entries).
func registerPeer(t *testing.T, store *crdt.StateStore, pid peer.ID, name string) {
	t.Helper()
	store.SetPeer(pid.String(), crdt.PeerInfo{
		Name:     name,
		Status:   "online",
		LastSeen: time.Now().UTC().Format(time.RFC3339),
	})
}

func TestNewMatcher_ConstructsWithStoreAndSelf(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)

	m := NewMatcher(store, self)

	require.NotNil(t, m, "NewMatcher must return a non-nil Matcher")
}

func TestTargets_EmptyStore_ReturnsEmpty(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	m := NewMatcher(store, self)

	got := m.Targets(protocols.Notification{
		Source:   "claude-code",
		Project:  "agenthive",
		Priority: "info",
		Message:  "hello",
	})

	require.Empty(t, got, "no routes ⇒ no targets")
}

func TestTargets_SingleMatchingRoute_ReturnsThatTarget(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	target := newPeerID(t)

	registerPeer(t, store, target, "phone")

	store.SetRoute("r1", crdt.RouteRule{
		Match:   crdt.RouteMatch{Source: "claude-code"},
		Targets: []string{target.String()},
	})

	m := NewMatcher(store, self)
	got := m.Targets(protocols.Notification{
		Source:  "claude-code",
		Message: "hello",
	})

	require.Equal(t, []peer.ID{target}, got)
}

func TestTargets_MultipleMatchingRoutes_UnionTargets(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	t1 := newPeerID(t)
	t2 := newPeerID(t)
	t3 := newPeerID(t)

	registerPeer(t, store, t1, "phone")
	registerPeer(t, store, t2, "laptop")
	registerPeer(t, store, t3, "tablet")

	store.SetRoute("by-source", crdt.RouteRule{
		Match:   crdt.RouteMatch{Source: "claude-code"},
		Targets: []string{t1.String(), t2.String()},
	})
	store.SetRoute("by-priority", crdt.RouteRule{
		Match:   crdt.RouteMatch{Priority: "critical"},
		Targets: []string{t2.String(), t3.String()},
	})

	m := NewMatcher(store, self)
	got := m.Targets(protocols.Notification{
		Source:   "claude-code",
		Priority: "critical",
	})

	require.ElementsMatch(t, []peer.ID{t1, t2, t3}, got, "union must dedupe overlapping targets")
}

func TestTargets_ALL_ExpandsToAllPeersExceptSelf(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	p1 := newPeerID(t)
	p2 := newPeerID(t)

	// self is registered as a peer too (the daemon registers itself); the
	// matcher must still strip self from the expansion.
	registerPeer(t, store, self, "me")
	registerPeer(t, store, p1, "phone")
	registerPeer(t, store, p2, "laptop")

	store.SetRoute("everyone", crdt.RouteRule{
		Match:   crdt.RouteMatch{}, // wildcard selector
		Targets: []string{"ALL"},
	})

	m := NewMatcher(store, self)
	got := m.Targets(protocols.Notification{
		Source:   "claude-code",
		Priority: "info",
	})

	require.ElementsMatch(t, sortIDs([]peer.ID{p1, p2}), sortIDs(got))
	require.NotContains(t, got, self, "ALL expansion must exclude self")
}

func TestTargets_WildcardSelector_MatchesEverything(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	target := newPeerID(t)
	registerPeer(t, store, target, "phone")

	store.SetRoute("catch-all", crdt.RouteRule{
		Match:   crdt.RouteMatch{}, // no fields set ⇒ wildcard
		Targets: []string{target.String()},
	})

	m := NewMatcher(store, self)

	require.Equal(t, []peer.ID{target}, m.Targets(protocols.Notification{Source: "claude-code"}))
	require.Equal(t, []peer.ID{target}, m.Targets(protocols.Notification{Source: "codex", Priority: "critical"}))
	require.Equal(t, []peer.ID{target}, m.Targets(protocols.Notification{Project: "anything", Priority: "warn"}))
}

func TestTargets_ProjectMismatch_NoMatch(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	target := newPeerID(t)
	registerPeer(t, store, target, "phone")

	store.SetRoute("by-project", crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "agenthive"},
		Targets: []string{target.String()},
	})

	m := NewMatcher(store, self)
	got := m.Targets(protocols.Notification{
		Project: "other-project",
		Source:  "claude-code",
	})

	require.Empty(t, got, "project field mismatch must not match")
}

func TestTargets_PriorityMatch_ProjectWildcard_Matches(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	target := newPeerID(t)
	registerPeer(t, store, target, "phone")

	store.SetRoute("critical-anywhere", crdt.RouteRule{
		Match:   crdt.RouteMatch{Priority: "critical"}, // project unset ⇒ wildcard
		Targets: []string{target.String()},
	})

	m := NewMatcher(store, self)
	got := m.Targets(protocols.Notification{
		Priority: "critical",
		Project:  "any-project",
		Source:   "claude-code",
	})

	require.Equal(t, []peer.ID{target}, got)
}

func TestTargets_ExcludesSelf(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	other := newPeerID(t)
	registerPeer(t, store, self, "me")
	registerPeer(t, store, other, "phone")

	// Route explicitly lists self as a target — matcher must still strip it.
	store.SetRoute("includes-self", crdt.RouteRule{
		Match:   crdt.RouteMatch{},
		Targets: []string{self.String(), other.String()},
	})

	m := NewMatcher(store, self)
	got := m.Targets(protocols.Notification{Source: "claude-code"})

	require.Equal(t, []peer.ID{other}, got, "self must always be excluded from targets")
}

func TestTargets_InvalidEncodedTarget_Skipped(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	target := newPeerID(t)
	registerPeer(t, store, target, "phone")

	store.SetRoute("mixed", crdt.RouteRule{
		Match:   crdt.RouteMatch{},
		Targets: []string{"not-a-peer-id", target.String()},
	})

	m := NewMatcher(store, self)
	got := m.Targets(protocols.Notification{Source: "claude-code"})

	require.Equal(t, []peer.ID{target}, got, "undecodable targets must be silently skipped, not crash")
}

func TestTargets_AllConjunctionFields(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	target := newPeerID(t)
	registerPeer(t, store, target, "phone")

	// Selector with every field set; notification must match all of them.
	store.SetRoute("strict", crdt.RouteRule{
		Match: crdt.RouteMatch{
			Project:  "agenthive",
			Session:  "sess-1",
			Source:   "claude-code",
			Priority: "warn",
		},
		Targets: []string{target.String()},
	})

	m := NewMatcher(store, self)

	// All match ⇒ target returned.
	require.Equal(t, []peer.ID{target}, m.Targets(protocols.Notification{
		SessionID: "sess-1",
		Source:    "claude-code",
		Project:   "agenthive",
		Priority:  "warn",
	}))

	// One field mismatched ⇒ no match.
	require.Empty(t, m.Targets(protocols.Notification{
		SessionID: "sess-1",
		Source:    "claude-code",
		Project:   "agenthive",
		Priority:  "info", // wrong priority
	}))
}

func TestTargets_ALL_WithExplicitTargetDedupes(t *testing.T) {
	store := crdt.NewStateStore("local")
	self := newPeerID(t)
	p1 := newPeerID(t)
	p2 := newPeerID(t)
	registerPeer(t, store, p1, "phone")
	registerPeer(t, store, p2, "laptop")

	store.SetRoute("noisy", crdt.RouteRule{
		Match:   crdt.RouteMatch{},
		Targets: []string{"ALL", p1.String()}, // p1 listed twice (once explicit, once via ALL)
	})

	m := NewMatcher(store, self)
	got := m.Targets(protocols.Notification{Source: "claude-code"})

	require.ElementsMatch(t, []peer.ID{p1, p2}, got, "result must be deduplicated")
}
