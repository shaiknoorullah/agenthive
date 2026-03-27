package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func init() {
	lipgloss.DefaultRenderer().SetColorProfile(termenv.Ascii)
}

func TestIntegration_TabSwitching(t *testing.T) {
	m := NewAppModel()
	m.peers = NewPeersModel([]PeerDisplay{
		{ID: "server", Name: "server", Status: "online", Latency: "5ms", Agents: 1, Messages: 10},
	}, m.styles)
	m.routes = NewRoutesModel([]RouteDisplay{
		{ID: "r1", Selector: "priority:critical", Targets: "ALL"},
	}, m.styles)

	tm := teatest.NewTestModel(
		t,
		m,
		teatest.WithInitialTermSize(80, 24),
	)

	// Verify initial state shows peers tab
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "server")
		},
		teatest.WithDuration(2*time.Second),
		teatest.WithCheckInterval(100*time.Millisecond),
	)

	// Switch to routes tab
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "priority:critical")
		},
		teatest.WithDuration(2*time.Second),
		teatest.WithCheckInterval(100*time.Millisecond),
	)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestIntegration_ActionsApproval(t *testing.T) {
	m := NewAppModel()
	m.actions = NewActionsModel([]ActionDisplay{
		{
			RequestID: "req-42",
			Agent:     "Claude",
			Project:   "api-server",
			Command:   "rm -rf /tmp/build",
			Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC),
		},
	}, m.styles)

	tm := teatest.NewTestModel(
		t,
		m,
		teatest.WithInitialTermSize(80, 24),
	)

	// Switch to actions tab
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	// Wait for action to appear
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "rm -rf /tmp/build")
		},
		teatest.WithDuration(2*time.Second),
		teatest.WithCheckInterval(100*time.Millisecond),
	)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestIntegration_LogsFiltering(t *testing.T) {
	m := NewAppModel()
	m.logs = NewLogsModel([]LogEntry{
		{Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC), Level: "critical", Agent: "Claude", Project: "api", Message: "Task failed"},
		{Timestamp: time.Date(2026, 3, 26, 14, 31, 0, 0, time.UTC), Level: "info", Agent: "Claude", Project: "web", Message: "Done"},
	}, m.styles)

	tm := teatest.NewTestModel(
		t,
		m,
		teatest.WithInitialTermSize(80, 24),
	)

	// Switch to logs tab
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	// Wait for logs to appear
	teatest.WaitFor(
		t,
		tm.Output(),
		func(bts []byte) bool {
			return strings.Contains(string(bts), "Task failed")
		},
		teatest.WithDuration(2*time.Second),
		teatest.WithCheckInterval(100*time.Millisecond),
	)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestIntegration_FinalModel(t *testing.T) {
	m := NewAppModel()
	m.peers = NewPeersModel([]PeerDisplay{
		{ID: "a", Name: "a", Status: "online"},
		{ID: "b", Name: "b", Status: "online"},
	}, m.styles)

	tm := teatest.NewTestModel(
		t,
		m,
		teatest.WithInitialTermSize(80, 24),
	)

	// Navigate down in peers
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})

	time.Sleep(100 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	am, ok := finalModel.(AppModel)
	assert.True(t, ok)
	assert.Equal(t, TabPeers, am.activeTab)
	assert.Equal(t, 1, am.peers.cursor)
}
