package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Tab constants.
const (
	TabPeers   = 0
	TabRoutes  = 1
	TabActions = 2
	TabLogs    = 3
)

var tabNames = []string{"[p]eers", "[r]outes", "[a]ctions", "[l]ogs"}

// AppModel is the root bubbletea model.
type AppModel struct {
	activeTab int
	peers     PeersModel
	routes    RoutesModel
	actions   ActionsModel
	logs      LogsModel
	styles    *Styles
	width     int
	height    int
}

// NewAppModel creates a new root App model with empty tab models.
func NewAppModel() AppModel {
	styles := NewStyles()
	return AppModel{
		activeTab: TabPeers,
		peers:     NewPeersModel(nil, styles),
		routes:    NewRoutesModel(nil, styles),
		actions:   NewActionsModel(nil, styles),
		logs:      NewLogsModel(nil, styles),
		styles:    styles,
	}
}

func (m AppModel) Init() tea.Cmd { return nil }

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global keys: tab switching and quit
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "ctrl+c":
			return m, tea.Quit
		case "p":
			m.activeTab = TabPeers
			return m, nil
		case "r":
			m.activeTab = TabRoutes
			return m, nil
		case "a":
			m.activeTab = TabActions
			return m, nil
		case "l":
			m.activeTab = TabLogs
			return m, nil
		}

		// Dispatch non-tab keys to active tab
		return m.updateActiveTab(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Forward to all tabs
		m.peers.width = msg.Width
		m.peers.height = msg.Height - 4
		m.routes.width = msg.Width
		m.routes.height = msg.Height - 4
		m.actions.width = msg.Width
		m.actions.height = msg.Height - 4
		m.logs.width = msg.Width
		m.logs.height = msg.Height - 4

		return m, nil

	default:
		// Forward other messages to active tab
		return m.updateActiveTab(msg)
	}
}

func (m AppModel) updateActiveTab(msg tea.Msg) (AppModel, tea.Cmd) {
	var cmd tea.Cmd
	switch m.activeTab {
	case TabPeers:
		var model tea.Model
		model, cmd = m.peers.Update(msg)
		m.peers = model.(PeersModel)
	case TabRoutes:
		var model tea.Model
		model, cmd = m.routes.Update(msg)
		m.routes = model.(RoutesModel)
	case TabActions:
		var model tea.Model
		model, cmd = m.actions.Update(msg)
		m.actions = model.(ActionsModel)
	case TabLogs:
		var model tea.Model
		model, cmd = m.logs.Update(msg)
		m.logs = model.(LogsModel)
	}
	return m, cmd
}

func (m AppModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(m.styles.Title.Render("agenthive"))
	b.WriteString("\n\n")

	// Tab bar
	b.WriteString("  ")
	for i, name := range tabNames {
		if i == m.activeTab {
			b.WriteString(m.styles.TabActive.Render(name))
		} else {
			b.WriteString(m.styles.TabInactive.Render(name))
		}
		b.WriteString("  ")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s\n", strings.Repeat("─", 60))

	// Active tab content
	switch m.activeTab {
	case TabPeers:
		b.WriteString(m.peers.View())
	case TabRoutes:
		b.WriteString(m.routes.View())
	case TabActions:
		b.WriteString(m.actions.View())
	case TabLogs:
		b.WriteString(m.logs.View())
	}

	// Bottom help
	b.WriteString("\n")
	b.WriteString(m.styles.Help.Render("  [p]eers [r]outes [a]ctions [l]ogs  |  [q]uit"))
	b.WriteString("\n")

	return b.String()
}
