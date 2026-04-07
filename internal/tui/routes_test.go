package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func init() {
	lipgloss.DefaultRenderer().SetColorProfile(termenv.Ascii)
}

func TestRoutesModel_InitialState(t *testing.T) {
	routes := []RouteDisplay{
		{ID: "r1", Selector: "project:api-server", Targets: "phone, laptop"},
		{ID: "r2", Selector: "session:refactor", Targets: "telegram"},
		{ID: "r3", Selector: "priority:critical", Targets: "ALL"},
	}
	m := NewRoutesModel(routes, NewStyles())

	assert.Equal(t, 0, m.cursor)
	assert.Equal(t, 3, len(m.routes))
	assert.False(t, m.adding)
}

func TestRoutesModel_CursorNavigation(t *testing.T) {
	routes := []RouteDisplay{
		{ID: "r1", Selector: "a", Targets: "x"},
		{ID: "r2", Selector: "b", Targets: "y"},
		{ID: "r3", Selector: "c", Targets: "z"},
	}
	m := NewRoutesModel(routes, NewStyles())

	// Down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	rm := updated.(RoutesModel)
	assert.Equal(t, 1, rm.cursor)

	// Down again
	updated, _ = rm.Update(tea.KeyMsg{Type: tea.KeyDown})
	rm = updated.(RoutesModel)
	assert.Equal(t, 2, rm.cursor)

	// Clamp at end
	updated, _ = rm.Update(tea.KeyMsg{Type: tea.KeyDown})
	rm = updated.(RoutesModel)
	assert.Equal(t, 2, rm.cursor)

	// Up
	updated, _ = rm.Update(tea.KeyMsg{Type: tea.KeyUp})
	rm = updated.(RoutesModel)
	assert.Equal(t, 1, rm.cursor)

	// Clamp at start
	updated, _ = rm.Update(tea.KeyMsg{Type: tea.KeyUp})
	rm = updated.(RoutesModel)
	updated, _ = rm.Update(tea.KeyMsg{Type: tea.KeyUp})
	rm = updated.(RoutesModel)
	assert.Equal(t, 0, rm.cursor)
}

func TestRoutesModel_EmptyList(t *testing.T) {
	m := NewRoutesModel(nil, NewStyles())
	view := m.View()
	assert.Contains(t, view, "No routes")
}

func TestRoutesModel_ViewContainsRouteInfo(t *testing.T) {
	routes := []RouteDisplay{
		{ID: "r1", Selector: "priority:critical", Targets: "ALL"},
	}
	m := NewRoutesModel(routes, NewStyles())
	view := m.View()

	assert.Contains(t, view, "priority:critical")
	assert.Contains(t, view, "ALL")
}

func TestRoutesModel_DeleteEmitsMessage(t *testing.T) {
	routes := []RouteDisplay{
		{ID: "r1", Selector: "a", Targets: "x"},
		{ID: "r2", Selector: "b", Targets: "y"},
	}
	m := NewRoutesModel(routes, NewStyles())

	// Move to second route
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	rm := updated.(RoutesModel)

	// Press 'd' to delete
	updated, cmd := rm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	_ = updated.(RoutesModel)

	// The cmd should produce a RouteDeleteMsg
	assert.NotNil(t, cmd)
	msg := cmd()
	deleteMsg, ok := msg.(RouteDeleteMsg)
	assert.True(t, ok)
	assert.Equal(t, "r2", deleteMsg.RouteID)
}

func TestRoutesModel_ToggleAddMode(t *testing.T) {
	m := NewRoutesModel(nil, NewStyles())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	rm := updated.(RoutesModel)
	assert.True(t, rm.adding)

	// Escape exits add mode
	updated, _ = rm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rm = updated.(RoutesModel)
	assert.False(t, rm.adding)
}

func TestRoutesModel_UpdateRoutes(t *testing.T) {
	m := NewRoutesModel(nil, NewStyles())

	newRoutes := []RouteDisplay{
		{ID: "r1", Selector: "priority:critical", Targets: "ALL"},
	}
	updated, _ := m.Update(RoutesUpdateMsg{Routes: newRoutes})
	rm := updated.(RoutesModel)
	assert.Equal(t, 1, len(rm.routes))
}

func TestRoutesModel_GoldenView(t *testing.T) {
	routes := []RouteDisplay{
		{ID: "r1", Selector: "project:api-server", Targets: "phone, laptop"},
		{ID: "r2", Selector: "session:refactor", Targets: "telegram"},
		{ID: "r3", Selector: "priority:critical", Targets: "ALL"},
		{ID: "r4", Selector: "source:Codex", Targets: "desktop-only"},
	}
	m := NewRoutesModel(routes, NewStyles())
	m.width = 80

	golden.RequireEqual(t, []byte(m.View()))
}
