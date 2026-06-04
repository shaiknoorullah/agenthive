// Package tui — App is the root bubbletea Model that owns the four tab
// Models (peers, routes, actions, logs) and dispatches Update messages
// based on the active tab. Tab switching is keyboard-driven (p / r / a / l)
// and quitting is q / ctrl+c.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Tab identifies one of the four top-level tabs.
type Tab int

// Tab values. Order is the rendering order in the tab bar.
const (
	TabPeers Tab = iota
	TabRoutes
	TabActions
	TabLogs
)

// App is the root bubbletea Model. It owns each tab Model and routes
// Update messages to the active one. The App also handles global key
// bindings (tab switching, quit).
type App struct {
	styles  Styles
	active  Tab
	width   int
	height  int
	peers   PeersModel
	routes  RoutesModel
	actions ActionsModel
	logs    LogsModel
}

// NewApp constructs the root App with all four tab Models initialised to
// the supplied styles.
func NewApp(styles Styles) App {
	panic("not implemented: tui.NewApp")
}

// Init returns the initial tea.Cmd for the App. It batches the Init
// commands from every child Model so each tab can start polling its own
// state.
func (a App) Init() tea.Cmd {
	panic("not implemented: tui.App.Init")
}

// Update routes the message to the active child Model and handles global
// keys (p/r/a/l to switch tabs, q/ctrl+c to quit).
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	panic("not implemented: tui.App.Update")
}

// View renders the tab bar, the active tab's view, and the footer hint bar.
func (a App) View() string {
	panic("not implemented: tui.App.View")
}
