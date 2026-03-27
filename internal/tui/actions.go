package tui

import (
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

func NewActionsModel(actions []ActionDisplay, styles *Styles) ActionsModel { return ActionsModel{} }
func (m ActionsModel) Init() tea.Cmd                                       { return nil }
func (m ActionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)            { return m, nil }
func (m ActionsModel) View() string                                        { return "" }
