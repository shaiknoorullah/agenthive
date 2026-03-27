package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueue_EnqueueAndDrain(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	msg := protocol.Message{
		ID:        "msg-1",
		Type:      protocol.MsgNotification,
		SourceID:  "peer-a",
		TargetID:  "peer-b",
		Timestamp: time.Now(),
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	err = q.Enqueue("peer-b", msg)
	require.NoError(t, err)

	msgs, err := q.Drain("peer-b")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "msg-1", msgs[0].ID)
}

func TestQueue_DrainEmpty(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	msgs, err := q.Drain("nonexistent")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestQueue_MultipleMessages(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		msg := protocol.Message{
			ID:        protocol.NewMessageID(),
			Type:      protocol.MsgNotification,
			SourceID:  "peer-a",
			TargetID:  "peer-b",
			Timestamp: time.Now(),
			Payload: protocol.NotificationPayload{
				Project: "api",
				Source:  "Claude",
				Message: "msg",
			},
		}
		err = q.Enqueue("peer-b", msg)
		require.NoError(t, err)
	}

	msgs, err := q.Drain("peer-b")
	require.NoError(t, err)
	assert.Len(t, msgs, 5)
}

func TestQueue_DrainClearsQueue(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	msg := protocol.Message{
		ID:        "msg-1",
		Type:      protocol.MsgNotification,
		SourceID:  "peer-a",
		TargetID:  "peer-b",
		Timestamp: time.Now(),
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	err = q.Enqueue("peer-b", msg)
	require.NoError(t, err)

	// First drain returns the message
	msgs, err := q.Drain("peer-b")
	require.NoError(t, err)
	assert.Len(t, msgs, 1)

	// Second drain returns empty
	msgs, err = q.Drain("peer-b")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestQueue_MultiplePeers(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	msgB := protocol.Message{
		ID:       "msg-b",
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		TargetID: "peer-b",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "for-b",
		},
	}
	msgC := protocol.Message{
		ID:       "msg-c",
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		TargetID: "peer-c",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "for-c",
		},
	}

	require.NoError(t, q.Enqueue("peer-b", msgB))
	require.NoError(t, q.Enqueue("peer-c", msgC))

	msgsB, err := q.Drain("peer-b")
	require.NoError(t, err)
	assert.Len(t, msgsB, 1)
	assert.Equal(t, "msg-b", msgsB[0].ID)

	msgsC, err := q.Drain("peer-c")
	require.NoError(t, err)
	assert.Len(t, msgsC, 1)
	assert.Equal(t, "msg-c", msgsC[0].ID)
}

func TestQueue_Depth(t *testing.T) {
	dir := t.TempDir()
	q, err := NewQueue(filepath.Join(dir, "queue"))
	require.NoError(t, err)

	assert.Equal(t, 0, q.Depth("peer-b"))

	for i := 0; i < 3; i++ {
		msg := protocol.Message{
			ID:       protocol.NewMessageID(),
			Type:     protocol.MsgNotification,
			SourceID: "peer-a",
			Payload: protocol.NotificationPayload{
				Project: "api",
				Source:  "Claude",
				Message: "msg",
			},
		}
		require.NoError(t, q.Enqueue("peer-b", msg))
	}

	assert.Equal(t, 3, q.Depth("peer-b"))
}

func TestQueue_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	queueDir := filepath.Join(dir, "queue")

	// Enqueue with first instance
	q1, err := NewQueue(queueDir)
	require.NoError(t, err)

	msg := protocol.Message{
		ID:       "persist-1",
		Type:     protocol.MsgNotification,
		SourceID: "peer-a",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "persistent",
		},
	}
	require.NoError(t, q1.Enqueue("peer-b", msg))

	// Create second instance (simulates daemon restart)
	q2, err := NewQueue(queueDir)
	require.NoError(t, err)

	msgs, err := q2.Drain("peer-b")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "persist-1", msgs[0].ID)
}
