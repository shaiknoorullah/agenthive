package crdt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestHLC_Now_ReturnsMonotonicallyIncreasingTimestamps(t *testing.T) {
	clock := NewHLC("peer-a")
	ts1 := clock.Now()
	ts2 := clock.Now()
	assert.True(t, ts2.After(ts1), "second timestamp must be after first")
}

func TestHLC_Now_IncreasesCounterWhenWallClockUnchanged(t *testing.T) {
	wall := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	clock := NewHLCWithWall("peer-a", func() time.Time { return wall })
	ts1 := clock.Now()
	ts2 := clock.Now()
	assert.Equal(t, ts1.Wall, ts2.Wall)
	assert.Equal(t, uint32(1), ts2.Counter-ts1.Counter)
}

func TestHLC_Update_AdvancesFromRemoteTimestamp(t *testing.T) {
	clockA := NewHLC("peer-a")
	clockB := NewHLC("peer-b")

	tsA := clockA.Now()
	// Simulate clockB receiving tsA from peer-a
	tsB := clockB.Update(tsA)

	assert.True(t, tsB.After(tsA), "updated timestamp must be after received timestamp")
}

func TestHLC_Update_HandlesRemoteAheadOfLocal(t *testing.T) {
	future := time.Now().Add(10 * time.Second)
	clockA := NewHLCWithWall("peer-a", func() time.Time { return future })
	tsA := clockA.Now()

	clockB := NewHLC("peer-b")
	tsB := clockB.Update(tsA)

	assert.True(t, tsB.After(tsA))
}

func TestTimestamp_After_UsesWallThenCounterThenPeerID(t *testing.T) {
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	later := now.Add(1 * time.Second)

	// Wall time wins
	ts1 := Timestamp{Wall: now, Counter: 5, PeerID: "z"}
	ts2 := Timestamp{Wall: later, Counter: 0, PeerID: "a"}
	assert.True(t, ts2.After(ts1))

	// Same wall, counter wins
	ts3 := Timestamp{Wall: now, Counter: 1, PeerID: "z"}
	ts4 := Timestamp{Wall: now, Counter: 2, PeerID: "a"}
	assert.True(t, ts4.After(ts3))

	// Same wall+counter, peer ID wins (lexicographic)
	ts5 := Timestamp{Wall: now, Counter: 1, PeerID: "aaa"}
	ts6 := Timestamp{Wall: now, Counter: 1, PeerID: "bbb"}
	assert.True(t, ts6.After(ts5))
}

func TestTimestamp_Equal(t *testing.T) {
	now := time.Now()
	ts1 := Timestamp{Wall: now, Counter: 1, PeerID: "a"}
	ts2 := Timestamp{Wall: now, Counter: 1, PeerID: "a"}
	assert.False(t, ts1.After(ts2))
	assert.False(t, ts2.After(ts1))
}

func TestTimestamp_JSONRoundTrip(t *testing.T) {
	ts := Timestamp{
		Wall:    time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC),
		Counter: 42,
		PeerID:  "peer-abc",
	}
	data, err := ts.MarshalJSON()
	require.NoError(t, err)

	var ts2 Timestamp
	err = ts2.UnmarshalJSON(data)
	require.NoError(t, err)

	assert.True(t, ts.Wall.Equal(ts2.Wall))
	assert.Equal(t, ts.Counter, ts2.Counter)
	assert.Equal(t, ts.PeerID, ts2.PeerID)
}

// Property-based: HLC is always monotonic
func TestHLC_Property_Monotonic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		peerID := rapid.StringMatching(`[a-z]{3,8}`).Draw(t, "peerID")
		clock := NewHLC(peerID)
		n := rapid.IntRange(2, 50).Draw(t, "n")

		prev := clock.Now()
		for i := 1; i < n; i++ {
			curr := clock.Now()
			if !curr.After(prev) {
				t.Fatalf("timestamp %d not after %d: %v vs %v", i, i-1, curr, prev)
			}
			prev = curr
		}
	})
}

// Property-based: Update always produces a timestamp after both local and remote
func TestHLC_Property_UpdateDominates(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		peerA := rapid.StringMatching(`[a-z]{3,8}`).Draw(t, "peerA")
		peerB := rapid.StringMatching(`[a-z]{3,8}`).Draw(t, "peerB")

		clockA := NewHLC(peerA)
		clockB := NewHLC(peerB)

		// Generate some ticks on A
		nA := rapid.IntRange(1, 10).Draw(t, "nA")
		var tsA Timestamp
		for i := 0; i < nA; i++ {
			tsA = clockA.Now()
		}

		// Generate some ticks on B
		nB := rapid.IntRange(1, 10).Draw(t, "nB")
		var tsB Timestamp
		for i := 0; i < nB; i++ {
			tsB = clockB.Now()
		}

		// B receives A's timestamp
		result := clockB.Update(tsA)

		if !result.After(tsA) {
			t.Fatalf("update result must be after remote timestamp")
		}
		if !result.After(tsB) {
			t.Fatalf("update result must be after local timestamp")
		}
	})
}
