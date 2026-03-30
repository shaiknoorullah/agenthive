package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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

// NewRoutesModel creates a new Routes tab model.
func NewRoutesModel(routes []RouteDisplay, styles *Styles) RoutesModel {
	if routes == nil {
		routes = []RouteDisplay{}
	}
	return RoutesModel{
		routes: routes,
		cursor: 0,
		styles: styles,
	}
}

func (m RoutesModel) Init() tea.Cmd { return nil }

func (m RoutesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.adding {
			return m.updateAddMode(msg)
		}
		switch msg.String() {
		case "down", "j":
			if m.cursor < len(m.routes)-1 {
				m.cursor++
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "a":
			m.adding = true
			m.input = ""
		case "d":
			if len(m.routes) > 0 && m.cursor < len(m.routes) {
				routeID := m.routes[m.cursor].ID
				return m, func() tea.Msg {
					return RouteDeleteMsg{RouteID: routeID}
				}
			}
		}
	case RoutesUpdateMsg:
		m.routes = msg.Routes
		if m.cursor >= len(m.routes) {
			m.cursor = max(0, len(m.routes)-1)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m RoutesModel) updateAddMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.adding = false
		m.input = ""
	case tea.KeyEnter:
		if m.input != "" {
			parts := strings.SplitN(m.input, " -> ", 2)
			if len(parts) == 2 {
				m.adding = false
				input := m.input
				m.input = ""
				return m, func() tea.Msg {
					p := strings.SplitN(input, " -> ", 2)
					return RouteAddMsg{Selector: p[0], Targets: p[1]}
				}
			}
		}
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	case tea.KeyRunes:
		m.input += string(msg.Runes)
	}
	return m, nil
}

func (m RoutesModel) View() string {
	if len(m.routes) == 0 && !m.adding {
		return "  No routes configured.\n\n  Press 'a' to add a routing rule.\n"
	}

	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "  %-30s %-5s %s\n", "SELECTOR", "->", "TARGETS")
	fmt.Fprintf(&b, "  %s\n", strings.Repeat("-", 60))

	for i, r := range m.routes {
		line := fmt.Sprintf("  %-30s  ->  %s", r.Selector, r.Targets)

		if i == m.cursor {
			line = m.styles.SelectedRow.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	if m.adding {
		b.WriteString("\n")
		fmt.Fprintf(&b, "  Add route: %s_\n", m.input)
		b.WriteString("  Format: <selector> -> <targets>  (Esc to cancel)\n")
	}

	return b.String()
}
