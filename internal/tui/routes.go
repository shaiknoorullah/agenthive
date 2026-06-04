package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
)

// RoutesUpdateMsg is dispatched by the parent App when fresh route data has
// been received from the daemon.
type RoutesUpdateMsg struct {
	Routes map[string]crdt.RouteRule `json:"routes"`
}

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
	ID       string           `json:"id"`
	Match    crdt.RouteMatch  `json:"match"`
	Targets  []string         `json:"targets"`
	Action   string           `json:"action"`
}

// NewRoutesModel constructs an empty RoutesModel using the supplied styles.
func NewRoutesModel(styles Styles) RoutesModel {
	panic("not implemented: tui.NewRoutesModel")
}

// Init returns the initial command for the routes tab.
func (m RoutesModel) Init() tea.Cmd {
	panic("not implemented: tui.RoutesModel.Init")
}

// Update handles a tea.Msg and returns the next RoutesModel state plus an
// optional follow-up command.
func (m RoutesModel) Update(msg tea.Msg) (RoutesModel, tea.Cmd) {
	panic("not implemented: tui.RoutesModel.Update")
}

// View renders the routes tab as a string.
func (m RoutesModel) View() string {
	panic("not implemented: tui.RoutesModel.View")
}
