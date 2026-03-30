package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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

// NewPeersModel creates a new Peers tab model.
func NewPeersModel(peers []PeerDisplay, styles *Styles) PeersModel {
	if peers == nil {
		peers = []PeerDisplay{}
	}
	return PeersModel{
		peers:  peers,
		cursor: 0,
		styles: styles,
	}
}

func (m PeersModel) Init() tea.Cmd { return nil }

func (m PeersModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "down", "j":
			if m.cursor < len(m.peers)-1 {
				m.cursor++
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		}
	case PeersUpdateMsg:
		m.peers = msg.Peers
		if m.cursor >= len(m.peers) {
			m.cursor = max(0, len(m.peers)-1)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m PeersModel) View() string {
	if len(m.peers) == 0 {
		return "  No peers connected.\n\n  Use 'agenthive pair --remote user@host' to add a peer.\n"
	}

	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "  %-18s %-10s %-8s %-10s %s\n",
		"PEER", "STATUS", "LATENCY", "AGENTS", "MESSAGES")
	fmt.Fprintf(&b, "  %s\n", strings.Repeat("-", 60))

	for i, p := range m.peers {
		statusIcon := "*"
		statusText := p.Status
		if p.Status == "offline" {
			statusIcon = "o"
		}

		latency := p.Latency
		extra := fmt.Sprintf("%d msgs today", p.Messages)
		if p.Status == "offline" && p.LastSeen != "" {
			extra = fmt.Sprintf("last seen %s", p.LastSeen)
		}

		line := fmt.Sprintf("  %s %-16s %-10s %-8s %-10s %s",
			statusIcon, p.Name, statusText, latency,
			fmt.Sprintf("%d agents", p.Agents), extra)

		if i == m.cursor {
			line = m.styles.SelectedRow.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}
