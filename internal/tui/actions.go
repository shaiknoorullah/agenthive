package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// ActionsUpdateMsg is dispatched by the parent App when fresh pending-action
// data has been received from the daemon. The Actions slice is rendered
// in-order: the daemon is responsible for sending it in a stable order
// (typically oldest-first).
type ActionsUpdateMsg struct {
	Actions []protocols.ActionRequest `json:"actions"`
}

// ActionDecisionMsg is emitted by ActionsModel.Update when the user approves
// (y) or denies (n) the currently highlighted action. The parent App is
// expected to take this message and persist the decision through whichever
// action queue it owns (hooks.Queue, mock, etc.).
//
// Decision is always one of "allow" or "deny" — ActionsModel never produces
// "allow-always"; that is reserved for explicit operator workflows.
type ActionDecisionMsg struct {
	ActionID  string `json:"action_id"`
	SessionID string `json:"session_id"`
	Decision  string `json:"decision"`
	DecidedBy string `json:"decided_by"`
}

// ActionDecider is the minimal interface the ActionsModel uses to persist a
// decision. *hooks.Queue satisfies it via its WriteResponse method, but tests
// (and alternative front-ends) can supply any implementation. Errors returned
// by WriteResponse are intentionally not surfaced to the user — the parent
// App owns logging and retry semantics; ActionsModel just records "we tried".
type ActionDecider interface {
	WriteResponse(protocols.ActionResponse) error
}

// ActionsModel renders the "actions" tab: a list of pending ActionRequests
// awaiting approval. Pressing y / n approves or denies the highlighted row,
// emits an ActionDecisionMsg as a tea.Cmd, and (if a queue has been wired in
// via WithQueue) writes the matching ActionResponse through that queue.
type ActionsModel struct {
	styles  Styles
	width   int
	height  int
	cursor  int
	pending []protocols.ActionRequest
	queue   ActionDecider
}

// NewActionsModel constructs an empty ActionsModel using the supplied styles.
// The model has no pending actions, a zero cursor, and zero width/height until
// the first WindowSizeMsg arrives.
func NewActionsModel(styles Styles) ActionsModel {
	return ActionsModel{styles: styles}
}

// WithQueue returns a copy of the model wired to write decisions through q.
// Callers that do not wire a queue still get an ActionDecisionMsg from
// Update, so the parent App can route the decision itself.
func (m ActionsModel) WithQueue(q ActionDecider) ActionsModel {
	m.queue = q
	return m
}

// Init returns the initial command for the actions tab. The actions tab has
// no background work of its own — it is driven exclusively by
// ActionsUpdateMsg values dispatched from the parent App.
func (m ActionsModel) Init() tea.Cmd { return nil }

// Update handles a tea.Msg and returns the next ActionsModel state plus an
// optional follow-up command.
//
// Recognised messages:
//   - ActionsUpdateMsg          replaces the rendered pending-action list
//   - tea.WindowSizeMsg         records the viewport dimensions
//   - tea.KeyMsg                j / down moves the cursor down; k / up moves
//                               up; y approves the highlighted action; n
//                               denies it
//
// Cursor moves are clamped to [0, len(pending)-1] so they never escape the
// visible row range.
func (m ActionsModel) Update(msg tea.Msg) (ActionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ActionsUpdateMsg:
		m.pending = append(m.pending[:0:0], msg.Actions...)
		m.cursor = actionsClampCursor(m.cursor, len(m.pending))
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.pending)-1 {
				m.cursor++
			}
			return m, nil
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "y":
			return m.decide("allow")
		case "n":
			return m.decide("deny")
		}
		return m, nil
	}
	return m, nil
}

// decide records a decision for the highlighted row: it pops the row from
// the pending slice, clamps the cursor, writes through the wired queue (if
// any), and emits an ActionDecisionMsg as the returned tea.Cmd. If the
// pending list is empty the call is a no-op that returns a nil-yielding cmd.
func (m ActionsModel) decide(decision string) (ActionsModel, tea.Cmd) {
	if len(m.pending) == 0 {
		return m, func() tea.Msg { return nil }
	}
	row := m.pending[m.cursor]

	// Remove the decided action from the visible list. Build a fresh slice so
	// we don't mutate any backing array a previous Update may share.
	next := make([]protocols.ActionRequest, 0, len(m.pending)-1)
	next = append(next, m.pending[:m.cursor]...)
	next = append(next, m.pending[m.cursor+1:]...)
	m.pending = next
	m.cursor = actionsClampCursor(m.cursor, len(m.pending))

	resp := protocols.ActionResponse{
		ActionID:  row.ActionID,
		Decision:  decision,
		DecidedBy: "tui",
	}
	if m.queue != nil {
		// Errors here are intentionally swallowed; the parent App owns
		// logging policy. The decision message is still emitted so the App
		// can log on its own.
		_ = m.queue.WriteResponse(resp)
	}

	dec := ActionDecisionMsg{
		ActionID:  row.ActionID,
		SessionID: row.SessionID,
		Decision:  decision,
		DecidedBy: "tui",
	}
	return m, func() tea.Msg { return dec }
}

// View renders the actions tab as a string. The output has a title line, a
// column header, and one row per pending action. The cursor row is prefixed
// with ">" and styled with the Cursor style.
func (m ActionsModel) View() string {
	var b strings.Builder

	b.WriteString(m.styles.Title.Render("Pending Actions"))
	b.WriteString("\n")

	header := fmt.Sprintf("  %-12s  %-10s  %-30s  %s",
		"ID", "TOOL", "INPUT", "EXPIRES")
	b.WriteString(m.styles.Header.Render(header))
	b.WriteString("\n")

	if len(m.pending) == 0 {
		b.WriteString(m.styles.Subtle.Render("  (No pending actions. Press y to approve or n to deny when an action arrives.)"))
		b.WriteString("\n")
		return b.String()
	}

	for i, a := range m.pending {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		row := fmt.Sprintf("%s %-12s  %-10s  %-30s  %s",
			marker,
			actionsTruncate(a.ActionID, 12),
			actionsTruncate(a.ToolName, 10),
			actionsTruncate(a.ToolInput, 30),
			formatExpiresAt(a.ExpiresAt),
		)
		if i == m.cursor {
			row = m.styles.Cursor.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

// Cursor reports the index of the currently highlighted row.
func (m ActionsModel) Cursor() int { return m.cursor }

// Width reports the viewport width last reported via tea.WindowSizeMsg.
func (m ActionsModel) Width() int { return m.width }

// Height reports the viewport height last reported via tea.WindowSizeMsg.
func (m ActionsModel) Height() int { return m.height }

// Pending returns a fresh copy of the pending action list so callers cannot
// mutate the model's internal state.
func (m ActionsModel) Pending() []protocols.ActionRequest {
	out := make([]protocols.ActionRequest, len(m.pending))
	copy(out, m.pending)
	return out
}

// formatExpiresAt renders an expiry timestamp as a compact UTC RFC-3339 stamp
// without sub-second precision so the rendered width and content are stable
// across runs (used by golden-file tests). A zero time renders as "—".
func formatExpiresAt(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format(time.RFC3339)
}

// actionsClampCursor keeps a cursor inside [0, n-1]. Returns 0 when n is
// zero. The function is intentionally local to this file (rather than shared
// with peers.go or routes.go) so the actions package is independently
// buildable, which the v0.1.0 implementation plan calls for explicitly.
func actionsClampCursor(cursor, n int) int {
	if n <= 0 {
		return 0
	}
	if cursor < 0 {
		return 0
	}
	if cursor >= n {
		return n - 1
	}
	return cursor
}

// actionsTruncate returns s shortened to at most n runes, with an ellipsis
// suffix when truncation actually happens. It is the actions-tab analogue of
// the truncate helpers used by sibling tab files; keeping it local keeps the
// file independently buildable.
func actionsTruncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
