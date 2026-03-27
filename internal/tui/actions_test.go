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

func TestActionsModel_InitialState(t *testing.T) {
	actions := []ActionDisplay{
		{
			RequestID: "req-42",
			Agent:     "Claude",
			Project:   "api-server",
			Command:   "rm -rf /tmp/build",
			Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC),
		},
	}
	m := NewActionsModel(actions, NewStyles())

	assert.Equal(t, 0, m.cursor)
	assert.Equal(t, 1, len(m.actions))
}

func TestActionsModel_CursorNavigation(t *testing.T) {
	actions := []ActionDisplay{
		{RequestID: "req-1", Agent: "Claude", Command: "cmd1"},
		{RequestID: "req-2", Agent: "Codex", Command: "cmd2"},
		{RequestID: "req-3", Agent: "Claude", Command: "cmd3"},
	}
	m := NewActionsModel(actions, NewStyles())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	am := updated.(ActionsModel)
	assert.Equal(t, 1, am.cursor)

	updated, _ = am.Update(tea.KeyMsg{Type: tea.KeyUp})
	am = updated.(ActionsModel)
	assert.Equal(t, 0, am.cursor)
}

func TestActionsModel_Approve(t *testing.T) {
	actions := []ActionDisplay{
		{RequestID: "req-42", Agent: "Claude", Command: "rm -rf /tmp/build"},
	}
	m := NewActionsModel(actions, NewStyles())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	assert.NotNil(t, cmd)
	msg := cmd()
	response, ok := msg.(ActionResponseMsg)
	assert.True(t, ok)
	assert.Equal(t, "req-42", response.RequestID)
	assert.Equal(t, "allow", response.Decision)
}

func TestActionsModel_Deny(t *testing.T) {
	actions := []ActionDisplay{
		{RequestID: "req-42", Agent: "Claude", Command: "rm -rf /tmp/build"},
	}
	m := NewActionsModel(actions, NewStyles())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	assert.NotNil(t, cmd)
	msg := cmd()
	response, ok := msg.(ActionResponseMsg)
	assert.True(t, ok)
	assert.Equal(t, "req-42", response.RequestID)
	assert.Equal(t, "deny", response.Decision)
}

func TestActionsModel_EmptyList(t *testing.T) {
	m := NewActionsModel(nil, NewStyles())
	view := m.View()
	assert.Contains(t, view, "No pending actions")
}

func TestActionsModel_ViewContainsActionInfo(t *testing.T) {
	actions := []ActionDisplay{
		{
			RequestID: "req-42",
			Agent:     "Claude",
			Project:   "api-server",
			Command:   "rm -rf /tmp/build",
			Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC),
		},
	}
	m := NewActionsModel(actions, NewStyles())
	view := m.View()

	assert.Contains(t, view, "Claude")
	assert.Contains(t, view, "api-server")
	assert.Contains(t, view, "rm -rf /tmp/build")
}

func TestActionsModel_UpdateActions(t *testing.T) {
	m := NewActionsModel(nil, NewStyles())

	newActions := []ActionDisplay{
		{RequestID: "req-99", Agent: "Codex", Command: "npm install"},
	}
	updated, _ := m.Update(ActionsUpdateMsg{Actions: newActions})
	am := updated.(ActionsModel)
	assert.Equal(t, 1, len(am.actions))
}

func TestActionsModel_GoldenView(t *testing.T) {
	actions := []ActionDisplay{
		{
			RequestID: "req-42",
			Agent:     "Claude",
			Project:   "api-server",
			Command:   "rm -rf /tmp/build",
			Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC),
		},
		{
			RequestID: "req-43",
			Agent:     "Codex",
			Project:   "docs-gen",
			Command:   "npm run build",
			Timestamp: time.Date(2026, 3, 26, 14, 32, 0, 0, time.UTC),
		},
	}
	m := NewActionsModel(actions, NewStyles())
	m.width = 80

	golden.RequireEqual(t, []byte(m.View()))
}
