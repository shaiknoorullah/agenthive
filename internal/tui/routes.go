package tui

import tea "github.com/charmbracelet/bubbletea"

// RouteDisplay holds route data for rendering.
type RouteDisplay struct {
	ID       string
	Selector string
	Targets  string
	Action   string
}

// RoutesUpdateMsg is sent when the route list changes.
type RoutesUpdateMsg struct {
	Routes []RouteDisplay
}

// RouteDeleteMsg requests deletion of a route.
type RouteDeleteMsg struct {
	RouteID string
}

// RouteAddMsg requests addition of a new route.
type RouteAddMsg struct {
	Selector string
	Targets  string
}

// RoutesModel is the bubbletea model for the Routes tab.
type RoutesModel struct {
	routes []RouteDisplay
	cursor int
	adding bool
	input  string
	width  int
	height int
	styles *Styles
}

func NewRoutesModel(routes []RouteDisplay, styles *Styles) RoutesModel { return RoutesModel{} }
func (m RoutesModel) Init() tea.Cmd                                     { return nil }
func (m RoutesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)          { return m, nil }
func (m RoutesModel) View() string                                      { return "" }
