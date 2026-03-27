package transport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipeLink_SendAndReceive(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkA.Close()
	defer linkB.Close()

	env := Envelope{
		Type:      MsgNotification,
		ID:        "msg-001",
		From:      "peer-a",
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{"message":"hello"}`),
	}

	err := linkA.Send(env)
	require.NoError(t, err)

	select {
	case received := <-linkB.Receive():
		assert.Equal(t, "msg-001", received.ID)
		assert.Equal(t, MsgNotification, received.Type)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestPipeLink_Bidirectional(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkA.Close()
	defer linkB.Close()

	// A -> B
	err := linkA.Send(Envelope{
		Type: MsgHeartbeat, ID: "a-to-b", From: "peer-a",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// B -> A
	err = linkB.Send(Envelope{
		Type: MsgHeartbeat, ID: "b-to-a", From: "peer-b",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	select {
	case msg := <-linkB.Receive():
		assert.Equal(t, "a-to-b", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: B did not receive from A")
	}

	select {
	case msg := <-linkA.Receive():
		assert.Equal(t, "b-to-a", msg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: A did not receive from B")
	}
}

func TestPipeLink_Status(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")

	assert.Equal(t, StatusConnected, linkA.Status())
	assert.Equal(t, StatusConnected, linkB.Status())

	linkA.Close()
	assert.Equal(t, StatusDisconnected, linkA.Status())
}

func TestPipeLink_PeerID(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkA.Close()
	defer linkB.Close()

	// linkA's peer is peer-b (the remote end)
	assert.Equal(t, "peer-b", linkA.PeerID())
	assert.Equal(t, "peer-a", linkB.PeerID())
}

func TestPipeLink_SendAfterClose_ReturnsError(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkB.Close()

	linkA.Close()

	err := linkA.Send(Envelope{
		Type: MsgHeartbeat, ID: "late", From: "peer-a",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})
	assert.Error(t, err)
}

func TestPipeLink_CloseIsIdempotent(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkB.Close()

	err := linkA.Close()
	assert.NoError(t, err)
	err = linkA.Close()
	assert.NoError(t, err)
}

func TestPipeLink_MultipleMessages(t *testing.T) {
	linkA, linkB := NewPipeLinkPair("peer-a", "peer-b")
	defer linkA.Close()
	defer linkB.Close()

	for i := 0; i < 10; i++ {
		err := linkA.Send(Envelope{
			Type: MsgNotification, ID: "msg", From: "peer-a",
			Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}

	received := 0
	for received < 10 {
		select {
		case <-linkB.Receive():
			received++
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout after receiving %d messages", received)
		}
	}
	assert.Equal(t, 10, received)
}
