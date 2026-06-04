package tui

import (
	"fmt"
	"sort"
	"strings"

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
// The model holds no peers, has a zero cursor, and zero width/height until
// the first WindowSizeMsg arrives. Peer data flows in via PeersUpdateMsg.
func NewPeersModel(styles Styles) PeersModel {
	return PeersModel{styles: styles}
}

// Init returns the initial command for the peers tab. The peers tab has no
// background work of its own — it is driven exclusively by inbound
// PeersUpdateMsg values dispatched from the parent App.
func (m PeersModel) Init() tea.Cmd { return nil }

// Update handles a tea.Msg and returns the next PeersModel state plus an
// optional follow-up command.
//
// Recognised messages:
//   - PeersUpdateMsg      replaces the rendered peer list (sorted by ID)
//   - tea.WindowSizeMsg   records the viewport dimensions
//   - tea.KeyMsg          j / down  : cursor down
//                         k / up    : cursor up
//
// Cursor moves are clamped to [0, len(peers)-1]. The cursor is also clamped
// after a PeersUpdateMsg so a shrunken peer set never leaves the cursor past
// the new last row.
func (m PeersModel) Update(msg tea.Msg) (PeersModel, tea.Cmd) {
	switch msg := msg.(type) {
	case PeersUpdateMsg:
		m.peers = peerRowsFromMap(msg.Peers)
		m.cursor = peersClampCursor(m.cursor, len(m.peers))
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.peers)-1 {
				m.cursor++
			}
			return m, nil
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		}
		return m, nil
	}
	return m, nil
}

// View renders the peers tab as a string. The output has a single header
// row and one row per peer, where the cursor row is prefixed with ">". When
// the peer set is empty the View shows a deliberate empty-state hint.
func (m PeersModel) View() string {
	var b strings.Builder

	b.WriteString(m.styles.Title.Render("Peers"))
	b.WriteString("\n")

	header := fmt.Sprintf("  %-20s  %-16s  %-8s  %-12s  %s",
		"ID", "NAME", "STATUS", "LINK", "ADDR")
	b.WriteString(m.styles.Header.Render(header))
	b.WriteString("\n")

	if len(m.peers) == 0 {
		b.WriteString(m.styles.Subtle.Render("  No peers connected."))
		b.WriteString("\n")
		return b.String()
	}

	for i, p := range m.peers {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		row := fmt.Sprintf("%s %-20s  %-16s  %-8s  %-12s  %s",
			marker,
			peersTruncate(p.ID, 20),
			peersTruncate(p.Name, 16),
			peerStatusIcon(p.Status, m.styles),
			peersTruncate(p.LinkType, 12),
			peersTruncate(p.Addr, 32),
		)
		if i == m.cursor {
			row = m.styles.Cursor.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

// Cursor reports the index of the currently highlighted row.
func (m PeersModel) Cursor() int { return m.cursor }

// Width reports the viewport width last reported via tea.WindowSizeMsg.
func (m PeersModel) Width() int { return m.width }

// Height reports the viewport height last reported via tea.WindowSizeMsg.
func (m PeersModel) Height() int { return m.height }

// Peers returns the current peer snapshot (sorted by ID) as a fresh slice so
// callers cannot mutate the model.
func (m PeersModel) Peers() []crdt.PeerInfo {
	out := make([]crdt.PeerInfo, 0, len(m.peers))
	for _, p := range m.peers {
		out = append(out, crdt.PeerInfo{
			Name:     p.Name,
			Status:   p.Status,
			Addr:     p.Addr,
			LinkType: p.LinkType,
			LastSeen: p.LastSeen,
		})
	}
	return out
}

// peerRowsFromMap converts the CRDT peer map into a stable, ID-sorted slice
// suitable for rendering. The peer IDs alone determine sort order so two
// TUIs viewing the same CRDT see byte-identical output.
func peerRowsFromMap(in map[string]crdt.PeerInfo) []peerRow {
	rows := make([]peerRow, 0, len(in))
	for id, info := range in {
		rows = append(rows, peerRow{
			ID:       id,
			Name:     info.Name,
			Status:   info.Status,
			Addr:     info.Addr,
			LinkType: info.LinkType,
			LastSeen: info.LastSeen,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

// peerStatusIcon maps a status string onto a short, styled label. Unknown
// statuses render as a literal dash so the column never collapses.
func peerStatusIcon(status string, styles Styles) string {
	label := status
	if label == "" {
		label = "-"
	}
	switch strings.ToLower(status) {
	case "online", "up", "ok":
		return styles.Good.Render(padCell(label, 8))
	case "warn", "degraded":
		return styles.Warn.Render(padCell(label, 8))
	case "down", "offline", "crit":
		return styles.Crit.Render(padCell(label, 8))
	}
	return padCell(label, 8)
}

// peersClampCursor keeps a cursor inside [0, n-1]. Returns 0 if n is zero.
func peersClampCursor(cursor, n int) int {
	if n <= 0 {
		return 0
	}
	if cursor < 0 {
		return 0
	}
	if cursor >= n {
		return n - 1
	}
	return cursor
}

// peersTruncate returns s shortened to at most n runes, with an ellipsis
// when truncation actually happens.
func peersTruncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// padCell pads s with spaces so its visual width equals n; if s is already
// wider, it is truncated.
func padCell(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		if len(r) > n {
			return string(r[:n])
		}
		return s
	}
	return s + strings.Repeat(" ", n-len(r))
}
