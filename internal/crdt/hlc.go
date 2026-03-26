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
