package transport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkManager_AddAndRemoveLink(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA, _ := NewPipeLinkPair("peer-local", "peer-a")
	defer linkA.Close()

	lm.AddLink(linkA)
	assert.Equal(t, 1, lm.LinkCount())

	peers := lm.ConnectedPeers()
	assert.Contains(t, peers, "peer-a")

	lm.RemoveLink("peer-a")
	assert.Equal(t, 0, lm.LinkCount())
}

func TestLinkManager_Broadcast(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	// Create two peer links
	linkA, remoteA := NewPipeLinkPair("peer-local", "peer-a")
	linkB, remoteB := NewPipeLinkPair("peer-local", "peer-b")
	defer linkA.Close()
	defer linkB.Close()
	defer remoteA.Close()
	defer remoteB.Close()

	lm.AddLink(linkA)
	lm.AddLink(linkB)

	env := Envelope{
		Type: MsgNotification, ID: "broadcast-001", From: "peer-local",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{"msg":"hello all"}`),
	}

	err := lm.Broadcast(env)
	require.NoError(t, err)

	// Both remote ends should receive the message
	select {
	case msg := <-remoteA.Receive():
		assert.Equal(t, "broadcast-001", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: remote A did not receive broadcast")
	}

	select {
	case msg := <-remoteB.Receive():
		assert.Equal(t, "broadcast-001", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: remote B did not receive broadcast")
	}
}

func TestLinkManager_SendTo(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA, remoteA := NewPipeLinkPair("peer-local", "peer-a")
	linkB, remoteB := NewPipeLinkPair("peer-local", "peer-b")
	defer linkA.Close()
	defer linkB.Close()
	defer remoteA.Close()
	defer remoteB.Close()

	lm.AddLink(linkA)
	lm.AddLink(linkB)

	env := Envelope{
		Type: MsgActionResponse, ID: "targeted-001", From: "peer-local",
		To: "peer-a", Timestamp: time.Now().UTC(),
		Payload: json.RawMessage(`{"action":"allow"}`),
	}

	err := lm.SendTo("peer-a", env)
	require.NoError(t, err)

	// Only remote A should receive it
	select {
	case msg := <-remoteA.Receive():
		assert.Equal(t, "targeted-001", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: remote A did not receive targeted message")
	}

	// Remote B should NOT receive it
	select {
	case <-remoteB.Receive():
		t.Fatal("remote B should not have received the targeted message")
	case <-time.After(100 * time.Millisecond):
		// Good -- nothing received
	}
}

func TestLinkManager_SendTo_UnknownPeer(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	err := lm.SendTo("nonexistent", Envelope{
		Type: MsgHeartbeat, ID: "lost", From: "peer-local",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	assert.Error(t, err)
}

func TestLinkManager_Inbound(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA, remoteA := NewPipeLinkPair("peer-local", "peer-a")
	linkB, remoteB := NewPipeLinkPair("peer-local", "peer-b")
	defer linkA.Close()
	defer linkB.Close()
	defer remoteA.Close()
	defer remoteB.Close()

	lm.AddLink(linkA)
	lm.AddLink(linkB)

	// Send from remote A
	remoteA.Send(Envelope{
		Type: MsgNotification, ID: "from-a", From: "peer-a",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})

	// Send from remote B
	remoteB.Send(Envelope{
		Type: MsgNotification, ID: "from-b", From: "peer-b",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})

	// Both should arrive on the aggregated inbound channel
	received := make(map[string]bool)
	inbound := lm.Inbound()
	for i := 0; i < 2; i++ {
		select {
		case msg := <-inbound:
			received[msg.ID] = true
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for inbound message %d", i)
		}
	}
	assert.True(t, received["from-a"])
	assert.True(t, received["from-b"])
}

func TestLinkManager_GetLinkStatus(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA, _ := NewPipeLinkPair("peer-local", "peer-a")
	defer linkA.Close()

	lm.AddLink(linkA)

	status, ok := lm.GetLinkStatus("peer-a")
	assert.True(t, ok)
	assert.Equal(t, StatusConnected, status)

	_, ok = lm.GetLinkStatus("nonexistent")
	assert.False(t, ok)
}

func TestLinkManager_Close_ClosesAllLinks(t *testing.T) {
	lm := NewLinkManager("peer-local")

	linkA, _ := NewPipeLinkPair("peer-local", "peer-a")
	linkB, _ := NewPipeLinkPair("peer-local", "peer-b")

	lm.AddLink(linkA)
	lm.AddLink(linkB)

	err := lm.Close()
	assert.NoError(t, err)

	assert.Equal(t, StatusDisconnected, linkA.Status())
	assert.Equal(t, StatusDisconnected, linkB.Status())
}

func TestLinkManager_AddLink_DuplicatePeerReplacesOld(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	linkA1, _ := NewPipeLinkPair("peer-local", "peer-a")
	linkA2, _ := NewPipeLinkPair("peer-local", "peer-a")

	lm.AddLink(linkA1)
	lm.AddLink(linkA2) // should replace linkA1

	assert.Equal(t, 1, lm.LinkCount())
	// Old link should be closed
	assert.Equal(t, StatusDisconnected, linkA1.Status())
}

func TestLinkManager_BroadcastToNone(t *testing.T) {
	lm := NewLinkManager("peer-local")
	defer lm.Close()

	// Broadcasting with no links should not error
	err := lm.Broadcast(Envelope{
		Type: MsgHeartbeat, ID: "lonely", From: "peer-local",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	assert.NoError(t, err)
}
