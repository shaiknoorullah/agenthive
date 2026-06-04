// Package router walks the CRDT routes table and returns the deduplicated
// set of target peer IDs for a given notification.
//
// The matcher uses the StateStore as its source of truth. Selector fields in
// a RouteMatch are treated as a conjunction: every set field must equal the
// notification's corresponding field. Unset fields act as wildcards.
//
// Target lists may contain the literal string "ALL" which expands to every
// peer in StateStore.ListPeers() except the local peer ID.
package router

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// Matcher walks the CRDT routes table and resolves notifications to a set of
// target peer IDs.
type Matcher struct {
	store *crdt.StateStore
	self  peer.ID
}

// NewMatcher constructs a Matcher backed by the supplied StateStore and
// keyed by the local peer ID (which is used to exclude self from "ALL"
// expansions).
func NewMatcher(store *crdt.StateStore, self peer.ID) *Matcher {
	panic("not implemented: router.NewMatcher")
}

// Targets returns the deduplicated set of target peer IDs (excluding self)
// for a notification, by walking all routes and unioning the targets whose
// RouteMatch selector accepts the notification's metadata.
//
// Selector fields are treated as conjunction: ALL set fields must equal the
// notification's corresponding fields. Unset selector fields are wildcards.
// Targets containing the literal string "ALL" expand to every peer in
// store.ListPeers() except self.
func (m *Matcher) Targets(notif protocols.Notification) []peer.ID {
	panic("not implemented: router.Matcher.Targets")
}
