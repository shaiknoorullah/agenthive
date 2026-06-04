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

// allTargetToken is the literal string in a RouteRule.Targets slice that means
// "expand to every known peer except self".
const allTargetToken = "ALL"

// Matcher walks the CRDT routes table and resolves notifications to a set of
// target peer IDs.
type Matcher struct {
	store *crdt.StateStore
	self  peer.ID
}

// NewMatcher constructs a Matcher backed by the supplied StateStore and
// keyed by the local peer ID (which is used to exclude self from "ALL"
// expansions and from the final result).
func NewMatcher(store *crdt.StateStore, self peer.ID) *Matcher {
	return &Matcher{
		store: store,
		self:  self,
	}
}

// Targets returns the deduplicated set of target peer IDs (excluding self)
// for a notification, by walking all routes and unioning the targets whose
// RouteMatch selector accepts the notification's metadata.
//
// Selector fields are treated as conjunction: ALL set fields must equal the
// notification's corresponding fields. Unset selector fields are wildcards.
// Targets containing the literal string "ALL" expand to every peer in
// store.ListPeers() except self. Targets that fail to decode as a libp2p
// peer ID are silently skipped so a malformed route entry cannot crash the
// dispatch loop.
func (m *Matcher) Targets(notif protocols.Notification) []peer.ID {
	if m == nil || m.store == nil {
		return nil
	}

	// Use a set keyed by the binary peer.ID form so duplicates introduced by
	// overlapping routes (or ALL + an explicit listing) collapse to one entry.
	seen := make(map[peer.ID]struct{})

	for _, rule := range m.store.ListRoutes() {
		if !selectorMatches(rule.Match, notif) {
			continue
		}
		for _, raw := range rule.Targets {
			if raw == allTargetToken {
				for encoded := range m.store.ListPeers() {
					pid, err := peer.Decode(encoded)
					if err != nil {
						continue
					}
					if pid == m.self {
						continue
					}
					seen[pid] = struct{}{}
				}
				continue
			}
			pid, err := peer.Decode(raw)
			if err != nil {
				// A malformed target should not prevent the rest of the rule
				// from being applied.
				continue
			}
			if pid == m.self {
				continue
			}
			seen[pid] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil
	}
	out := make([]peer.ID, 0, len(seen))
	for pid := range seen {
		out = append(out, pid)
	}
	return out
}

// selectorMatches reports whether every set field of match equals the
// corresponding field of notif. Unset (zero-value) fields in the selector
// act as wildcards. Agent/Window/Pane are accepted in the selector grammar
// but have no corresponding Notification fields in v0.1.0; if those are set
// in the selector, the rule is treated as non-matching for plain
// notifications (action-routing covers those fields).
func selectorMatches(match crdt.RouteMatch, notif protocols.Notification) bool {
	if match.Project != "" && match.Project != notif.Project {
		return false
	}
	if match.Session != "" && match.Session != notif.SessionID {
		return false
	}
	if match.Source != "" && match.Source != notif.Source {
		return false
	}
	if match.Priority != "" && match.Priority != notif.Priority {
		return false
	}
	// Agent/Window/Pane are not part of a plain Notification's metadata, so
	// any rule that pins one of those fields must not fire on a notification.
	if match.Agent != "" || match.Window != "" || match.Pane != "" {
		return false
	}
	return true
}
