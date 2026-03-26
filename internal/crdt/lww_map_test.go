package crdt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestLWWMap_SetAndGet(t *testing.T) {
	clock := NewHLC("peer-a")
	m := NewLWWMap[string]()

	m.Set("key1", "value1", clock.Now())

	val, ok := m.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)
}

func TestLWWMap_Get_MissingKeyReturnsFalse(t *testing.T) {
	m := NewLWWMap[string]()
	_, ok := m.Get("nonexistent")
	assert.False(t, ok)
}

func TestLWWMap_Delete(t *testing.T) {
	clock := NewHLC("peer-a")
	m := NewLWWMap[string]()

	m.Set("key1", "value1", clock.Now())
	m.Delete("key1", clock.Now())

	_, ok := m.Get("key1")
	assert.False(t, ok)
}

func TestLWWMap_Keys_ExcludesDeleted(t *testing.T) {
	clock := NewHLC("peer-a")
	m := NewLWWMap[string]()

	m.Set("a", "1", clock.Now())
	m.Set("b", "2", clock.Now())
	m.Set("c", "3", clock.Now())
	m.Delete("b", clock.Now())

	keys := m.Keys()
	assert.ElementsMatch(t, []string{"a", "c"}, keys)
}

func TestLWWMap_Len(t *testing.T) {
	clock := NewHLC("peer-a")
	m := NewLWWMap[string]()

	m.Set("a", "1", clock.Now())
	m.Set("b", "2", clock.Now())
	m.Delete("a", clock.Now())

	assert.Equal(t, 1, m.Len())
}

func TestLWWMap_Merge_CombinesMaps(t *testing.T) {
	clockA := NewHLC("peer-a")
	clockB := NewHLC("peer-b")

	mapA := NewLWWMap[string]()
	mapA.Set("key1", "from-a", clockA.Now())

	mapB := NewLWWMap[string]()
	mapB.Set("key2", "from-b", clockB.Now())

	mapA.Merge(mapB)

	val1, ok1 := mapA.Get("key1")
	val2, ok2 := mapA.Get("key2")
	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.Equal(t, "from-a", val1)
	assert.Equal(t, "from-b", val2)
}

func TestLWWMap_Merge_ConflictHigherTimestampWins(t *testing.T) {
	clockA := NewHLC("peer-a")
	clockB := NewHLC("peer-b")

	mapA := NewLWWMap[string]()
	tsA := clockA.Now()
	mapA.Set("key1", "old", tsA)

	mapB := NewLWWMap[string]()
	clockB.Update(tsA) // advance B past A
	mapB.Set("key1", "new", clockB.Now())

	mapA.Merge(mapB)

	val, _ := mapA.Get("key1")
	assert.Equal(t, "new", val)
}

func TestLWWMap_Delta_ReturnsSinceTimestamp(t *testing.T) {
	clock := NewHLC("peer-a")
	m := NewLWWMap[string]()

	m.Set("old-key", "old-val", clock.Now())
	since := clock.Now()
	m.Set("new-key", "new-val", clock.Now())

	delta := m.Delta(since)

	assert.Equal(t, 1, delta.Len())
	val, ok := delta.Get("new-key")
	assert.True(t, ok)
	assert.Equal(t, "new-val", val)
}

func TestLWWMap_Snapshot_ReturnsFullState(t *testing.T) {
	clock := NewHLC("peer-a")
	m := NewLWWMap[string]()

	m.Set("a", "1", clock.Now())
	m.Set("b", "2", clock.Now())

	snap := m.Snapshot()
	assert.Equal(t, 2, snap.Len())
}

func TestLWWMap_JSONRoundTrip(t *testing.T) {
	clock := NewHLC("peer-a")
	m := NewLWWMap[string]()
	m.Set("key1", "value1", clock.Now())
	m.Set("key2", "value2", clock.Now())

	data, err := json.Marshal(m)
	require.NoError(t, err)

	m2 := NewLWWMap[string]()
	err = json.Unmarshal(data, m2)
	require.NoError(t, err)

	val, ok := m2.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)
	assert.Equal(t, 2, m2.Len())
}

// Property: merge is commutative
func TestLWWMap_Property_MergeCommutative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		clockA := NewHLC("peer-a")
		clockB := NewHLC("peer-b")

		mapA := NewLWWMap[string]()
		mapB := NewLWWMap[string]()

		nA := rapid.IntRange(1, 10).Draw(t, "nA")
		for i := 0; i < nA; i++ {
			key := rapid.StringMatching(`[a-z]{1,5}`).Draw(t, "keyA")
			val := rapid.String().Draw(t, "valA")
			mapA.Set(key, val, clockA.Now())
		}

		nB := rapid.IntRange(1, 10).Draw(t, "nB")
		for i := 0; i < nB; i++ {
			key := rapid.StringMatching(`[a-z]{1,5}`).Draw(t, "keyB")
			val := rapid.String().Draw(t, "valB")
			mapB.Set(key, val, clockB.Now())
		}

		// merge(A,B)
		abData, _ := json.Marshal(mapA)
		var ab LWWMap[string]
		json.Unmarshal(abData, &ab)
		ab.Merge(mapB)

		// merge(B,A)
		baData, _ := json.Marshal(mapB)
		var ba LWWMap[string]
		json.Unmarshal(baData, &ba)
		ba.Merge(mapA)

		// All keys should have same values
		allKeys := make(map[string]bool)
		for _, k := range ab.Keys() {
			allKeys[k] = true
		}
		for _, k := range ba.Keys() {
			allKeys[k] = true
		}

		for k := range allKeys {
			vAB, okAB := ab.Get(k)
			vBA, okBA := ba.Get(k)
			if okAB != okBA || vAB != vBA {
				t.Fatalf("not commutative for key %q: merge(A,B)=%v, merge(B,A)=%v", k, vAB, vBA)
			}
		}
	})
}

// Property: merge is associative
func TestLWWMap_Property_MergeAssociative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		clockA := NewHLC("peer-a")
		clockB := NewHLC("peer-b")
		clockC := NewHLC("peer-c")

		mapA := NewLWWMap[string]()
		mapB := NewLWWMap[string]()
		mapC := NewLWWMap[string]()

		for _, pair := range []struct {
			m     *LWWMap[string]
			clock *HLC
		}{{mapA, clockA}, {mapB, clockB}, {mapC, clockC}} {
			n := rapid.IntRange(1, 5).Draw(t, "n")
			for i := 0; i < n; i++ {
				key := rapid.StringMatching(`[a-z]{1,3}`).Draw(t, "key")
				val := rapid.String().Draw(t, "val")
				pair.m.Set(key, val, pair.clock.Now())
			}
		}

		cloneMap := func(m *LWWMap[string]) *LWWMap[string] {
			data, _ := json.Marshal(m)
			var c LWWMap[string]
			json.Unmarshal(data, &c)
			return &c
		}

		// (A merge B) merge C
		ab := cloneMap(mapA)
		ab.Merge(mapB)
		abc1 := cloneMap(ab)
		abc1.Merge(mapC)

		// A merge (B merge C)
		bc := cloneMap(mapB)
		bc.Merge(mapC)
		abc2 := cloneMap(mapA)
		abc2.Merge(bc)

		for _, k := range abc1.Keys() {
			v1, _ := abc1.Get(k)
			v2, _ := abc2.Get(k)
			if v1 != v2 {
				t.Fatalf("not associative for key %q", k)
			}
		}
	})
}

// Property: merge is idempotent
func TestLWWMap_Property_MergeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		clock := NewHLC("peer-a")
		m := NewLWWMap[string]()

		n := rapid.IntRange(1, 10).Draw(t, "n")
		for i := 0; i < n; i++ {
			key := rapid.StringMatching(`[a-z]{1,5}`).Draw(t, "key")
			val := rapid.String().Draw(t, "val")
			m.Set(key, val, clock.Now())
		}

		before := make(map[string]string)
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			before[k] = v
		}

		m.Merge(m)

		for k, v := range before {
			after, ok := m.Get(k)
			if !ok || after != v {
				t.Fatalf("merge(A,A) changed key %q: %q -> %q", k, v, after)
			}
		}
	})
}
