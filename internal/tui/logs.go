package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
// numeric keys 1 (info), 2 (warn), 3 (crit) and clear the filter with 0.
// Cursor movement is j (down) and k (up); arrow keys are accepted too so
// the tab is usable for terminals without modal navigation muscle memory.
type LogsModel struct {
	styles  Styles
	width   int
	height  int
	cursor  int
	entries []LogEntry
	// filter is empty (show all) or one of "info" / "warn" / "crit".
	filter string
}

// NewLogsModel constructs an empty LogsModel using the supplied styles.
func NewLogsModel(styles Styles) LogsModel {
	return LogsModel{
		styles:  styles,
		width:   80,
		height:  24,
		cursor:  0,
		entries: nil,
		filter:  "",
	}
}

// Init returns the initial command for the logs tab. The tab has no
// background work of its own — the parent App polls the daemon and feeds
// LogsUpdateMsg in — so Init is a no-op.
func (m LogsModel) Init() tea.Cmd {
	return nil
}

// Update handles a tea.Msg and returns the next LogsModel state plus an
// optional follow-up command.
func (m LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	switch v := msg.(type) {
	case LogsUpdateMsg:
		m.entries = append([]LogEntry(nil), v.Entries...)
		m.clampCursor()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(v), nil
	}
	return m, nil
}

// handleKey routes a single keypress to the appropriate state mutation.
func (m LogsModel) handleKey(k tea.KeyMsg) LogsModel {
	switch k.Type {
	case tea.KeyUp:
		return m.moveCursor(-1)
	case tea.KeyDown:
		return m.moveCursor(1)
	case tea.KeyRunes:
		if len(k.Runes) == 0 {
			return m
		}
		switch k.Runes[0] {
		case 'j':
			return m.moveCursor(1)
		case 'k':
			return m.moveCursor(-1)
		case '1':
			m.filter = "info"
			m.clampCursor()
			return m
		case '2':
			m.filter = "warn"
			m.clampCursor()
			return m
		case '3':
			m.filter = "crit"
			m.clampCursor()
			return m
		case '0':
			m.filter = ""
			m.clampCursor()
			return m
		}
	}
	return m
}

// moveCursor shifts the cursor by delta, clamped to the bounds of the
// currently visible (post-filter) slice.
func (m LogsModel) moveCursor(delta int) LogsModel {
	n := len(m.visible())
	if n == 0 {
		m.cursor = 0
		return m
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	return m
}

// clampCursor adjusts the cursor when the visible-row count shrinks (for
// example because the user just applied a tighter filter or a smaller
// LogsUpdateMsg arrived).
func (m *LogsModel) clampCursor() {
	n := len(m.visible())
	if n == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// visible returns the slice of entries that match the active filter.
// An empty filter returns every entry.
func (m LogsModel) visible() []LogEntry {
	if m.filter == "" {
		return m.entries
	}
	out := make([]LogEntry, 0, len(m.entries))
	for _, e := range m.entries {
		if strings.EqualFold(e.Level, m.filter) {
			out = append(out, e)
		}
	}
	return out
}

// View renders the logs tab as a string.
func (m LogsModel) View() string {
	var b strings.Builder

	// Title row: tab name plus the current filter, if any.
	title := "logs"
	if m.filter != "" {
		title = fmt.Sprintf("logs [%s]", m.filter)
	}
	b.WriteString(m.styles.Title.Render(title))
	b.WriteString("\n")

	// Column header row.
	header := fmt.Sprintf("%-19s  %-5s  %-12s  %s", "time", "level", "source", "message")
	b.WriteString(m.styles.Header.Render(header))
	b.WriteString("\n")

	rows := m.visible()
	if len(rows) == 0 {
		b.WriteString(m.styles.Subtle.Render("(no log entries)"))
		b.WriteString("\n")
		b.WriteString(m.footer())
		return b.String()
	}

	// Reserve 3 lines of chrome (title, header, footer) so the body
	// fits the reported height. If the height is tiny, just emit one
	// row so the tab still renders something useful.
	maxRows := m.height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	if maxRows > len(rows) {
		maxRows = len(rows)
	}

	// Centre the visible window on the cursor.
	start := m.cursor - maxRows/2
	if start < 0 {
		start = 0
	}
	end := start + maxRows
	if end > len(rows) {
		end = len(rows)
		start = end - maxRows
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		row := m.renderRow(rows[i])
		if i == m.cursor {
			row = m.styles.Cursor.Render("> " + row)
		} else {
			row = "  " + row
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	b.WriteString(m.footer())
	return b.String()
}

// renderRow formats a single LogEntry as a fixed-column row, colouring
// the level token by severity.
func (m LogsModel) renderRow(e LogEntry) string {
	level := e.Level
	switch strings.ToLower(level) {
	case "info":
		level = m.styles.Good.Render(padRight(level, 5))
	case "warn":
		level = m.styles.Warn.Render(padRight(level, 5))
	case "crit":
		level = m.styles.Crit.Render(padRight(level, 5))
	default:
		level = padRight(level, 5)
	}
	source := padRight(e.Source, 12)
	msg := e.Message
	// Trim the message so a single row never overflows the terminal
	// width. We allow a generous max so small terminals do not wrap
	// the row across two visual lines (which would break golden
	// snapshots).
	maxMsg := m.width - 19 - 5 - 12 - 6 // padding between cols
	if maxMsg > 0 && lipgloss.Width(msg) > maxMsg {
		msg = msg[:maxMsg]
	}
	stamp := e.Timestamp.UTC().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("%s  %s  %s  %s", stamp, level, source, msg)
}

// footer renders the keybinding hint bar.
func (m LogsModel) footer() string {
	hints := "1 info  2 warn  3 crit  0 all  j/k scroll"
	return m.styles.Footer.Render(hints)
}

// padRight pads s with spaces so its visual width equals n; if s is
// already wider, it is truncated.
func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		if len(s) > n {
			return s[:n]
		}
		return s
	}
	return s + strings.Repeat(" ", n-w)
}
