# CRDT State Store Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundational distributed state store for agenthive using LWW-Register CRDTs with Hybrid Logical Clocks, enabling eventual consistency across all mesh peers without coordination.

**Architecture:** Three LWW-Maps (peers, routes, config) each containing LWW-Registers keyed by string IDs. Each register holds a JSON-serializable value and an HLC timestamp. Merge is commutative, associative, and idempotent -- higher HLC wins, peer ID breaks ties. The store supports delta computation (changes since a given HLC) and full-state snapshots for reconnection sync.

**Tech Stack:** Go 1.22+, `pgregory.net/rapid` (property-based testing), `github.com/stretchr/testify` (assertions), `encoding/json` (serialization).

**Testing strategy:** TDD throughout. Property-based tests verify CRDT mathematical properties (commutativity, associativity, idempotency). Fuzz tests verify JSON round-trip safety. Race detector on all tests.

---

## File Structure

```
agenthive/
  go.mod
  go.sum
  internal/
    crdt/
      hlc.go                    # Hybrid Logical Clock
      hlc_test.go               # Unit + property tests for HLC
      lww_register.go           # Last-Writer-Wins Register
      lww_register_test.go      # Unit + property tests
      lww_map.go                # Map of string -> LWW-Register
      lww_map_test.go           # Unit + property tests
      lww_map_fuzz_test.go      # Fuzz tests for JSON round-trip
      state_store.go            # Combined store (peers, routes, config)
      state_store_test.go       # Integration tests for full store
      state_store_property_test.go  # Property tests: multi-peer convergence
      state_store_concurrent_test.go # Concurrent access tests
```

---

## Task 1: Initialize Go Module

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialize the Go module**

```bash
cd /home/devsupreme/agenthive
go mod init github.com/shaiknoorullah/agenthive
```

- [ ] **Step 2: Create placeholder Go file (required before `go get`)**

```bash
mkdir -p internal/crdt
cat > internal/crdt/doc.go << 'EOF'
// Package crdt implements CRDT data structures for agenthive's distributed state.
package crdt
EOF
```

- [ ] **Step 3: Add test dependencies**

```bash
go get pgregory.net/rapid@latest
go get github.com/stretchr/testify@latest
```

- [ ] **Step 4: Verify module**

Run: `cat go.mod`
Expected: Module path `github.com/shaiknoorullah/agenthive` with require entries for rapid and testify.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/
git commit -m "feat: initialize Go module with test dependencies"
```

---

## Task 2: Hybrid Logical Clock -- Failing Tests

**Files:**
- Create: `internal/crdt/hlc.go`
- Create: `internal/crdt/hlc_test.go`

The HLC combines a physical wall clock with a logical counter and a peer ID for deterministic tiebreaking. It guarantees monotonically increasing timestamps even when physical clocks drift.

- [ ] **Step 1: Write the failing tests**

Create `internal/crdt/hlc_test.go`:

```go
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
```

- [ ] **Step 2: Create minimal type stubs so it compiles**

Create `internal/crdt/hlc.go`:

```go
package crdt

import "time"

// Timestamp is a Hybrid Logical Clock timestamp.
type Timestamp struct {
	Wall    time.Time `json:"wall"`
	Counter uint32    `json:"counter"`
	PeerID  string    `json:"peer_id"`
}

// HLC is a Hybrid Logical Clock.
type HLC struct{}

func NewHLC(peerID string) *HLC                                   { return nil }
func NewHLCWithWall(peerID string, wallFn func() time.Time) *HLC  { return nil }
func (h *HLC) Now() Timestamp                                     { return Timestamp{} }
func (h *HLC) Update(remote Timestamp) Timestamp                  { return Timestamp{} }
func (ts Timestamp) After(other Timestamp) bool                   { return false }
func (ts Timestamp) MarshalJSON() ([]byte, error)                 { return nil, nil }
func (ts *Timestamp) UnmarshalJSON(data []byte) error             { return nil }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestHLC -v -count=1 2>&1 | head -40`
Expected: All tests FAIL (stubs return zero values).

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/crdt/hlc.go internal/crdt/hlc_test.go
git commit -m "test: add failing tests for Hybrid Logical Clock"
```

---

## Task 3: Hybrid Logical Clock -- Implementation

**Files:**
- Modify: `internal/crdt/hlc.go`

- [ ] **Step 1: Implement the HLC**

Replace `internal/crdt/hlc.go` with:

```go
package crdt

import (
	"encoding/json"
	"sync"
	"time"
)

// Timestamp is a Hybrid Logical Clock timestamp.
// Ordering: Wall > Counter > PeerID (lexicographic).
type Timestamp struct {
	Wall    time.Time `json:"wall"`
	Counter uint32    `json:"counter"`
	PeerID  string    `json:"peer_id"`
}

// After returns true if ts is strictly after other.
// Comparison order: wall time, then counter, then peer ID (lexicographic).
func (ts Timestamp) After(other Timestamp) bool {
	if !ts.Wall.Equal(other.Wall) {
		return ts.Wall.After(other.Wall)
	}
	if ts.Counter != other.Counter {
		return ts.Counter > other.Counter
	}
	return ts.PeerID > other.PeerID
}

// IsZero returns true if the timestamp has not been initialized.
func (ts Timestamp) IsZero() bool {
	return ts.Wall.IsZero() && ts.Counter == 0 && ts.PeerID == ""
}

type timestampJSON struct {
	Wall    string `json:"wall"`
	Counter uint32 `json:"counter"`
	PeerID  string `json:"peer_id"`
}

func (ts Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(timestampJSON{
		Wall:    ts.Wall.UTC().Format(time.RFC3339Nano),
		Counter: ts.Counter,
		PeerID:  ts.PeerID,
	})
}

func (ts *Timestamp) UnmarshalJSON(data []byte) error {
	var raw timestampJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	wall, err := time.Parse(time.RFC3339Nano, raw.Wall)
	if err != nil {
		return err
	}
	ts.Wall = wall
	ts.Counter = raw.Counter
	ts.PeerID = raw.PeerID
	return nil
}

// HLC is a Hybrid Logical Clock.
// Safe for concurrent use.
type HLC struct {
	mu     sync.Mutex
	peerID string
	wallFn func() time.Time
	last   Timestamp
}

// NewHLC creates a new HLC with the system clock.
func NewHLC(peerID string) *HLC {
	return &HLC{
		peerID: peerID,
		wallFn: time.Now,
	}
}

// NewHLCWithWall creates an HLC with a custom wall clock (for testing).
func NewHLCWithWall(peerID string, wallFn func() time.Time) *HLC {
	return &HLC{
		peerID: peerID,
		wallFn: wallFn,
	}
}

// Now generates a new timestamp guaranteed to be after the last one.
func (h *HLC) Now() Timestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := h.wallFn().UTC().Truncate(time.Millisecond)

	if now.After(h.last.Wall) {
		h.last = Timestamp{Wall: now, Counter: 0, PeerID: h.peerID}
	} else {
		h.last = Timestamp{Wall: h.last.Wall, Counter: h.last.Counter + 1, PeerID: h.peerID}
	}

	return h.last
}

// Update advances the clock using a remote timestamp, returning a new
// timestamp guaranteed to be after both the local state and the remote.
func (h *HLC) Update(remote Timestamp) Timestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := h.wallFn().UTC().Truncate(time.Millisecond)

	if now.After(h.last.Wall) && now.After(remote.Wall) {
		h.last = Timestamp{Wall: now, Counter: 0, PeerID: h.peerID}
	} else if h.last.Wall.After(remote.Wall) || h.last.Wall.Equal(remote.Wall) && h.last.Wall.After(now) {
		h.last = Timestamp{Wall: h.last.Wall, Counter: h.last.Counter + 1, PeerID: h.peerID}
	} else if remote.Wall.After(h.last.Wall) {
		h.last = Timestamp{Wall: remote.Wall, Counter: remote.Counter + 1, PeerID: h.peerID}
	} else {
		// h.last.Wall == remote.Wall
		counter := h.last.Counter
		if remote.Counter > counter {
			counter = remote.Counter
		}
		h.last = Timestamp{Wall: h.last.Wall, Counter: counter + 1, PeerID: h.peerID}
	}

	return h.last
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestHLC -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Run property-based tests**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestHLC_Property -v -count=1`
Expected: All property tests PASS (200 iterations each by default).

- [ ] **Step 4: Commit**

```bash
git add internal/crdt/hlc.go
git commit -m "feat: implement Hybrid Logical Clock with monotonicity guarantees"
```

---

## Task 4: LWW-Register -- Failing Tests

**Files:**
- Create: `internal/crdt/lww_register.go`
- Create: `internal/crdt/lww_register_test.go`

An LWW-Register holds a single value with a timestamp. On merge, the higher timestamp wins.

- [ ] **Step 1: Write the failing tests**

Create `internal/crdt/lww_register_test.go`:

```go
package crdt

import (
	"encoding/json"
	"testing"
	"time"

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
```

- [ ] **Step 2: Create minimal type stubs**

Create `internal/crdt/lww_register.go`:

```go
package crdt

// LWWRegister is a Last-Writer-Wins Register.
type LWWRegister[T any] struct{}

func NewLWWRegister[T any]() *LWWRegister[T]            { return nil }
func (r *LWWRegister[T]) Set(value T, ts Timestamp)     {}
func (r *LWWRegister[T]) Delete(ts Timestamp)            {}
func (r *LWWRegister[T]) Get() (T, bool)                 { var zero T; return zero, false }
func (r *LWWRegister[T]) IsDeleted() bool                { return false }
func (r *LWWRegister[T]) Merge(other *LWWRegister[T])    {}
func (r *LWWRegister[T]) GetTimestamp() Timestamp        { return Timestamp{} }
func (r *LWWRegister[T]) IsSet() bool                    { return false }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestLWWRegister -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit**

```bash
git add internal/crdt/lww_register.go internal/crdt/lww_register_test.go
git commit -m "test: add failing tests for LWW-Register"
```

---

## Task 5: LWW-Register -- Implementation

**Files:**
- Modify: `internal/crdt/lww_register.go`

- [ ] **Step 1: Implement the LWW-Register**

Replace `internal/crdt/lww_register.go`:

```go
package crdt

import "encoding/json"

// LWWRegister is a Last-Writer-Wins Register.
// The value with the highest timestamp wins on merge.
// Supports soft deletion via a deleted flag.
type LWWRegister[T any] struct {
	Value   T         `json:"value"`
	TS      Timestamp `json:"ts"`
	Deleted bool      `json:"deleted,omitempty"`
	set     bool      // tracks whether any value has been set
}

// NewLWWRegister creates a new empty LWW-Register.
func NewLWWRegister[T any]() *LWWRegister[T] {
	return &LWWRegister[T]{}
}

// Set updates the register value if ts is after the current timestamp.
func (r *LWWRegister[T]) Set(value T, ts Timestamp) {
	if !r.set || ts.After(r.TS) {
		r.Value = value
		r.TS = ts
		r.Deleted = false
		r.set = true
	}
}

// Delete marks the register as deleted if ts is after the current timestamp.
func (r *LWWRegister[T]) Delete(ts Timestamp) {
	if !r.set || ts.After(r.TS) {
		r.TS = ts
		r.Deleted = true
		r.set = true
	}
}

// Get returns the value and whether it is set (and not deleted).
func (r *LWWRegister[T]) Get() (T, bool) {
	if !r.set || r.Deleted {
		var zero T
		return zero, false
	}
	return r.Value, true
}

// IsDeleted returns whether the register has been soft-deleted.
func (r *LWWRegister[T]) IsDeleted() bool {
	return r.set && r.Deleted
}

// Timestamp returns the current timestamp of the register.
func (r *LWWRegister[T]) GetTimestamp() Timestamp {
	return r.TS
}

// IsSet returns whether the register has been initialized.
func (r *LWWRegister[T]) IsSet() bool {
	return r.set
}

// Merge incorporates the state from another register.
// The register with the higher timestamp wins.
func (r *LWWRegister[T]) Merge(other *LWWRegister[T]) {
	if other == nil || !other.set {
		return
	}
	if !r.set || other.TS.After(r.TS) {
		r.Value = other.Value
		r.TS = other.TS
		r.Deleted = other.Deleted
		r.set = true
	}
}

// MarshalJSON implements json.Marshaler.
func (r *LWWRegister[T]) MarshalJSON() ([]byte, error) {
	type alias struct {
		Value   T         `json:"value"`
		TS      Timestamp `json:"ts"`
		Deleted bool      `json:"deleted,omitempty"`
	}
	return json.Marshal(alias{Value: r.Value, TS: r.TS, Deleted: r.Deleted})
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *LWWRegister[T]) UnmarshalJSON(data []byte) error {
	type alias struct {
		Value   T         `json:"value"`
		TS      Timestamp `json:"ts"`
		Deleted bool      `json:"deleted,omitempty"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	r.Value = a.Value
	r.TS = a.TS
	r.Deleted = a.Deleted
	r.set = true
	return nil
}
```

- [ ] **Step 2: Run tests**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestLWWRegister -v -count=1`
Expected: All tests PASS including property tests.

- [ ] **Step 3: Commit**

```bash
git add internal/crdt/lww_register.go
git commit -m "feat: implement LWW-Register with merge, delete, and JSON serialization"
```

---

## Task 6: LWW-Map -- Failing Tests

**Files:**
- Create: `internal/crdt/lww_map.go`
- Create: `internal/crdt/lww_map_test.go`

An LWW-Map is a map of string keys to LWW-Registers. Merge operates per-key. Supports delta computation (entries changed since a given timestamp).

- [ ] **Step 1: Write the failing tests**

Create `internal/crdt/lww_map_test.go`:

```go
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
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/crdt/lww_map.go`:

```go
package crdt

// LWWMap is a map of string keys to LWW-Registers.
type LWWMap[T any] struct{}

func NewLWWMap[T any]() *LWWMap[T]                               { return nil }
func (m *LWWMap[T]) Set(key string, value T, ts Timestamp)       {}
func (m *LWWMap[T]) Delete(key string, ts Timestamp)              {}
func (m *LWWMap[T]) Get(key string) (T, bool)                     { var zero T; return zero, false }
func (m *LWWMap[T]) Keys() []string                               { return nil }
func (m *LWWMap[T]) Len() int                                     { return 0 }
func (m *LWWMap[T]) Merge(other *LWWMap[T])                       {}
func (m *LWWMap[T]) Delta(since Timestamp) *LWWMap[T]             { return nil }
func (m *LWWMap[T]) Snapshot() *LWWMap[T]                         { return nil }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestLWWMap -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit**

```bash
git add internal/crdt/lww_map.go internal/crdt/lww_map_test.go
git commit -m "test: add failing tests for LWW-Map with CRDT properties"
```

---

## Task 7: LWW-Map -- Implementation

**Files:**
- Modify: `internal/crdt/lww_map.go`

- [ ] **Step 1: Implement LWW-Map**

Replace `internal/crdt/lww_map.go`:

```go
package crdt

import (
	"encoding/json"
	"sort"
	"sync"
)

// LWWMap is a map of string keys to LWW-Registers.
// Safe for concurrent use.
type LWWMap[T any] struct {
	mu       sync.RWMutex
	entries  map[string]*LWWRegister[T]
}

// NewLWWMap creates a new empty LWW-Map.
func NewLWWMap[T any]() *LWWMap[T] {
	return &LWWMap[T]{
		entries: make(map[string]*LWWRegister[T]),
	}
}

// Set updates or inserts a key-value pair with the given timestamp.
func (m *LWWMap[T]) Set(key string, value T, ts Timestamp) {
	m.mu.Lock()
	defer m.mu.Unlock()

	reg, ok := m.entries[key]
	if !ok {
		reg = NewLWWRegister[T]()
		m.entries[key] = reg
	}
	reg.Set(value, ts)
}

// Delete marks a key as deleted with the given timestamp.
func (m *LWWMap[T]) Delete(key string, ts Timestamp) {
	m.mu.Lock()
	defer m.mu.Unlock()

	reg, ok := m.entries[key]
	if !ok {
		reg = NewLWWRegister[T]()
		m.entries[key] = reg
	}
	reg.Delete(ts)
}

// Get returns the value for a key if it exists and is not deleted.
func (m *LWWMap[T]) Get(key string) (T, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	reg, ok := m.entries[key]
	if !ok {
		var zero T
		return zero, false
	}
	return reg.Get()
}

// Keys returns all non-deleted keys, sorted.
func (m *LWWMap[T]) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0)
	for k, reg := range m.entries {
		if v, ok := reg.Get(); ok {
			_ = v
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// Len returns the number of non-deleted entries.
func (m *LWWMap[T]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, reg := range m.entries {
		if _, ok := reg.Get(); ok {
			count++
		}
	}
	return count
}

// Merge incorporates all entries from another map.
// Per-key LWW semantics: the register with the higher timestamp wins.
func (m *LWWMap[T]) Merge(other *LWWMap[T]) {
	if other == nil {
		return
	}

	other.mu.RLock()
	otherEntries := make(map[string]*LWWRegister[T], len(other.entries))
	for k, v := range other.entries {
		otherEntries[k] = v
	}
	other.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, otherReg := range otherEntries {
		localReg, ok := m.entries[key]
		if !ok {
			clone := NewLWWRegister[T]()
			clone.Merge(otherReg)
			m.entries[key] = clone
		} else {
			localReg.Merge(otherReg)
		}
	}
}

// Delta returns a new LWWMap containing only entries with timestamps
// strictly after the given timestamp.
func (m *LWWMap[T]) Delta(since Timestamp) *LWWMap[T] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := NewLWWMap[T]()
	for key, reg := range m.entries {
		if reg.IsSet() && reg.TS.After(since) {
			clone := NewLWWRegister[T]()
			clone.Merge(reg)
			result.entries[key] = clone
		}
	}
	return result
}

// Snapshot returns a deep copy of the entire map.
func (m *LWWMap[T]) Snapshot() *LWWMap[T] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := NewLWWMap[T]()
	for key, reg := range m.entries {
		clone := NewLWWRegister[T]()
		clone.Merge(reg)
		result.entries[key] = clone
	}
	return result
}

// MarshalJSON implements json.Marshaler.
func (m *LWWMap[T]) MarshalJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.entries)
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *LWWMap[T]) UnmarshalJSON(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries := make(map[string]*LWWRegister[T])
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	m.entries = entries
	return nil
}
```

- [ ] **Step 2: Run all tests**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestLWWMap -v -count=1`
Expected: All tests PASS including all 3 property tests.

- [ ] **Step 3: Commit**

```bash
git add internal/crdt/lww_map.go
git commit -m "feat: implement LWW-Map with per-key merge, delta, and snapshot"
```

---

## Task 8: LWW-Map Fuzz Tests

**Files:**
- Create: `internal/crdt/lww_map_fuzz_test.go`

- [ ] **Step 1: Write fuzz tests**

Create `internal/crdt/lww_map_fuzz_test.go`:

```go
package crdt

import (
	"encoding/json"
	"testing"
)

func FuzzLWWMapJSONRoundTrip(f *testing.F) {
	// Seed corpus
	f.Add(`{"key1":{"value":"hello","ts":{"wall":"2026-03-26T14:00:00Z","counter":0,"peer_id":"a"}}}`)
	f.Add(`{}`)
	f.Add(`{"k":{"value":"","ts":{"wall":"2026-01-01T00:00:00Z","counter":999,"peer_id":"peer-xyz"}}}`)

	f.Fuzz(func(t *testing.T, data string) {
		var m LWWMap[string]
		err := json.Unmarshal([]byte(data), &m)
		if err != nil {
			return // invalid JSON is fine, just skip
		}

		// Re-marshal
		marshaled, err := json.Marshal(&m)
		if err != nil {
			t.Fatalf("marshal after unmarshal failed: %v", err)
		}

		// Re-unmarshal
		var m2 LWWMap[string]
		err = json.Unmarshal(marshaled, &m2)
		if err != nil {
			t.Fatalf("unmarshal after marshal failed: %v", err)
		}

		// Values should match
		for _, k := range m.Keys() {
			v1, ok1 := m.Get(k)
			v2, ok2 := m2.Get(k)
			if ok1 != ok2 || v1 != v2 {
				t.Fatalf("round-trip mismatch for key %q", k)
			}
		}
	})
}

func FuzzTimestampJSONRoundTrip(f *testing.F) {
	f.Add(`{"wall":"2026-03-26T14:00:00Z","counter":0,"peer_id":"a"}`)
	f.Add(`{"wall":"2026-01-01T00:00:00.123456789Z","counter":4294967295,"peer_id":"very-long-peer-id"}`)

	f.Fuzz(func(t *testing.T, data string) {
		var ts Timestamp
		err := ts.UnmarshalJSON([]byte(data))
		if err != nil {
			return
		}

		marshaled, err := ts.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal after unmarshal failed: %v", err)
		}

		var ts2 Timestamp
		err = ts2.UnmarshalJSON(marshaled)
		if err != nil {
			t.Fatalf("unmarshal after marshal failed: %v", err)
		}

		if !ts.Wall.Equal(ts2.Wall) || ts.Counter != ts2.Counter || ts.PeerID != ts2.PeerID {
			t.Fatalf("round-trip mismatch")
		}
	})
}
```

- [ ] **Step 2: Run fuzz tests briefly**

Run: `cd /home/devsupreme/agenthive && go test -fuzz=FuzzLWWMapJSONRoundTrip -fuzztime=10s ./internal/crdt/`
Expected: No failures found in 10 seconds.

Run: `cd /home/devsupreme/agenthive && go test -fuzz=FuzzTimestampJSONRoundTrip -fuzztime=10s ./internal/crdt/`
Expected: No failures found in 10 seconds.

- [ ] **Step 3: Commit**

```bash
git add internal/crdt/lww_map_fuzz_test.go
git commit -m "test: add fuzz tests for JSON round-trip safety"
```

---

## Task 9: State Store -- Failing Tests

**Files:**
- Create: `internal/crdt/state_store.go`
- Create: `internal/crdt/state_store_test.go`

The State Store wraps three LWW-Maps: peers, routes, and config. It provides domain-typed accessors and handles persistence to/from disk.

- [ ] **Step 1: Write the failing tests**

Create `internal/crdt/state_store_test.go`:

```go
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

	store.SetPeer("server-1", PeerInfo{
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

	store.SetRoute("route-1", RouteRule{
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
	store.SetPeer("s1", PeerInfo{Name: "s1"})
	store.SetPeer("s2", PeerInfo{Name: "s2"})

	peers := store.ListPeers()
	assert.Len(t, peers, 2)
}

func TestStateStore_ListRoutes(t *testing.T) {
	store := NewStateStore("peer-a")
	store.SetRoute("r1", RouteRule{Match: RouteMatch{Project: "a"}})
	store.SetRoute("r2", RouteRule{Match: RouteMatch{Project: "b"}})

	routes := store.ListRoutes()
	assert.Len(t, routes, 2)
}

func TestStateStore_DeleteRoute(t *testing.T) {
	store := NewStateStore("peer-a")
	store.SetRoute("r1", RouteRule{Match: RouteMatch{Project: "a"}})
	store.DeleteRoute("r1")

	_, ok := store.GetRoute("r1")
	assert.False(t, ok)
}

func TestStateStore_Merge_TwoStoresConverge(t *testing.T) {
	storeA := NewStateStore("peer-a")
	storeB := NewStateStore("peer-b")

	storeA.SetPeer("s1", PeerInfo{Name: "from-a"})
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

	store.SetPeer("old", PeerInfo{Name: "old"})
	since := store.CurrentTimestamp()
	store.SetPeer("new", PeerInfo{Name: "new"})

	peersDelta, _, _ := store.DeltaMaps(since)

	_, oldExists := peersDelta.Get("old")
	_, newExists := peersDelta.Get("new")

	assert.False(t, oldExists, "old peer should not be in delta")
	assert.True(t, newExists, "new peer should be in delta")
}

func TestStateStore_Delta_IncludesDeletedEntries(t *testing.T) {
	store := NewStateStore("peer-a")

	store.SetRoute("r1", RouteRule{Match: RouteMatch{Project: "api"}})
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
	store.SetPeer("s1", PeerInfo{Name: "s1", Status: "online"})
	store.SetRoute("r1", RouteRule{Match: RouteMatch{Project: "api"}, Targets: []string{"phone"}})
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
```

- [ ] **Step 2: Create minimal stubs**

Create `internal/crdt/state_store.go`:

```go
package crdt

// PeerInfo holds metadata about a peer in the mesh.
type PeerInfo struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Addr     string `json:"addr,omitempty"`
	LinkType string `json:"link_type,omitempty"`
	LastSeen string `json:"last_seen,omitempty"`
}

// RouteMatch defines the selector for a routing rule.
type RouteMatch struct {
	Agent    string `json:"agent,omitempty"`
	Project  string `json:"project,omitempty"`
	Session  string `json:"session,omitempty"`
	Window   string `json:"window,omitempty"`
	Pane     string `json:"pane,omitempty"`
	Source   string `json:"source,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// RouteRule defines where to send notifications matching the selector.
type RouteRule struct {
	Match   RouteMatch `json:"match"`
	Targets []string   `json:"targets"`
	Action  string     `json:"action,omitempty"` // "notify" or "notify+action"
}

// ConfigEntry holds a config value with provenance metadata.
type ConfigEntry struct {
	Value     string `json:"value"`
	UpdatedBy string `json:"updated_by,omitempty"`
}

// StateStore holds the distributed state for the mesh.
type StateStore struct{}

func NewStateStore(peerID string) *StateStore                          { return nil }
func (s *StateStore) SetPeer(id string, info PeerInfo)                 {}
func (s *StateStore) GetPeer(id string) (PeerInfo, bool)               { return PeerInfo{}, false }
func (s *StateStore) ListPeers() map[string]PeerInfo                   { return nil }
func (s *StateStore) SetRoute(id string, rule RouteRule)               {}
func (s *StateStore) GetRoute(id string) (RouteRule, bool)             { return RouteRule{}, false }
func (s *StateStore) ListRoutes() map[string]RouteRule                 { return nil }
func (s *StateStore) DeleteRoute(id string)                            {}
func (s *StateStore) SetConfig(key string, value string)               {}
func (s *StateStore) GetConfig(key string) (string, bool)              { return "", false }
func (s *StateStore) CurrentTimestamp() Timestamp                      { return Timestamp{} }
func (s *StateStore) PeersMap() *LWWMap[PeerInfo]                      { return nil }
func (s *StateStore) RoutesMap() *LWWMap[RouteRule]                    { return nil }
func (s *StateStore) ConfigMap() *LWWMap[ConfigEntry]                  { return nil }
func (s *StateStore) MergeMaps(peers *LWWMap[PeerInfo], routes *LWWMap[RouteRule], config *LWWMap[ConfigEntry]) {}
func (s *StateStore) DeltaMaps(since Timestamp) (peers *LWWMap[PeerInfo], routes *LWWMap[RouteRule], config *LWWMap[ConfigEntry]) { return nil, nil, nil }
func (s *StateStore) SaveToFile(path string) error                     { return nil }
func (s *StateStore) LoadFromFile(path string) error                   { return nil }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestStateStore -v -count=1 2>&1 | head -30`
Expected: All tests FAIL.

- [ ] **Step 4: Commit**

```bash
git add internal/crdt/state_store.go internal/crdt/state_store_test.go
git commit -m "test: add failing tests for StateStore with persistence and merge"
```

---

## Task 10: State Store -- Core Methods

**Files:**
- Modify: `internal/crdt/state_store.go`

- [ ] **Step 1: Implement core CRUD methods**

Replace `internal/crdt/state_store.go`:

```go
package crdt

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// PeerInfo holds metadata about a peer in the mesh.
type PeerInfo struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Addr     string `json:"addr,omitempty"`
	LinkType string `json:"link_type,omitempty"`
	LastSeen string `json:"last_seen,omitempty"`
}

// RouteMatch defines the selector for a routing rule.
type RouteMatch struct {
	Agent    string `json:"agent,omitempty"`
	Project  string `json:"project,omitempty"`
	Session  string `json:"session,omitempty"`
	Window   string `json:"window,omitempty"`
	Pane     string `json:"pane,omitempty"`
	Source   string `json:"source,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// RouteRule defines where to send notifications matching the selector.
type RouteRule struct {
	Match   RouteMatch `json:"match"`
	Targets []string   `json:"targets"`
	Action  string     `json:"action,omitempty"` // "notify" or "notify+action"
}

// ConfigEntry holds a config value with provenance metadata.
type ConfigEntry struct {
	Value     string `json:"value"`
	UpdatedBy string `json:"updated_by,omitempty"`
}

// StateStore holds the distributed CRDT state for the mesh.
// It wraps three LWW-Maps: peers, routes, and config.
// CRITICAL: Merge operations happen at the LWWMap level to preserve
// CRDT timestamps. Never re-stamp entries with fresh local timestamps
// during merge -- that would make the last merger always win instead
// of the last writer.
type StateStore struct {
	clock  *HLC
	peerID string
	peers  *LWWMap[PeerInfo]
	routes *LWWMap[RouteRule]
	config *LWWMap[ConfigEntry]
}

// NewStateStore creates a new state store with the given peer ID for the HLC.
func NewStateStore(peerID string) *StateStore {
	return &StateStore{
		clock:  NewHLC(peerID),
		peerID: peerID,
		peers:  NewLWWMap[PeerInfo](),
		routes: NewLWWMap[RouteRule](),
		config: NewLWWMap[ConfigEntry](),
	}
}

func (s *StateStore) SetPeer(id string, info PeerInfo) {
	s.peers.Set(id, info, s.clock.Now())
}

func (s *StateStore) GetPeer(id string) (PeerInfo, bool) {
	return s.peers.Get(id)
}

func (s *StateStore) ListPeers() map[string]PeerInfo {
	result := make(map[string]PeerInfo)
	for _, k := range s.peers.Keys() {
		if v, ok := s.peers.Get(k); ok {
			result[k] = v
		}
	}
	return result
}

func (s *StateStore) SetRoute(id string, rule RouteRule) {
	s.routes.Set(id, rule, s.clock.Now())
}

func (s *StateStore) GetRoute(id string) (RouteRule, bool) {
	return s.routes.Get(id)
}

func (s *StateStore) ListRoutes() map[string]RouteRule {
	result := make(map[string]RouteRule)
	for _, k := range s.routes.Keys() {
		if v, ok := s.routes.Get(k); ok {
			result[k] = v
		}
	}
	return result
}

func (s *StateStore) DeleteRoute(id string) {
	s.routes.Delete(id, s.clock.Now())
}

func (s *StateStore) SetConfig(key string, value string) {
	s.config.Set(key, ConfigEntry{Value: value, UpdatedBy: s.peerID}, s.clock.Now())
}

func (s *StateStore) GetConfig(key string) (string, bool) {
	entry, ok := s.config.Get(key)
	if !ok {
		return "", false
	}
	return entry.Value, true
}

func (s *StateStore) CurrentTimestamp() Timestamp {
	return s.clock.Now()
}

// PeersMap returns the underlying CRDT map for peer state.
// Used for merge operations that preserve timestamps.
func (s *StateStore) PeersMap() *LWWMap[PeerInfo] {
	return s.peers.Snapshot()
}

// RoutesMap returns the underlying CRDT map for route state.
func (s *StateStore) RoutesMap() *LWWMap[RouteRule] {
	return s.routes.Snapshot()
}

// ConfigMap returns the underlying CRDT map for config state.
func (s *StateStore) ConfigMap() *LWWMap[ConfigEntry] {
	return s.config.Snapshot()
}

// MergeMaps merges remote CRDT maps into the local store.
// This preserves the original HLC timestamps from the remote peer,
// ensuring that the true last-writer wins (not the last merger).
func (s *StateStore) MergeMaps(peers *LWWMap[PeerInfo], routes *LWWMap[RouteRule], config *LWWMap[ConfigEntry]) {
	if peers != nil {
		s.peers.Merge(peers)
	}
	if routes != nil {
		s.routes.Merge(routes)
	}
	if config != nil {
		s.config.Merge(config)
	}
}

// DeltaMaps returns LWW-Maps containing only entries changed since the given timestamp.
// Includes tombstones (deleted entries) so deletions propagate.
func (s *StateStore) DeltaMaps(since Timestamp) (*LWWMap[PeerInfo], *LWWMap[RouteRule], *LWWMap[ConfigEntry]) {
	return s.peers.Delta(since), s.routes.Delta(since), s.config.Delta(since)
}

func (s *StateStore) SaveToFile(path string) error {
	type persistedState struct {
		Peers  *LWWMap[PeerInfo]      `json:"peers"`
		Routes *LWWMap[RouteRule]     `json:"routes"`
		Config *LWWMap[ConfigEntry]   `json:"config"`
	}

	data, err := json.MarshalIndent(persistedState{
		Peers:  s.peers,
		Routes: s.routes,
		Config: s.config,
	}, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *StateStore) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	type persistedState struct {
		Peers  *LWWMap[PeerInfo]      `json:"peers"`
		Routes *LWWMap[RouteRule]     `json:"routes"`
		Config *LWWMap[ConfigEntry]   `json:"config"`
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if state.Peers != nil {
		s.peers.Merge(state.Peers)
	}
	if state.Routes != nil {
		s.routes.Merge(state.Routes)
	}
	if state.Config != nil {
		s.config.Merge(state.Config)
	}
	return nil
}
```

- [ ] **Step 2: Run all tests**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestStateStore -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/crdt/state_store.go
git commit -m "feat: implement StateStore with peers, routes, config, persistence"
```

---

## Task 11: Multi-Peer Convergence Property Tests

**Files:**
- Create: `internal/crdt/state_store_property_test.go`

- [ ] **Step 1: Write property tests for multi-peer convergence**

Create `internal/crdt/state_store_property_test.go`:

```go
package crdt

import (
	"testing"

	"pgregory.net/rapid"
)

// Property: N peers making random edits, merging in any order, all converge
func TestStateStore_Property_MultiPeerConvergence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nPeers := rapid.IntRange(2, 5).Draw(t, "nPeers")

		stores := make([]*StateStore, nPeers)
		for i := 0; i < nPeers; i++ {
			stores[i] = NewStateStore(rapid.StringMatching(`peer-[a-z]`).Draw(t, "peerID"))
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
```

- [ ] **Step 2: Run property tests**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestStateStore_Property -v -count=1`
Expected: All property tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/crdt/state_store_property_test.go
git commit -m "test: add multi-peer convergence property tests for StateStore"
```

---

## Task 12: Concurrent Access Tests

**Files:**
- Create: `internal/crdt/state_store_concurrent_test.go`

- [ ] **Step 1: Write concurrent access tests**

Create `internal/crdt/state_store_concurrent_test.go`:

```go
package crdt

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLWWMap_ConcurrentSetAndGet(t *testing.T) {
	m := NewLWWMap[string]()
	clock := NewHLC("peer-a")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%10)
			m.Set(key, fmt.Sprintf("val-%d", i), clock.Now())
			m.Get(key)
		}(i)
	}
	wg.Wait()

	// All 10 keys should exist
	assert.Equal(t, 10, m.Len())
}

func TestLWWMap_ConcurrentMerge(t *testing.T) {
	m1 := NewLWWMap[string]()
	clock1 := NewHLC("peer-a")

	m2 := NewLWWMap[string]()
	clock2 := NewHLC("peer-b")

	// Populate both maps concurrently
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			m1.Set(fmt.Sprintf("k%d", i), "a", clock1.Now())
		}(i)
		go func(i int) {
			defer wg.Done()
			m2.Set(fmt.Sprintf("k%d", i), "b", clock2.Now())
		}(i)
	}
	wg.Wait()

	// Merge concurrently from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m1.Merge(m2)
		}()
	}
	wg.Wait()

	// Should have all 50 keys, no panics, no races
	assert.Equal(t, 50, m1.Len())
}

func TestStateStore_ConcurrentOperations(t *testing.T) {
	store := NewStateStore("peer-a")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			store.SetPeer(fmt.Sprintf("p%d", i%5), PeerInfo{Name: fmt.Sprintf("peer-%d", i)})
		}(i)
		go func(i int) {
			defer wg.Done()
			store.SetConfig(fmt.Sprintf("c%d", i%5), fmt.Sprintf("val-%d", i))
		}(i)
		go func(i int) {
			defer wg.Done()
			store.SetRoute(fmt.Sprintf("r%d", i%5), RouteRule{
				Match: RouteMatch{Project: fmt.Sprintf("proj-%d", i)},
			})
		}(i)
	}
	wg.Wait()

	// 5 unique keys each
	assert.Equal(t, 5, len(store.ListPeers()))
	assert.Equal(t, 5, len(store.ListRoutes()))
}
```

- [ ] **Step 2: Run with race detector**

Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestLWWMap_Concurrent -v -count=1`
Run: `cd /home/devsupreme/agenthive && go test -race ./internal/crdt/ -run TestStateStore_Concurrent -v -count=1`
Expected: All PASS, zero races detected.

- [ ] **Step 3: Commit**

```bash
git add internal/crdt/state_store_concurrent_test.go
git commit -m "test: add concurrent access tests for LWWMap and StateStore"
```

---

## Task 13: Run Full Test Suite and Verify

**Files:** None (verification only)

- [ ] **Step 1: Run all unit tests with race detector**

Run: `cd /home/devsupreme/agenthive && go test -race -v -count=1 ./internal/crdt/...`
Expected: All tests PASS. Zero race conditions.

- [ ] **Step 2: Run fuzz tests**

Run: `cd /home/devsupreme/agenthive && go test -fuzz=FuzzLWWMapJSONRoundTrip -fuzztime=30s ./internal/crdt/`
Expected: No failures.

Run: `cd /home/devsupreme/agenthive && go test -fuzz=FuzzTimestampJSONRoundTrip -fuzztime=30s ./internal/crdt/`
Expected: No failures.

- [ ] **Step 3: Check test coverage**

Run: `cd /home/devsupreme/agenthive && go test -race -coverprofile=coverage.out ./internal/crdt/ && go tool cover -func=coverage.out | tail -1`
Expected: Coverage > 80% for the crdt package.

- [ ] **Step 4: Commit coverage config to gitignore**

```bash
echo "coverage.out" >> /home/devsupreme/agenthive/.gitignore
git add .gitignore
git commit -m "chore: add coverage output to gitignore"
```

---

## Summary

| Task | Component | Type | Est. Time |
|------|-----------|------|-----------|
| 1 | Go module init | Setup | 3 min |
| 2 | HLC failing tests | Test | 5 min |
| 3 | HLC implementation | Code | 5 min |
| 4 | LWW-Register failing tests (+ assoc, delete-vs-live) | Test | 5 min |
| 5 | LWW-Register implementation | Code | 5 min |
| 6 | LWW-Map failing tests | Test | 5 min |
| 7 | LWW-Map implementation | Code | 5 min |
| 8 | LWW-Map fuzz tests | Test | 3 min |
| 9 | State Store failing tests (CRDT-preserving merge) | Test | 5 min |
| 10 | State Store core methods | Code | 5 min |
| 11 | Multi-peer convergence property tests (forced collisions) | Test | 5 min |
| 12 | Concurrent access tests | Test | 5 min |
| 13 | Full suite verification | Verify | 5 min |
| | **Total** | | **~66 min** |
