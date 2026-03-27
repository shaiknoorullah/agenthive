package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ActionDisplay holds action request data for rendering.
type ActionDisplay struct {
	RequestID string
	Agent     string
	Project   string
	Command   string
	Timestamp time.Time
}

// ActionsUpdateMsg is sent when the action list changes.
type ActionsUpdateMsg struct {
	Actions []ActionDisplay
}

// ActionResponseMsg is the user's decision on an action.
type ActionResponseMsg struct {
	RequestID string
	Decision  string // "allow" or "deny"
}

// ActionsModel is the bubbletea model for the Actions tab.
type ActionsModel struct {
	actions []ActionDisplay
	cursor  int
	width   int
	height  int
	styles  *Styles
}

// NewActionsModel creates a new Actions tab model.
func NewActionsModel(actions []ActionDisplay, styles *Styles) ActionsModel {
	if actions == nil {
		actions = []ActionDisplay{}
	}
	return ActionsModel{
		actions: actions,
		cursor:  0,
		styles:  styles,
	}
}

func (m ActionsModel) Init() tea.Cmd { return nil }

func (m ActionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "down", "j":
			if m.cursor < len(m.actions)-1 {
				m.cursor++
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "y":
			if len(m.actions) > 0 && m.cursor < len(m.actions) {
				reqID := m.actions[m.cursor].RequestID
				return m, func() tea.Msg {
					return ActionResponseMsg{RequestID: reqID, Decision: "allow"}
				}
			}
		case "n":
			if len(m.actions) > 0 && m.cursor < len(m.actions) {
				reqID := m.actions[m.cursor].RequestID
				return m, func() tea.Msg {
					return ActionResponseMsg{RequestID: reqID, Decision: "deny"}
				}
			}
		}
	case ActionsUpdateMsg:
		m.actions = msg.Actions
		if m.cursor >= len(m.actions) {
			m.cursor = max(0, len(m.actions)-1)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m ActionsModel) View() string {
	if len(m.actions) == 0 {
		return "  No pending actions.\n\n  Action requests from agents will appear here.\n"
	}

	var b strings.Builder

	b.WriteString(fmt.Sprintf("  %d pending action(s)\n\n", len(m.actions)))

	for i, a := range m.actions {
		ts := a.Timestamp.Format("15:04")
		header := fmt.Sprintf("  [%s] %s/%s wants to run:", ts, a.Agent, a.Project)
		command := fmt.Sprintf("    $ %s", a.Command)

		if i == m.cursor {
			header = m.styles.SelectedRow.Render(header)
			command = m.styles.SelectedRow.Render(command)
		}

		b.WriteString(header)
		b.WriteString("\n")
		b.WriteString(command)
		b.WriteString("\n")

		if i == m.cursor {
			approve := m.styles.ActionApprove.Render("[y] Allow")
			deny := m.styles.ActionDeny.Render("[n] Deny")
			b.WriteString(fmt.Sprintf("    %s  %s\n", approve, deny))
		}

		b.WriteString("\n")
	}

	return b.String()
}
