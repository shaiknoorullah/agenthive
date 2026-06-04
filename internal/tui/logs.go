package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LogEntry is one rendered log line in the logs tab.
type LogEntry struct {
	Timestamp time.Time `json:"ts"`
	Level     string    `json:"level"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
}

// LogsUpdateMsg is dispatched by the parent App when fresh log data has been
// received from the daemon.
type LogsUpdateMsg struct {
	Entries []LogEntry `json:"entries"`
}

// LogsModel renders the "logs" tab. The user can filter by level via the
// numeric keys 1 (info), 2 (warn), 3 (crit).
type LogsModel struct {
	styles  Styles
	width   int
	height  int
	cursor  int
	entries []LogEntry
	filter  string
}

// NewLogsModel constructs an empty LogsModel using the supplied styles.
func NewLogsModel(styles Styles) LogsModel {
	panic("not implemented: tui.NewLogsModel")
}

// Init returns the initial command for the logs tab.
func (m LogsModel) Init() tea.Cmd {
	panic("not implemented: tui.LogsModel.Init")
}

// Update handles a tea.Msg and returns the next LogsModel state plus an
// optional follow-up command.
func (m LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	panic("not implemented: tui.LogsModel.Update")
}

// View renders the logs tab as a string.
func (m LogsModel) View() string {
	panic("not implemented: tui.LogsModel.View")
}
