package tui

import (
	"fmt"
	"strings"
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

// NewLogsModel creates a new Logs tab model.
func NewLogsModel(logs []LogEntry, styles *Styles) LogsModel {
	if logs == nil {
		logs = []LogEntry{}
	}
	return LogsModel{
		logs:   logs,
		styles: styles,
	}
}

func (m LogsModel) Init() tea.Cmd { return nil }

func (m LogsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "down", "j":
			filtered := m.filteredLogs()
			visibleLines := m.visibleLines()
			if m.offset < len(filtered)-visibleLines {
				m.offset++
			}
		case "up", "k":
			if m.offset > 0 {
				m.offset--
			}
		case "1":
			if m.filterLevel == "critical" {
				m.filterLevel = ""
			} else {
				m.filterLevel = "critical"
			}
			m.offset = 0
		case "2":
			if m.filterLevel == "warning" {
				m.filterLevel = ""
			} else {
				m.filterLevel = "warning"
			}
			m.offset = 0
		case "3":
			if m.filterLevel == "info" {
				m.filterLevel = ""
			} else {
				m.filterLevel = "info"
			}
			m.offset = 0
		}
	case LogsUpdateMsg:
		m.logs = msg.Logs
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m LogsModel) visibleLines() int {
	if m.height <= 6 {
		return 10 // default
	}
	return m.height - 6 // header + stats + help
}

func (m LogsModel) filteredLogs() []LogEntry {
	if m.filterLevel == "" {
		return m.logs
	}
	var result []LogEntry
	for _, l := range m.logs {
		if l.Level == m.filterLevel {
			result = append(result, l)
		}
	}
	return result
}

func (m LogsModel) stats() LogStats {
	s := LogStats{Total: len(m.logs)}
	for _, l := range m.logs {
		switch l.Level {
		case "critical":
			s.Critical++
		case "warning":
			s.Warning++
		case "info":
			s.Info++
		}
	}
	return s
}

func (m LogsModel) View() string {
	if len(m.logs) == 0 {
		return "  No log entries.\n\n  Events will appear here as they occur.\n"
	}

	var b strings.Builder

	// Summary stats
	s := m.stats()
	fmt.Fprintf(&b, "  Total: %d  |  ", s.Total)
	b.WriteString(m.styles.PriorityCritical.Render(fmt.Sprintf("Critical: %d", s.Critical)))
	b.WriteString("  ")
	b.WriteString(m.styles.PriorityWarning.Render(fmt.Sprintf("Warning: %d", s.Warning)))
	b.WriteString("  ")
	b.WriteString(m.styles.PriorityInfo.Render(fmt.Sprintf("Info: %d", s.Info)))
	b.WriteString("\n")

	// Filter indicator
	if m.filterLevel != "" {
		fmt.Fprintf(&b, "  Filter: %s (press same key to clear)\n", m.filterLevel)
	}
	b.WriteString("\n")

	// Log entries
	filtered := m.filteredLogs()
	visible := m.visibleLines()
	end := m.offset + visible
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := m.offset; i < end; i++ {
		l := filtered[i]
		ts := l.Timestamp.Format("15:04")
		line := fmt.Sprintf("  [%s] %s/%s: %s", ts, l.Agent, l.Project, l.Message)

		switch l.Level {
		case "critical":
			line = m.styles.PriorityCritical.Render(line)
		case "warning":
			line = m.styles.PriorityWarning.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	// Help
	b.WriteString("\n")
	b.WriteString(m.styles.Help.Render("  Filter: [1]critical [2]warning [3]info  |  Scroll: up/down"))
	b.WriteString("\n")

	return b.String()
}
