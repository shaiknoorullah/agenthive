package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// Suppress unused import warning.
var _ = protocol.MsgNotification

// Queue is a disk-backed message queue for offline peers.
type Queue struct{}

func NewQueue(dir string) (*Queue, error)                          { return nil, nil }
func (q *Queue) Enqueue(peerID string, msg protocol.Message) error { return nil }
func (q *Queue) Drain(peerID string) ([]protocol.Message, error)   { return nil, nil }
func (q *Queue) Depth(peerID string) int                           { return 0 }
