package tui

import tea "github.com/charmbracelet/bubbletea"

// Tab constants.
const (
	TabPeers   = 0
	TabRoutes  = 1
	TabActions = 2
	TabLogs    = 3
)

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

func NewAppModel() AppModel                                 { return AppModel{} }
func (m AppModel) Init() tea.Cmd                            { return nil }
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m AppModel) View() string                             { return "" }
