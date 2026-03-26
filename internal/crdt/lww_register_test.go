package crdt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestLWWRegister_Set_StoresValueWithTimestamp(t *testing.T) {
	clock := NewHLC("peer-a")
	reg := NewLWWRegister[string]()
	ts := clock.Now()

	reg.Set("hello", ts)

	val, ok := reg.Get()
	assert.True(t, ok)
	assert.Equal(t, "hello", val)
}

func TestLWWRegister_Get_EmptyReturnsFalse(t *testing.T) {
	reg := NewLWWRegister[string]()
	_, ok := reg.Get()
	assert.False(t, ok)
}

func TestLWWRegister_Set_LaterTimestampWins(t *testing.T) {
	clock := NewHLC("peer-a")
	reg := NewLWWRegister[string]()

	ts1 := clock.Now()
	ts2 := clock.Now()

	reg.Set("first", ts1)
	reg.Set("second", ts2)

	val, _ := reg.Get()
	assert.Equal(t, "second", val)
}

func TestLWWRegister_Set_EarlierTimestampIgnored(t *testing.T) {
	clock := NewHLC("peer-a")
	reg := NewLWWRegister[string]()

	ts1 := clock.Now()
	ts2 := clock.Now()

	reg.Set("second", ts2)
	reg.Set("first", ts1) // should be ignored

	val, _ := reg.Get()
	assert.Equal(t, "second", val)
}

func TestLWWRegister_Merge_HigherTimestampWins(t *testing.T) {
	clockA := NewHLC("peer-a")
	clockB := NewHLC("peer-b")

	regA := NewLWWRegister[string]()
	regB := NewLWWRegister[string]()

	tsA := clockA.Now()
	regA.Set("from-a", tsA)

	// Advance B's clock past A
	clockB.Update(tsA)
	tsB := clockB.Now()
	regB.Set("from-b", tsB)

	regA.Merge(regB)

	val, _ := regA.Get()
	assert.Equal(t, "from-b", val)
}

func TestLWWRegister_Merge_PreservesHigherLocal(t *testing.T) {
	clockA := NewHLC("peer-a")
	clockB := NewHLC("peer-b")

	regA := NewLWWRegister[string]()
	regB := NewLWWRegister[string]()

	tsB := clockB.Now()
	regB.Set("from-b", tsB)

	// Advance A past B
	clockA.Update(tsB)
	tsA := clockA.Now()
	regA.Set("from-a", tsA)

	regA.Merge(regB)

	val, _ := regA.Get()
	assert.Equal(t, "from-a", val)
}

func TestLWWRegister_Deleted(t *testing.T) {
	clock := NewHLC("peer-a")
	reg := NewLWWRegister[string]()

	ts1 := clock.Now()
	reg.Set("hello", ts1)

	ts2 := clock.Now()
	reg.Delete(ts2)

	_, ok := reg.Get()
	assert.False(t, ok)
	assert.True(t, reg.IsDeleted())
}

func TestLWWRegister_JSONRoundTrip(t *testing.T) {
	clock := NewHLC("peer-a")
	reg := NewLWWRegister[string]()
	reg.Set("hello", clock.Now())

	data, err := json.Marshal(reg)
	require.NoError(t, err)

	var reg2 LWWRegister[string]
	err = json.Unmarshal(data, &reg2)
	require.NoError(t, err)

	val, ok := reg2.Get()
	assert.True(t, ok)
	assert.Equal(t, "hello", val)
}

// Property: merge is commutative
func TestLWWRegister_Property_MergeCommutative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		peerA := "peer-a"
		peerB := "peer-b"
		clockA := NewHLC(peerA)
		clockB := NewHLC(peerB)

		valA := rapid.String().Draw(t, "valA")
		valB := rapid.String().Draw(t, "valB")

		regA := NewLWWRegister[string]()
		regA.Set(valA, clockA.Now())

		regB := NewLWWRegister[string]()
		regB.Set(valB, clockB.Now())

		// Merge A into B
		copyAB := cloneRegister(regA)
		copyAB.Merge(regB)

		// Merge B into A
		copyBA := cloneRegister(regB)
		copyBA.Merge(regA)

		vAB, _ := copyAB.Get()
		vBA, _ := copyBA.Get()

		if vAB != vBA {
			t.Fatalf("merge not commutative: merge(A,B)=%q, merge(B,A)=%q", vAB, vBA)
		}
	})
}

// Property: merge is associative
func TestLWWRegister_Property_MergeAssociative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		clockA := NewHLC("peer-a")
		clockB := NewHLC("peer-b")
		clockC := NewHLC("peer-c")

		regA := NewLWWRegister[string]()
		regA.Set(rapid.String().Draw(t, "valA"), clockA.Now())

		regB := NewLWWRegister[string]()
		regB.Set(rapid.String().Draw(t, "valB"), clockB.Now())

		regC := NewLWWRegister[string]()
		regC.Set(rapid.String().Draw(t, "valC"), clockC.Now())

		// (A merge B) merge C
		ab := cloneRegister(regA)
		ab.Merge(regB)
		abc1 := cloneRegister(ab)
		abc1.Merge(regC)

		// A merge (B merge C)
		bc := cloneRegister(regB)
		bc.Merge(regC)
		abc2 := cloneRegister(regA)
		abc2.Merge(bc)

		v1, _ := abc1.Get()
		v2, _ := abc2.Get()
		if v1 != v2 {
			t.Fatalf("merge not associative: (A+B)+C=%q, A+(B+C)=%q", v1, v2)
		}
	})
}

func TestLWWRegister_Merge_DeleteVsLiveValue(t *testing.T) {
	clockA := NewHLC("peer-a")
	clockB := NewHLC("peer-b")

	// A has a live value
	regA := NewLWWRegister[string]()
	tsA := clockA.Now()
	regA.Set("alive", tsA)

	// B deletes with a later timestamp
	regB := NewLWWRegister[string]()
	clockB.Update(tsA)
	regB.Set("temp", clockB.Now())
	regB.Delete(clockB.Now())

	// Merge: deletion with later timestamp should win
	regA.Merge(regB)
	_, ok := regA.Get()
	assert.False(t, ok, "deletion with later timestamp should win")
	assert.True(t, regA.IsDeleted())
}

func TestLWWRegister_Merge_LiveValueBeatsOlderDeletion(t *testing.T) {
	clockA := NewHLC("peer-a")
	clockB := NewHLC("peer-b")

	// B deletes first
	regB := NewLWWRegister[string]()
	tsB := clockB.Now()
	regB.Set("temp", tsB)
	regB.Delete(clockB.Now())

	// A sets with a later timestamp
	regA := NewLWWRegister[string]()
	clockA.Update(clockB.Now())
	regA.Set("alive", clockA.Now())

	// Merge: live value with later timestamp should win
	regA.Merge(regB)
	val, ok := regA.Get()
	assert.True(t, ok, "live value with later timestamp should win")
	assert.Equal(t, "alive", val)
}

// Property: merge is idempotent
func TestLWWRegister_Property_MergeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		clock := NewHLC("peer-a")
		val := rapid.String().Draw(t, "val")

		reg := NewLWWRegister[string]()
		reg.Set(val, clock.Now())

		before, _ := reg.Get()
		reg.Merge(reg)
		after, _ := reg.Get()

		if before != after {
			t.Fatalf("merge not idempotent: before=%q, after=%q", before, after)
		}
	})
}

func cloneRegister[T any](src *LWWRegister[T]) *LWWRegister[T] {
	data, _ := json.Marshal(src)
	var dst LWWRegister[T]
	json.Unmarshal(data, &dst)
	return &dst
}
