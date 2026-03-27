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

func TestLogsModel_InitialState(t *testing.T) {
	logs := []LogEntry{
		{Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC), Level: "critical", Agent: "Claude", Project: "api-server", Message: "Task failed"},
		{Timestamp: time.Date(2026, 3, 26, 14, 31, 0, 0, time.UTC), Level: "info", Agent: "Claude", Project: "frontend", Message: "Agent has finished"},
	}
	m := NewLogsModel(logs, NewStyles())

	assert.Equal(t, 2, len(m.logs))
	assert.Equal(t, "", m.filterLevel)
}

func TestLogsModel_ScrollDown(t *testing.T) {
	logs := make([]LogEntry, 30)
	for i := range logs {
		logs[i] = LogEntry{Level: "info", Message: "msg"}
	}
	m := NewLogsModel(logs, NewStyles())
	m.height = 10

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	lm := updated.(LogsModel)
	assert.Equal(t, 1, lm.offset)
}

func TestLogsModel_ScrollUp(t *testing.T) {
	logs := make([]LogEntry, 30)
	for i := range logs {
		logs[i] = LogEntry{Level: "info", Message: "msg"}
	}
	m := NewLogsModel(logs, NewStyles())
	m.height = 10
	m.offset = 5

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	lm := updated.(LogsModel)
	assert.Equal(t, 4, lm.offset)
}

func TestLogsModel_ScrollUpClamps(t *testing.T) {
	logs := []LogEntry{
		{Level: "info", Message: "msg"},
	}
	m := NewLogsModel(logs, NewStyles())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	lm := updated.(LogsModel)
	assert.Equal(t, 0, lm.offset)
}

func TestLogsModel_FilterByLevel(t *testing.T) {
	logs := []LogEntry{
		{Level: "info", Message: "info-msg"},
		{Level: "critical", Message: "crit-msg"},
		{Level: "warning", Message: "warn-msg"},
	}
	m := NewLogsModel(logs, NewStyles())

	// Press '1' for critical
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	lm := updated.(LogsModel)
	assert.Equal(t, "critical", lm.filterLevel)

	filtered := lm.filteredLogs()
	assert.Equal(t, 1, len(filtered))
	assert.Equal(t, "crit-msg", filtered[0].Message)
}

func TestLogsModel_FilterToggle(t *testing.T) {
	logs := []LogEntry{
		{Level: "info", Message: "msg"},
	}
	m := NewLogsModel(logs, NewStyles())

	// Press '1' for critical filter
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	lm := updated.(LogsModel)
	assert.Equal(t, "critical", lm.filterLevel)

	// Press '1' again to clear filter
	updated, _ = lm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	lm = updated.(LogsModel)
	assert.Equal(t, "", lm.filterLevel)
}

func TestLogsModel_FilterWarning(t *testing.T) {
	logs := []LogEntry{
		{Level: "info", Message: "info-msg"},
		{Level: "warning", Message: "warn-msg"},
	}
	m := NewLogsModel(logs, NewStyles())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	lm := updated.(LogsModel)
	assert.Equal(t, "warning", lm.filterLevel)

	filtered := lm.filteredLogs()
	assert.Equal(t, 1, len(filtered))
}

func TestLogsModel_FilterInfo(t *testing.T) {
	logs := []LogEntry{
		{Level: "info", Message: "info-msg"},
		{Level: "critical", Message: "crit-msg"},
	}
	m := NewLogsModel(logs, NewStyles())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	lm := updated.(LogsModel)
	assert.Equal(t, "info", lm.filterLevel)
}

func TestLogsModel_SummaryStats(t *testing.T) {
	logs := []LogEntry{
		{Level: "critical", Message: "a"},
		{Level: "critical", Message: "b"},
		{Level: "warning", Message: "c"},
		{Level: "info", Message: "d"},
		{Level: "info", Message: "e"},
		{Level: "info", Message: "f"},
	}
	m := NewLogsModel(logs, NewStyles())
	stats := m.stats()

	assert.Equal(t, 2, stats.Critical)
	assert.Equal(t, 1, stats.Warning)
	assert.Equal(t, 3, stats.Info)
	assert.Equal(t, 6, stats.Total)
}

func TestLogsModel_EmptyList(t *testing.T) {
	m := NewLogsModel(nil, NewStyles())
	view := m.View()
	assert.Contains(t, view, "No log entries")
}

func TestLogsModel_UpdateLogs(t *testing.T) {
	m := NewLogsModel(nil, NewStyles())

	newLogs := []LogEntry{
		{Level: "info", Message: "new entry"},
	}
	updated, _ := m.Update(LogsUpdateMsg{Logs: newLogs})
	lm := updated.(LogsModel)
	assert.Equal(t, 1, len(lm.logs))
}

func TestLogsModel_GoldenView(t *testing.T) {
	logs := []LogEntry{
		{Timestamp: time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC), Level: "critical", Agent: "Claude", Project: "api-server", Message: "Task failed"},
		{Timestamp: time.Date(2026, 3, 26, 14, 31, 0, 0, time.UTC), Level: "info", Agent: "Claude", Project: "frontend", Message: "Agent has finished"},
		{Timestamp: time.Date(2026, 3, 26, 14, 32, 0, 0, time.UTC), Level: "warning", Agent: "Codex", Project: "docs", Message: "Needs approval"},
	}
	m := NewLogsModel(logs, NewStyles())
	m.width = 80
	m.height = 24

	golden.RequireEqual(t, []byte(m.View()))
}
