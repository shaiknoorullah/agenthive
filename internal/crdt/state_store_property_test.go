package crdt

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// Property: N peers making random edits, merging in any order, all converge
func TestStateStore_Property_MultiPeerConvergence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nPeers := rapid.IntRange(2, 5).Draw(t, "nPeers")

		stores := make([]*StateStore, nPeers)
		for i := 0; i < nPeers; i++ {
			// Peer IDs must be unique for CRDT tiebreaking to work correctly.
			// Use index suffix to guarantee uniqueness.
			base := rapid.StringMatching(`[a-z]{3,8}`).Draw(t, "peerBase")
			stores[i] = NewStateStore(fmt.Sprintf("peer-%s-%d", base, i))
		}

		// Each peer makes some edits
		for i := 0; i < nPeers; i++ {
			nOps := rapid.IntRange(1, 5).Draw(t, "nOps")
			for j := 0; j < nOps; j++ {
				key := rapid.StringMatching(`[a-z]{1,4}`).Draw(t, "key")
				val := rapid.String().Draw(t, "val")
				stores[i].SetConfig(key, val)
			}
		}

		// Merge all stores into each other using LWWMap-level merge
		for i := 0; i < nPeers; i++ {
			for j := 0; j < nPeers; j++ {
				if i != j {
					stores[i].MergeMaps(stores[j].PeersMap(), stores[j].RoutesMap(), stores[j].ConfigMap())
				}
			}
		}

		// All stores should have identical config
		ref := stores[0]
		for i := 1; i < nPeers; i++ {
			for _, k := range stores[0].ConfigMap().Keys() {
				v0, ok0 := ref.GetConfig(k)
				vi, oki := stores[i].GetConfig(k)
				if ok0 != oki || v0 != vi {
					t.Fatalf("peer %d diverged from peer 0 on key %q: %q vs %q", i, k, vi, v0)
				}
			}
		}
	})
}

// Property: merge order does not matter (forced key collisions)
func TestStateStore_Property_MergeOrderIndependent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		storeA := NewStateStore("peer-a")
		storeB := NewStateStore("peer-b")
		storeC := NewStateStore("peer-c")

		// ALL peers write to the SAME key -- forces collision
		sharedKey := "shared"
		storeA.SetConfig(sharedKey, rapid.String().Draw(t, "vA"))
		storeB.SetConfig(sharedKey, rapid.String().Draw(t, "vB"))
		storeC.SetConfig(sharedKey, rapid.String().Draw(t, "vC"))

		// Order 1: A + B + C
		s1 := NewStateStore("test-1")
		s1.MergeMaps(storeA.PeersMap(), storeA.RoutesMap(), storeA.ConfigMap())
		s1.MergeMaps(storeB.PeersMap(), storeB.RoutesMap(), storeB.ConfigMap())
		s1.MergeMaps(storeC.PeersMap(), storeC.RoutesMap(), storeC.ConfigMap())

		// Order 2: C + A + B
		s2 := NewStateStore("test-2")
		s2.MergeMaps(storeC.PeersMap(), storeC.RoutesMap(), storeC.ConfigMap())
		s2.MergeMaps(storeA.PeersMap(), storeA.RoutesMap(), storeA.ConfigMap())
		s2.MergeMaps(storeB.PeersMap(), storeB.RoutesMap(), storeB.ConfigMap())

		// Order 3: B + C + A
		s3 := NewStateStore("test-3")
		s3.MergeMaps(storeB.PeersMap(), storeB.RoutesMap(), storeB.ConfigMap())
		s3.MergeMaps(storeC.PeersMap(), storeC.RoutesMap(), storeC.ConfigMap())
		s3.MergeMaps(storeA.PeersMap(), storeA.RoutesMap(), storeA.ConfigMap())

		// All three must converge to the same value
		v1, _ := s1.GetConfig(sharedKey)
		v2, _ := s2.GetConfig(sharedKey)
		v3, _ := s3.GetConfig(sharedKey)

		if v1 != v2 || v1 != v3 {
			t.Fatalf("merge order dependent: %q vs %q vs %q", v1, v2, v3)
		}
	})
}
