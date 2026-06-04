package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// ActionsUpdateMsg is dispatched by the parent App when fresh pending-action
// data has been received from the daemon.
type ActionsUpdateMsg struct {
	Actions []protocols.ActionRequest `json:"actions"`
}

// ActionsModel renders the "actions" tab: a list of pending ActionRequests
// awaiting approval. Pressing y / n approves or denies the highlighted row.
type ActionsModel struct {
	styles  Styles
	width   int
	height  int
	cursor  int
	pending []protocols.ActionRequest
}

// NewActionsModel constructs an empty ActionsModel using the supplied styles.
func NewActionsModel(styles Styles) ActionsModel {
	panic("not implemented: tui.NewActionsModel")
}

// Init returns the initial command for the actions tab.
func (m ActionsModel) Init() tea.Cmd {
	panic("not implemented: tui.ActionsModel.Init")
}

// Update handles a tea.Msg and returns the next ActionsModel state plus an
// optional follow-up command.
func (m ActionsModel) Update(msg tea.Msg) (ActionsModel, tea.Cmd) {
	panic("not implemented: tui.ActionsModel.Update")
}

// View renders the actions tab as a string.
func (m ActionsModel) View() string {
	panic("not implemented: tui.ActionsModel.View")
}
