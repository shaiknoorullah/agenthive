// Package tui — App is the root bubbletea Model that owns the four tab
// Models (peers, routes, actions, logs) and dispatches Update messages
// based on the active tab. Tab switching is keyboard-driven (p / r / a / l)
// and quitting is q / ctrl+c.
package tui

import (
	"fmt"
	"strings"

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

// tabLabel renders a Tab as the lowercase label used in the tab bar and
// the integration-test assertions.
func (t Tab) String() string {
	switch t {
	case TabPeers:
		return "peers"
	case TabRoutes:
		return "routes"
	case TabActions:
		return "actions"
	case TabLogs:
		return "logs"
	}
	return ""
}

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
// the supplied styles. The initial active tab is TabPeers so the operator
// opens onto the peer mesh overview.
func NewApp(styles Styles) App {
	return App{
		styles:  styles,
		active:  TabPeers,
		peers:   NewPeersModel(styles),
		routes:  NewRoutesModel(styles),
		actions: NewActionsModel(styles),
		logs:    NewLogsModel(styles),
	}
}

// Active reports the currently focused tab.
func (a App) Active() Tab { return a.active }

// Width reports the most recently received viewport width.
func (a App) Width() int { return a.width }

// Height reports the most recently received viewport height.
func (a App) Height() int { return a.height }

// Init returns the initial tea.Cmd for the App. It batches the Init
// commands from every child Model so each tab can start polling its own
// state. All current child Inits return nil so the batch is also nil-safe.
func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.peers.Init(),
		a.routes.Init(),
		a.actions.Init(),
		a.logs.Init(),
	)
}

// Update routes the message to the appropriate child Model(s) and handles
// global keys (p / r / a / l to switch tabs, q / ctrl+c to quit).
//
// Semantics:
//   - tea.WindowSizeMsg is broadcast to every child Model so their View
//     calculations stay consistent across the tab bar and footer.
//   - Tab-typed Update messages (PeersUpdateMsg, RoutesUpdateMsg,
//     ActionsUpdateMsg, LogsUpdateMsg) are routed to every child Model.
//     Each child's Update is a no-op for messages it does not recognise,
//     so the broadcast is cheap and lets the daemon push data to a tab
//     even when the operator is currently viewing another tab.
//   - tea.KeyMsg is intercepted for the global key bindings; otherwise
//     forwarded only to the currently active child Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.peers, _ = a.peers.Update(m)
		a.routes, _ = a.routes.Update(m)
		a.actions, _ = a.actions.Update(m)
		a.logs, _ = a.logs.Update(m)
		return a, nil
	case tea.KeyMsg:
		switch m.String() {
		case "q":
			return a, tea.Quit
		case "ctrl+c":
			return a, tea.Quit
		case "p":
			a.active = TabPeers
			return a, nil
		case "r":
			a.active = TabRoutes
			return a, nil
		case "a":
			a.active = TabActions
			return a, nil
		case "l":
			a.active = TabLogs
			return a, nil
		}
		return a.delegateKey(m)
	case PeersUpdateMsg:
		a.peers, _ = a.peers.Update(m)
		return a, nil
	case RoutesUpdateMsg:
		a.routes, _ = a.routes.Update(m)
		return a, nil
	case ActionsUpdateMsg:
		a.actions, _ = a.actions.Update(m)
		return a, nil
	case LogsUpdateMsg:
		a.logs, _ = a.logs.Update(m)
		return a, nil
	}
	return a, nil
}

// delegateKey forwards a key message to the currently active child Model
// and returns the resulting tea.Cmd.
func (a App) delegateKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch a.active {
	case TabPeers:
		a.peers, cmd = a.peers.Update(k)
	case TabRoutes:
		a.routes, cmd = a.routes.Update(k)
	case TabActions:
		a.actions, cmd = a.actions.Update(k)
	case TabLogs:
		a.logs, cmd = a.logs.Update(k)
	}
	return a, cmd
}

// View renders the tab bar, the active tab's view, and the footer hint bar.
// The active tab is wrapped in square brackets ("[peers]" vs " routes ") so
// even a colour-stripped render still communicates focus.
func (a App) View() string {
	var b strings.Builder

	// Tab bar.
	b.WriteString(a.renderTabBar())
	b.WriteString("\n")

	// Active tab body.
	switch a.active {
	case TabPeers:
		b.WriteString(a.peers.View())
	case TabRoutes:
		b.WriteString(a.routes.View())
	case TabActions:
		b.WriteString(a.actions.View())
	case TabLogs:
		b.WriteString(a.logs.View())
	}

	// Footer hint bar.
	b.WriteString("\n")
	b.WriteString(a.renderFooter())

	return b.String()
}

// renderTabBar renders the four tab labels with the active one bracketed
// and styled with TabActive. Order matches the Tab const block.
func (a App) renderTabBar() string {
	tabs := []Tab{TabPeers, TabRoutes, TabActions, TabLogs}
	parts := make([]string, 0, len(tabs))
	for _, t := range tabs {
		label := t.String()
		if t == a.active {
			parts = append(parts, a.styles.TabActive.Render("["+label+"]"))
		} else {
			parts = append(parts, a.styles.TabInactive.Render(" "+label+" "))
		}
	}
	return strings.Join(parts, " ")
}

// renderFooter renders the global key-binding hint bar.
func (a App) renderFooter() string {
	hint := fmt.Sprintf("%s switch: p peers / r routes / a actions / l logs   q quit",
		"agenthive")
	return a.styles.Footer.Render(hint)
}
