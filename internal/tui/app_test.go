package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func init() {
	lipgloss.DefaultRenderer().SetColorProfile(termenv.Ascii)
}

func TestAppModel_InitialTab(t *testing.T) {
	m := NewAppModel()
	assert.Equal(t, TabPeers, m.activeTab)
}

func TestAppModel_SwitchToPeers(t *testing.T) {
	m := NewAppModel()
	m.activeTab = TabRoutes

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	am := updated.(AppModel)
	assert.Equal(t, TabPeers, am.activeTab)
}

func TestAppModel_SwitchToRoutes(t *testing.T) {
	m := NewAppModel()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	am := updated.(AppModel)
	assert.Equal(t, TabRoutes, am.activeTab)
}

func TestAppModel_SwitchToActions(t *testing.T) {
	m := NewAppModel()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	am := updated.(AppModel)
	assert.Equal(t, TabActions, am.activeTab)
}

func TestAppModel_SwitchToLogs(t *testing.T) {
	m := NewAppModel()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	am := updated.(AppModel)
	assert.Equal(t, TabLogs, am.activeTab)
}

func TestAppModel_QuitOnQ(t *testing.T) {
	m := NewAppModel()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.NotNil(t, cmd)

	// The cmd should be tea.Quit
	msg := cmd()
	assert.IsType(t, tea.QuitMsg{}, msg)
}

func TestAppModel_QuitOnCtrlC(t *testing.T) {
	m := NewAppModel()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
}

func TestAppModel_WindowSizeMsg(t *testing.T) {
	m := NewAppModel()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	am := updated.(AppModel)
	assert.Equal(t, 120, am.width)
	assert.Equal(t, 40, am.height)
}

func TestAppModel_ViewContainsTabBar(t *testing.T) {
	m := NewAppModel()
	view := m.View()

	assert.Contains(t, view, "[p]eers")
	assert.Contains(t, view, "[r]outes")
	assert.Contains(t, view, "[a]ctions")
	assert.Contains(t, view, "[l]ogs")
}

func TestAppModel_ViewContainsActiveTabContent(t *testing.T) {
	m := NewAppModel()
	m.peers = NewPeersModel([]PeerDisplay{
		{ID: "test-server", Name: "test-server", Status: "online", Latency: "5ms"},
	}, m.styles)
	m.activeTab = TabPeers

	view := m.View()
	assert.Contains(t, view, "test-server")
}

func TestAppModel_TabSwitchPreservesState(t *testing.T) {
	m := NewAppModel()

	// Set up some route data
	m.routes = NewRoutesModel([]RouteDisplay{
		{ID: "r1", Selector: "priority:critical", Targets: "ALL"},
	}, m.styles)

	// Switch to routes
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	am := updated.(AppModel)

	// Switch to peers and back
	updated, _ = am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	am = updated.(AppModel)
	updated, _ = am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	am = updated.(AppModel)

	// Route data should be preserved
	assert.Equal(t, 1, len(am.routes.routes))
}

func TestAppModel_DispatchesToActiveTab(t *testing.T) {
	m := NewAppModel()
	m.peers = NewPeersModel([]PeerDisplay{
		{ID: "a", Name: "a", Status: "online"},
		{ID: "b", Name: "b", Status: "online"},
	}, m.styles)
	m.activeTab = TabPeers

	// Down arrow should go to the peers model
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	am := updated.(AppModel)
	assert.Equal(t, 1, am.peers.cursor)
}

func TestAppModel_DoesNotDispatchTabKeysToChildren(t *testing.T) {
	m := NewAppModel()
	m.activeTab = TabActions

	// 'a' is a tab-switch key, should NOT be dispatched to actions tab
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	am := updated.(AppModel)

	// Should switch to Actions tab (already there), NOT trigger add mode
	assert.Equal(t, TabActions, am.activeTab)
}

func TestAppModel_GoldenView_PeersTab(t *testing.T) {
	m := NewAppModel()
	m.width = 80
	m.height = 24
	m.peers = NewPeersModel([]PeerDisplay{
		{ID: "dev-server", Name: "dev-server", Status: "online", Latency: "12ms", Agents: 5, Messages: 43},
		{ID: "macbook-pro", Name: "macbook-pro", Status: "online", Latency: "3ms", Agents: 2, Messages: 18},
		{ID: "pixel-phone", Name: "pixel-phone", Status: "online", Latency: "45ms", Agents: 0, Messages: 7},
	}, m.styles)
	m.activeTab = TabPeers

	golden.RequireEqual(t, []byte(m.View()))
}

func TestAppModel_GoldenView_ActionsTab(t *testing.T) {
	m := NewAppModel()
	m.width = 80
	m.height = 24
	m.actions = NewActionsModel([]ActionDisplay{
		{
			RequestID: "req-42",
			Agent:     "Claude",
			Project:   "api-server",
			Command:   "rm -rf /tmp/build",
			Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC),
		},
	}, m.styles)
	m.activeTab = TabActions

	golden.RequireEqual(t, []byte(m.View()))
}
