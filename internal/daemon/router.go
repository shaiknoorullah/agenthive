package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// Suppress unused import warnings.
var _ = (*crdt.StateStore)(nil)
var _ = protocol.MsgNotification

// Router evaluates routing rules against message metadata
// and returns matching target peer IDs.
type Router struct{}

func NewRouter(store *crdt.StateStore, selfID string) *Router { return nil }
func (r *Router) Route(msg protocol.Message) []string         { return nil }
