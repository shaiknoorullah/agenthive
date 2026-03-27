package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LogEntry holds a single log event for rendering.
type LogEntry struct {
	Timestamp time.Time
	Level     string // "info", "warning", "critical"
	Agent     string
	Project   string
	Message   string
}

// LogsUpdateMsg is sent when the log entries change.
type LogsUpdateMsg struct {
	Logs []LogEntry
}

// LogStats holds summary statistics for log entries.
type LogStats struct {
	Total    int
	Critical int
	Warning  int
	Info     int
}

// LogsModel is the bubbletea model for the Logs tab.
type LogsModel struct {
	logs        []LogEntry
	filterLevel string
	offset      int
	width       int
	height      int
	styles      *Styles
}

func NewLogsModel(logs []LogEntry, styles *Styles) LogsModel  { return LogsModel{} }
func (m LogsModel) Init() tea.Cmd                              { return nil }
func (m LogsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)   { return m, nil }
func (m LogsModel) View() string                               { return "" }
func (m LogsModel) filteredLogs() []LogEntry                   { return nil }
func (m LogsModel) stats() LogStats                            { return LogStats{} }
