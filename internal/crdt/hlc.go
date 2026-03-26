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
