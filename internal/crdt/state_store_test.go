package crdt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Suppress unused import warning
var _ = json.Marshal
var _ = os.ReadFile
var _ = filepath.Join
var _ = time.Now

func TestStateStore_SetAndGetPeer(t *testing.T) {
	store := NewStateStore("peer-a")

	store.SetPeer("server-1", &PeerInfo{
		Name:   "server-1",
		Status: "online",
		Addr:   "10.0.0.1:19222",
	})

	peer, ok := store.GetPeer("server-1")
	assert.True(t, ok)
	assert.Equal(t, "server-1", peer.Name)
	assert.Equal(t, "online", peer.Status)
}

func TestStateStore_SetAndGetRoute(t *testing.T) {
	store := NewStateStore("peer-a")

	store.SetRoute("route-1", &RouteRule{
		Match:   RouteMatch{Project: "api-server", Priority: "critical"},
		Targets: []string{"phone", "laptop"},
	})

	route, ok := store.GetRoute("route-1")
	assert.True(t, ok)
	assert.Equal(t, "api-server", route.Match.Project)
	assert.ElementsMatch(t, []string{"phone", "laptop"}, route.Targets)
}

func TestStateStore_SetAndGetConfig(t *testing.T) {
	store := NewStateStore("peer-a")

	store.SetConfig("stale-timeout", "3600")

	val, ok := store.GetConfig("stale-timeout")
	assert.True(t, ok)
	assert.Equal(t, "3600", val)
}

func TestStateStore_ListPeers(t *testing.T) {
	store := NewStateStore("peer-a")
	store.SetPeer("s1", &PeerInfo{Name: "s1"})
	store.SetPeer("s2", &PeerInfo{Name: "s2"})

	peers := store.ListPeers()
	assert.Len(t, peers, 2)
}

func TestStateStore_ListRoutes(t *testing.T) {
	store := NewStateStore("peer-a")
	store.SetRoute("r1", &RouteRule{Match: RouteMatch{Project: "a"}})
	store.SetRoute("r2", &RouteRule{Match: RouteMatch{Project: "b"}})

	routes := store.ListRoutes()
	assert.Len(t, routes, 2)
}

func TestStateStore_DeleteRoute(t *testing.T) {
	store := NewStateStore("peer-a")
	store.SetRoute("r1", &RouteRule{Match: RouteMatch{Project: "a"}})
	store.DeleteRoute("r1")

	_, ok := store.GetRoute("r1")
	assert.False(t, ok)
}

func TestStateStore_Merge_TwoStoresConverge(t *testing.T) {
	storeA := NewStateStore("peer-a")
	storeB := NewStateStore("peer-b")

	storeA.SetPeer("s1", &PeerInfo{Name: "from-a"})
	storeB.SetConfig("key1", "from-b")

	// Merge using LWWMap-level merge (preserves CRDT timestamps)
	storeA.MergeMaps(storeB.PeersMap(), storeB.RoutesMap(), storeB.ConfigMap())

	peer, ok := storeA.GetPeer("s1")
	assert.True(t, ok)
	assert.Equal(t, "from-a", peer.Name)

	cfg, ok := storeA.GetConfig("key1")
	assert.True(t, ok)
	assert.Equal(t, "from-b", cfg)
}

func TestStateStore_Merge_PreservesOriginalTimestamps(t *testing.T) {
	storeA := NewStateStore("peer-a")
	storeB := NewStateStore("peer-b")

	// B writes first
	storeB.SetConfig("key1", "from-b-early")

	// A writes later with a higher timestamp
	time.Sleep(1 * time.Millisecond) // ensure wall clock advances
	storeA.SetConfig("key1", "from-a-later")

	// Merge B into A: A's value should win (later timestamp)
	storeA.MergeMaps(storeB.PeersMap(), storeB.RoutesMap(), storeB.ConfigMap())
	cfg, ok := storeA.GetConfig("key1")
	assert.True(t, ok)
	assert.Equal(t, "from-a-later", cfg)

	// Merge A into B: A's value should still win
	storeB.MergeMaps(storeA.PeersMap(), storeA.RoutesMap(), storeA.ConfigMap())
	cfg2, ok := storeB.GetConfig("key1")
	assert.True(t, ok)
	assert.Equal(t, "from-a-later", cfg2)
}

func TestStateStore_Delta_ReturnsSinceLastSync(t *testing.T) {
	store := NewStateStore("peer-a")

	store.SetPeer("old", &PeerInfo{Name: "old"})
	since := store.CurrentTimestamp()
	store.SetPeer("new", &PeerInfo{Name: "new"})

	peersDelta, _, _ := store.DeltaMaps(since)

	_, oldExists := peersDelta.Get("old")
	_, newExists := peersDelta.Get("new")

	assert.False(t, oldExists, "old peer should not be in delta")
	assert.True(t, newExists, "new peer should be in delta")
}

func TestStateStore_Delta_IncludesDeletedEntries(t *testing.T) {
	store := NewStateStore("peer-a")

	store.SetRoute("r1", &RouteRule{Match: RouteMatch{Project: "api"}})
	since := store.CurrentTimestamp()
	store.DeleteRoute("r1")

	_, routesDelta, _ := store.DeltaMaps(since)

	// The deleted route must be in the delta so deletion propagates.
	// Serialize the delta to JSON and verify the tombstone is present.
	data, err := json.Marshal(routesDelta)
	require.NoError(t, err)
	assert.Contains(t, string(data), "r1", "delta must contain tombstone for deleted key")
	assert.Contains(t, string(data), `"deleted"`, "delta must mark entry as deleted")
}

func TestStateStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store := NewStateStore("peer-a")
	store.SetPeer("s1", &PeerInfo{Name: "s1", Status: "online"})
	store.SetRoute("r1", &RouteRule{Match: RouteMatch{Project: "api"}, Targets: []string{"phone"}})
	store.SetConfig("timeout", "1800")

	err := store.SaveToFile(path)
	require.NoError(t, err)

	store2 := NewStateStore("peer-a")
	err = store2.LoadFromFile(path)
	require.NoError(t, err)

	peer, ok := store2.GetPeer("s1")
	assert.True(t, ok)
	assert.Equal(t, "online", peer.Status)

	route, ok := store2.GetRoute("r1")
	assert.True(t, ok)
	assert.Equal(t, "api", route.Match.Project)

	cfg, ok := store2.GetConfig("timeout")
	assert.True(t, ok)
	assert.Equal(t, "1800", cfg)
}

func TestStateStore_LoadFromFile_NonexistentIsNotError(t *testing.T) {
	store := NewStateStore("peer-a")
	err := store.LoadFromFile("/tmp/nonexistent-agenthive-test-file.json")
	assert.NoError(t, err)
}

func TestStateStore_MergeMaps_IsCommutative(t *testing.T) {
	storeA := NewStateStore("peer-a")
	storeB := NewStateStore("peer-b")

	// Both write to the SAME key with different values
	storeA.SetConfig("shared-key", "value-a")
	time.Sleep(1 * time.Millisecond)
	storeB.SetConfig("shared-key", "value-b")

	// Merge into EMPTY stores in opposite orders to test pure commutativity
	s1 := NewStateStore("empty-1")
	s1.MergeMaps(storeA.PeersMap(), storeA.RoutesMap(), storeA.ConfigMap())
	s1.MergeMaps(storeB.PeersMap(), storeB.RoutesMap(), storeB.ConfigMap())

	s2 := NewStateStore("empty-2")
	s2.MergeMaps(storeB.PeersMap(), storeB.RoutesMap(), storeB.ConfigMap())
	s2.MergeMaps(storeA.PeersMap(), storeA.RoutesMap(), storeA.ConfigMap())

	cfgAB, _ := s1.GetConfig("shared-key")
	cfgBA, _ := s2.GetConfig("shared-key")
	assert.Equal(t, cfgAB, cfgBA, "merge must be commutative: both must converge to the same value")
}
