package daemon

import (
	"testing"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore() *crdt.StateStore {
	return crdt.NewStateStore("peer-a")
}

func TestRouter_MatchesByProject(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api-server"},
		Targets: []string{"phone", "laptop"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api-server",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"phone", "laptop"}, targets)
}

func TestRouter_MatchesBySource(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Source: "Codex"},
		Targets: []string{"desktop-only"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "any-project",
			Source:  "Codex",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"desktop-only"}, targets)
}

func TestRouter_MatchesByPriority(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Priority: "critical"},
		Targets: []string{"ALL"},
	})
	// Register known peers for ALL expansion
	store.SetPeer("phone", &crdt.PeerInfo{Name: "phone", Status: "online"})
	store.SetPeer("laptop", &crdt.PeerInfo{Name: "laptop", Status: "online"})
	store.SetPeer("peer-a", &crdt.PeerInfo{Name: "peer-a", Status: "online"})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project:  "api",
			Source:   "Claude",
			Message:  "FAILED",
			Priority: protocol.PriorityCritical,
		},
	}

	targets := router.Route(msg)
	// ALL expands to all known peers except self
	assert.ElementsMatch(t, []string{"phone", "laptop"}, targets)
}

func TestRouter_MatchesBySession(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Session: "refactor"},
		Targets: []string{"telegram"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Session: "refactor",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"telegram"}, targets)
}

func TestRouter_MultipleRulesMatch(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})
	store.SetRoute("r2", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Priority: "critical"},
		Targets: []string{"laptop"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project:  "api",
			Source:   "Claude",
			Message:  "FAILED",
			Priority: protocol.PriorityCritical,
		},
	}

	targets := router.Route(msg)
	// Both rules match, targets are deduplicated
	assert.ElementsMatch(t, []string{"phone", "laptop"}, targets)
}

func TestRouter_DefaultRuleCatchesUnmatched(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})
	store.SetRoute("default", &crdt.RouteRule{
		Match:   crdt.RouteMatch{}, // empty match = default
		Targets: []string{"laptop"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "frontend",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"laptop"}, targets)
}

func TestRouter_NoRulesMatch_ReturnsEmpty(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "frontend",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.Empty(t, targets)
}

func TestRouter_MultiFieldMatch_AllFieldsMustMatch(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api", Source: "Claude"},
		Targets: []string{"phone"},
	})

	router := NewRouter(store, "peer-a")

	// Message with matching project but wrong source
	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Codex",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.Empty(t, targets, "should not match when source differs")
}

func TestRouter_ActionRequest_RoutedByProject(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgActionRequest,
		SourceID: "peer-a",
		Payload: protocol.ActionRequestPayload{
			RequestID: "req-1",
			Tool:      "Bash",
			Command:   "rm -rf /tmp",
			Project:   "api",
			Source:    "Claude",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"phone"}, targets)
}

func TestRouter_DuplicateTargetsDeduped(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"phone", "laptop"},
	})
	store.SetRoute("r2", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Source: "Claude"},
		Targets: []string{"phone"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	// "phone" appears in both rules but should only appear once
	require.Len(t, targets, 2)
	assert.ElementsMatch(t, []string{"phone", "laptop"}, targets)
}

func TestRouter_ExcludesSelf(t *testing.T) {
	store := newTestStore()
	store.SetRoute("r1", &crdt.RouteRule{
		Match:   crdt.RouteMatch{Project: "api"},
		Targets: []string{"peer-a", "phone"},
	})

	router := NewRouter(store, "peer-a")

	msg := protocol.Message{
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	targets := router.Route(msg)
	assert.ElementsMatch(t, []string{"phone"}, targets)
}

func TestMatchesRule_SessionFilter(t *testing.T) {
	rule := crdt.RouteMatch{Session: "refactor"}

	// Matching session
	meta := messageMetadata{Session: "refactor", Project: "any"}
	assert.True(t, matchesRule(rule, meta))

	// Non-matching session
	meta2 := messageMetadata{Session: "deploy", Project: "any"}
	assert.False(t, matchesRule(rule, meta2))
}

func TestMatchesRule_PriorityFilter(t *testing.T) {
	rule := crdt.RouteMatch{Priority: "critical"}

	meta := messageMetadata{Priority: "critical", Project: "api"}
	assert.True(t, matchesRule(rule, meta))

	meta2 := messageMetadata{Priority: "info", Project: "api"}
	assert.False(t, matchesRule(rule, meta2))
}

func TestMatchesRule_WindowFilter(t *testing.T) {
	rule := crdt.RouteMatch{Window: "editor"}

	meta := messageMetadata{Window: "editor"}
	assert.True(t, matchesRule(rule, meta))

	meta2 := messageMetadata{Window: "terminal"}
	assert.False(t, matchesRule(rule, meta2))
}

func TestMatchesRule_PaneFilter(t *testing.T) {
	rule := crdt.RouteMatch{Pane: "%3"}

	meta := messageMetadata{Pane: "%3"}
	assert.True(t, matchesRule(rule, meta))

	meta2 := messageMetadata{Pane: "%0"}
	assert.False(t, matchesRule(rule, meta2))
}

func TestExtractMetadata_ActionRequestPayload(t *testing.T) {
	msg := protocol.Message{
		Type:     protocol.MsgActionRequest,
		SourceID: "peer-a",
		Payload: protocol.ActionRequestPayload{
			RequestID: "req-1",
			Tool:      "Bash",
			Command:   "ls",
			Project:   "api",
			Source:    "Claude",
			Pane:      "%2",
		},
	}

	meta := extractMetadata(msg)
	assert.Equal(t, "api", meta.Project)
	assert.Equal(t, "Claude", meta.Source)
	assert.Equal(t, "%2", meta.Pane)
	// ActionRequestPayload doesn't carry Session, Window, or Priority
	assert.Empty(t, meta.Session)
	assert.Empty(t, meta.Window)
	assert.Empty(t, meta.Priority)
}

func TestExtractMetadata_UnknownPayloadType(t *testing.T) {
	msg := protocol.Message{
		Type:     protocol.MsgHeartbeat,
		SourceID: "peer-a",
		Payload:  protocol.HeartbeatPayload{Uptime: 100},
	}

	meta := extractMetadata(msg)
	// Unknown payload type returns empty metadata
	assert.Equal(t, messageMetadata{}, meta)
}
