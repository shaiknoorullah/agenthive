package tui

import tea "github.com/charmbracelet/bubbletea"

// PeerDisplay holds peer data for rendering.
type PeerDisplay struct {
	ID       string
	Name     string
	Status   string
	Latency  string
	Agents   int
	Messages int
	LastSeen string
}

// PeersUpdateMsg is sent when the peer list changes.
type PeersUpdateMsg struct {
	Peers []PeerDisplay
}

// PeersModel is the bubbletea model for the Peers tab.
type PeersModel struct {
	peers  []PeerDisplay
	cursor int
	width  int
	height int
	styles *Styles
}

func NewPeersModel(peers []PeerDisplay, styles *Styles) PeersModel { return PeersModel{} }
func (m PeersModel) Init() tea.Cmd                                 { return nil }
func (m PeersModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)       { return m, nil }
func (m PeersModel) View() string                                  { return "" }
