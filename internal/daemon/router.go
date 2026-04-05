package daemon

import (
	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocol"
)

// Router evaluates routing rules from the CRDT state store against
// message metadata and returns matching target peer IDs.
// Rules are evaluated independently; a message may match multiple rules.
// An empty match field is a wildcard (matches anything).
// A fully empty match (all fields empty) is the "default" rule.
// "ALL" as a target expands to all known peers except self.
type Router struct {
	store  *crdt.StateStore
	selfID string
}

// NewRouter creates a new message router.
func NewRouter(store *crdt.StateStore, selfID string) *Router {
	return &Router{store: store, selfID: selfID}
}

// Route evaluates all routing rules and returns deduplicated target peer IDs.
// Returns an empty slice if no rules match.
func (r *Router) Route(msg protocol.Message) []string {
	meta := extractMetadata(msg)
	rules := r.store.ListRoutes()

	seen := make(map[string]bool)
	var targets []string
	matched := false

	for id, rule := range rules {
		if id == "default" {
			continue // evaluate default rule only if nothing else matches
		}
		if matchesRule(rule.Match, meta) {
			matched = true
			for _, target := range r.expandTargets(rule.Targets) {
				if !seen[target] {
					seen[target] = true
					targets = append(targets, target)
				}
			}
		}
	}

	// If no specific rules matched, try the default rule
	if !matched {
		if defaultRule, ok := rules["default"]; ok {
			for _, target := range r.expandTargets(defaultRule.Targets) {
				if !seen[target] {
					seen[target] = true
					targets = append(targets, target)
				}
			}
		}
	}

	return targets
}

// messageMetadata holds extracted routing-relevant fields from a message.
type messageMetadata struct {
	Project  string
	Source   string
	Session  string
	Window   string
	Pane     string
	Priority string
}

// extractMetadata pulls routing-relevant fields from a message payload.
func extractMetadata(msg protocol.Message) messageMetadata {
	switch p := msg.Payload.(type) {
	case protocol.NotificationPayload:
		return messageMetadata{
			Project:  p.Project,
			Source:   p.Source,
			Session:  p.Session,
			Window:   p.Window,
			Pane:     p.Pane,
			Priority: string(p.Priority),
		}
	case protocol.ActionRequestPayload:
		return messageMetadata{
			Project: p.Project,
			Source:  p.Source,
			Pane:    p.Pane,
		}
	default:
		return messageMetadata{}
	}
}

// matchesRule checks if a message's metadata matches a routing rule.
// Empty fields in the rule match are wildcards.
// All non-empty fields must match for the rule to apply.
func matchesRule(match crdt.RouteMatch, meta messageMetadata) bool {
	if match.Project != "" && match.Project != meta.Project {
		return false
	}
	if match.Source != "" && match.Source != meta.Source {
		return false
	}
	if match.Session != "" && match.Session != meta.Session {
		return false
	}
	if match.Window != "" && match.Window != meta.Window {
		return false
	}
	if match.Pane != "" && match.Pane != meta.Pane {
		return false
	}
	if match.Priority != "" && match.Priority != meta.Priority {
		return false
	}
	return true
}

// expandTargets expands "ALL" to all known peers (except self) and
// filters out self from explicit targets.
func (r *Router) expandTargets(targets []string) []string {
	var result []string
	for _, t := range targets {
		if t == "ALL" {
			peers := r.store.ListPeers()
			for id := range peers {
				if id != r.selfID {
					result = append(result, id)
				}
			}
		} else if t != r.selfID {
			result = append(result, t)
		}
	}
	return result
}
