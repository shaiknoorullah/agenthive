package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/assert"
)

func TestPeersModel_InitialState(t *testing.T) {
	peers := []PeerDisplay{
		{ID: "dev-server", Name: "dev-server", Status: "online", Latency: "12ms", Agents: 5, Messages: 43},
		{ID: "macbook-pro", Name: "macbook-pro", Status: "online", Latency: "3ms", Agents: 2, Messages: 18},
		{ID: "pixel-phone", Name: "pixel-phone", Status: "online", Latency: "45ms", Agents: 0, Messages: 7},
	}
	m := NewPeersModel(peers, NewStyles())

	assert.Equal(t, 0, m.cursor)
	assert.Equal(t, 3, len(m.peers))
}

func TestPeersModel_CursorDown(t *testing.T) {
	peers := []PeerDisplay{
		{ID: "a", Name: "a", Status: "online"},
		{ID: "b", Name: "b", Status: "online"},
		{ID: "c", Name: "c", Status: "offline"},
	}
	m := NewPeersModel(peers, NewStyles())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	pm := updated.(PeersModel)
	assert.Equal(t, 1, pm.cursor)

	updated, _ = pm.Update(tea.KeyMsg{Type: tea.KeyDown})
	pm = updated.(PeersModel)
	assert.Equal(t, 2, pm.cursor)

	// Should not go past the end
	updated, _ = pm.Update(tea.KeyMsg{Type: tea.KeyDown})
	pm = updated.(PeersModel)
	assert.Equal(t, 2, pm.cursor)
}

func TestPeersModel_CursorUp(t *testing.T) {
	peers := []PeerDisplay{
		{ID: "a", Name: "a", Status: "online"},
		{ID: "b", Name: "b", Status: "online"},
	}
	m := NewPeersModel(peers, NewStyles())

	// Move down first
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	pm := updated.(PeersModel)
	assert.Equal(t, 1, pm.cursor)

	// Move back up
	updated, _ = pm.Update(tea.KeyMsg{Type: tea.KeyUp})
	pm = updated.(PeersModel)
	assert.Equal(t, 0, pm.cursor)

	// Should not go past the beginning
	updated, _ = pm.Update(tea.KeyMsg{Type: tea.KeyUp})
	pm = updated.(PeersModel)
	assert.Equal(t, 0, pm.cursor)
}

func TestPeersModel_CursorWithJK(t *testing.T) {
	peers := []PeerDisplay{
		{ID: "a", Name: "a", Status: "online"},
		{ID: "b", Name: "b", Status: "online"},
	}
	m := NewPeersModel(peers, NewStyles())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	pm := updated.(PeersModel)
	assert.Equal(t, 1, pm.cursor)

	updated, _ = pm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	pm = updated.(PeersModel)
	assert.Equal(t, 0, pm.cursor)
}

func TestPeersModel_EmptyList(t *testing.T) {
	m := NewPeersModel(nil, NewStyles())
	view := m.View()
	assert.Contains(t, view, "No peers")
}

func TestPeersModel_ViewContainsPeerInfo(t *testing.T) {
	peers := []PeerDisplay{
		{ID: "dev-server", Name: "dev-server", Status: "online", Latency: "12ms", Agents: 5, Messages: 43},
	}
	m := NewPeersModel(peers, NewStyles())
	view := m.View()

	assert.Contains(t, view, "dev-server")
	assert.Contains(t, view, "online")
	assert.Contains(t, view, "12ms")
}

func TestPeersModel_UpdatePeers(t *testing.T) {
	m := NewPeersModel(nil, NewStyles())
	assert.Equal(t, 0, len(m.peers))

	newPeers := []PeerDisplay{
		{ID: "server", Name: "server", Status: "online"},
	}
	updated, _ := m.Update(PeersUpdateMsg{Peers: newPeers})
	pm := updated.(PeersModel)
	assert.Equal(t, 1, len(pm.peers))
}

func TestPeersModel_GoldenView(t *testing.T) {
	peers := []PeerDisplay{
		{ID: "dev-server", Name: "dev-server", Status: "online", Latency: "12ms", Agents: 5, Messages: 43},
		{ID: "macbook-pro", Name: "macbook-pro", Status: "online", Latency: "3ms", Agents: 2, Messages: 18},
		{ID: "pixel-phone", Name: "pixel-phone", Status: "online", Latency: "45ms", Agents: 0, Messages: 7},
		{ID: "work-desktop", Name: "work-desktop", Status: "offline", Latency: "--", Agents: 0, Messages: 0, LastSeen: "2h ago"},
	}
	m := NewPeersModel(peers, NewStyles())
	m.width = 80

	golden.RequireEqual(t, []byte(m.View()))
}
