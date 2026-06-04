// Package tui — routes.go owns the "routes" tab Model.
//
// RoutesModel renders the deterministic, ID-sorted view of every CRDT
// RouteRule currently in the StateStore. Users move the cursor with
// j/k or the arrow keys; pressing `a` surfaces an "add route" intent
// and pressing `d` surfaces a "delete the highlighted route" intent.
// The actual modal flow for composing a new route lives in the parent
// App for v0.1.0 — this tab only fires typed request messages that the
// App can intercept and lift into a libp2p CRDT mutation.
package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
)

// RoutesUpdateMsg is dispatched by the parent App when fresh route data has
// been received from the daemon.
type RoutesUpdateMsg struct {
	Routes map[string]crdt.RouteRule `json:"routes"`
}

// RouteDeleteRequestMsg is emitted by RoutesModel when the user presses `d`
// on a populated row. The parent App is responsible for translating it into
// a CRDT mutation and a daemon RPC.
type RouteDeleteRequestMsg struct {
	ID string `json:"id"`
}

// RouteAddRequestMsg is emitted by RoutesModel when the user presses `a`.
// For v0.1.0 the TUI exposes only the intent; the actual selector + target
// composition lives in the App-owned modal that the parent attaches.
type RouteAddRequestMsg struct{}

// RoutesModel renders the "routes" tab: a list of route rules. The user can
// move the cursor and trigger add / delete via key bindings.
type RoutesModel struct {
	styles Styles
	width  int
	height int
	cursor int
	routes []routeRow
}

// routeRow is the rendered representation of a single route rule.
type routeRow struct {
	ID      string          `json:"id"`
	Match   crdt.RouteMatch `json:"match"`
	Targets []string        `json:"targets"`
	Action  string          `json:"action"`
}

// NewRoutesModel constructs an empty RoutesModel using the supplied styles.
// The model holds no rules, has a zero cursor, and zero width/height until
// the first WindowSizeMsg arrives. Route data flows in via RoutesUpdateMsg.
func NewRoutesModel(styles Styles) RoutesModel {
	return RoutesModel{styles: styles}
}

// Init returns the initial command for the routes tab. The routes tab has
// no background work of its own — it is driven exclusively by inbound
// RoutesUpdateMsg values dispatched from the parent App.
func (m RoutesModel) Init() tea.Cmd { return nil }

// Update handles a tea.Msg and returns the next RoutesModel state plus an
// optional follow-up command.
//
// Recognised messages:
//   - RoutesUpdateMsg     replaces the rendered route list (sorted by ID)
//   - tea.WindowSizeMsg   records the viewport dimensions
//   - tea.KeyMsg          j / down  : cursor down
//                         k / up    : cursor up
//                         d         : emit RouteDeleteRequestMsg{ID}
//                         a         : emit RouteAddRequestMsg{}
//
// Cursor moves are clamped to [0, len(routes)-1]. Pressing `d` with no
// routes is a no-op and emits no command. The cursor is also clamped after
// a RoutesUpdateMsg so a shrunken route set never leaves the cursor past
// the new last row.
func (m RoutesModel) Update(msg tea.Msg) (RoutesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case RoutesUpdateMsg:
		m.routes = routeRowsFromMap(msg.Routes)
		m.cursor = routesClampCursor(m.cursor, len(m.routes))
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.routes)-1 {
				m.cursor++
			}
			return m, nil
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "d":
			if len(m.routes) == 0 {
				return m, nil
			}
			if m.cursor < 0 || m.cursor >= len(m.routes) {
				return m, nil
			}
			id := m.routes[m.cursor].ID
			return m, func() tea.Msg { return RouteDeleteRequestMsg{ID: id} }
		case "a":
			return m, func() tea.Msg { return RouteAddRequestMsg{} }
		}
		return m, nil
	}
	return m, nil
}

// View renders the routes tab as a string. The output has a single header
// row and one row per rule, where the cursor row is prefixed with ">". When
// the route set is empty the View shows a deliberate empty-state hint
// pointing the user at the `a` key so they know how to bootstrap.
func (m RoutesModel) View() string {
	var b strings.Builder

	header := fmt.Sprintf("  %-20s  %-32s  %-14s  %s",
		"ID", "SELECTOR", "ACTION", "TARGETS")
	b.WriteString(m.styles.Header.Render(header))
	b.WriteString("\n")

	if len(m.routes) == 0 {
		b.WriteString(m.styles.Subtle.Render("  No routes configured. Press 'a' to add one."))
		b.WriteString("\n")
		return b.String()
	}

	for i, r := range m.routes {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		row := fmt.Sprintf("%s %-20s  %-32s  %-14s  %s",
			marker,
			routesTruncate(r.ID, 20),
			routesTruncate(renderRouteSelector(r.Match), 32),
			routesTruncate(routeActionLabel(r.Action), 14),
			strings.Join(r.Targets, ","),
		)
		if i == m.cursor {
			row = m.styles.Cursor.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	return b.String()
}

// Cursor reports the index of the currently highlighted row. Exposed for
// tests that drive the model directly without going through View.
func (m RoutesModel) Cursor() int { return m.cursor }

// Width reports the viewport width last reported via tea.WindowSizeMsg.
func (m RoutesModel) Width() int { return m.width }

// Height reports the viewport height last reported via tea.WindowSizeMsg.
func (m RoutesModel) Height() int { return m.height }

// Routes returns the current snapshot of rendered routes (sorted by ID).
// The returned slice is a fresh copy so callers cannot mutate the model.
func (m RoutesModel) Routes() []crdt.RouteRule {
	out := make([]crdt.RouteRule, 0, len(m.routes))
	for _, r := range m.routes {
		out = append(out, crdt.RouteRule{
			Match:   r.Match,
			Targets: append([]string(nil), r.Targets...),
			Action:  r.Action,
		})
	}
	return out
}

// routesClampCursor keeps a cursor inside [0, n-1]. Returns 0 if n is zero.
// Named with a "routes" prefix so it never collides with sibling helpers
// added by the peers/actions/logs tab implementations.
func routesClampCursor(cursor, n int) int {
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

// routesTruncate returns s shortened to at most n runes, with an ellipsis
// when truncation actually happens.
func routesTruncate(s string, n int) string {
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

// routeRowsFromMap converts the CRDT route map into a stable, ID-sorted
// slice suitable for rendering. The route IDs alone determine sort order
// so two TUIs viewing the same CRDT see byte-identical output.
func routeRowsFromMap(in map[string]crdt.RouteRule) []routeRow {
	rows := make([]routeRow, 0, len(in))
	for id, rule := range in {
		rows = append(rows, routeRow{
			ID:      id,
			Match:   rule.Match,
			Targets: append([]string(nil), rule.Targets...),
			Action:  rule.Action,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

// renderRouteSelector formats a RouteMatch as a comma-separated key:value
// selector string in a stable field order. Empty fields are skipped so the
// rendered selector matches what an `agenthive routes add` user would type.
func renderRouteSelector(m crdt.RouteMatch) string {
	parts := make([]string, 0, 7)
	if m.Agent != "" {
		parts = append(parts, "agent:"+m.Agent)
	}
	if m.Project != "" {
		parts = append(parts, "project:"+m.Project)
	}
	if m.Session != "" {
		parts = append(parts, "session:"+m.Session)
	}
	if m.Window != "" {
		parts = append(parts, "window:"+m.Window)
	}
	if m.Pane != "" {
		parts = append(parts, "pane:"+m.Pane)
	}
	if m.Source != "" {
		parts = append(parts, "source:"+m.Source)
	}
	if m.Priority != "" {
		parts = append(parts, "priority:"+m.Priority)
	}
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, ",")
}

// routeActionLabel maps an empty action onto the default "notify" so the
// rendered table never shows a blank action column.
func routeActionLabel(action string) string {
	if action == "" {
		return "notify"
	}
	return action
}
