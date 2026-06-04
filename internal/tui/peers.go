package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
)

// PeersUpdateMsg is dispatched by the parent App when fresh peer data has
// been received from the daemon. The PeersModel ingests it on Update.
type PeersUpdateMsg struct {
	Peers map[string]crdt.PeerInfo `json:"peers"`
}

// PeersModel renders the "peers" tab: a sortable list of peers with status
// icons. The cursor field tracks the currently highlighted row.
type PeersModel struct {
	styles Styles
	width  int
	height int
	cursor int
	peers  []peerRow
}

// peerRow is the rendered representation of a single peer. It is sorted by
// PeerID so output is stable across runs.
type peerRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Addr     string `json:"addr"`
	LinkType string `json:"link_type"`
	LastSeen string `json:"last_seen"`
}

// NewPeersModel constructs an empty PeersModel using the supplied styles.
func NewPeersModel(styles Styles) PeersModel {
	panic("not implemented: tui.NewPeersModel")
}

// Init returns the initial command for the peers tab.
func (m PeersModel) Init() tea.Cmd {
	panic("not implemented: tui.PeersModel.Init")
}

// Update handles a tea.Msg and returns the next PeersModel state plus an
// optional follow-up command.
func (m PeersModel) Update(msg tea.Msg) (PeersModel, tea.Cmd) {
	panic("not implemented: tui.PeersModel.Update")
}

// View renders the peers tab as a string.
func (m PeersModel) View() string {
	panic("not implemented: tui.PeersModel.View")
}
