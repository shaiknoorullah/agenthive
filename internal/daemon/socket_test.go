package daemon

import (
	"encoding/json"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocketListener_AcceptsConnection(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "test.sock")

	var received []protocol.Message
	var mu sync.Mutex

	handler := func(msg protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go func() { _ = listener.Serve() }()
	defer func() { _ = listener.Close() }()

	// Wait for listener to be ready
	time.Sleep(50 * time.Millisecond)

	// Connect and send a message
	conn, err := net.Dial("unix", sockPath) //nolint:noctx // acceptable in tests
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck // test cleanup

	msg := protocol.Message{
		ID:       "test-1",
		Type:     protocol.MsgNotification,
		SourceID: "hook",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	_, err = conn.Write(append(data, '\n'))
	require.NoError(t, err)

	// Wait for message processing
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1)
	assert.Equal(t, "test-1", received[0].ID)
}

func TestSocketListener_MultipleMessages(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "test.sock")

	var received []protocol.Message
	var mu sync.Mutex

	handler := func(msg protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go func() { _ = listener.Serve() }()
	defer func() { _ = listener.Close() }()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath) //nolint:noctx // acceptable in tests
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck // test cleanup

	for i := 0; i < 3; i++ {
		msg := protocol.Message{
			ID:       protocol.NewMessageID(),
			Type:     protocol.MsgNotification,
			SourceID: "hook",
			Payload: protocol.NotificationPayload{
				Project: "api",
				Source:  "Claude",
				Message: "msg",
			},
		}
		data, _ := json.Marshal(msg)
		_, err = conn.Write(append(data, '\n'))
		require.NoError(t, err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 3)
}

func TestSocketListener_MultipleConnections(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "test.sock")

	var received []protocol.Message
	var mu sync.Mutex

	handler := func(msg protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go func() { _ = listener.Serve() }()
	defer func() { _ = listener.Close() }()

	time.Sleep(50 * time.Millisecond)

	// Two separate connections
	for i := 0; i < 2; i++ {
		conn, err := net.Dial("unix", sockPath) //nolint:noctx // acceptable in tests
		require.NoError(t, err)

		msg := protocol.Message{
			ID:       protocol.NewMessageID(),
			Type:     protocol.MsgNotification,
			SourceID: "hook",
			Payload: protocol.NotificationPayload{
				Project: "api",
				Source:  "Claude",
				Message: "msg",
			},
		}
		data, _ := json.Marshal(msg)
		_, err = conn.Write(append(data, '\n'))
		require.NoError(t, err)
		_ = conn.Close() // best-effort close in loop
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 2)
}

func TestSocketListener_Close_StopsAccepting(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "test.sock")

	handler := func(msg protocol.Message) {}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go func() { _ = listener.Serve() }()

	time.Sleep(50 * time.Millisecond)

	// Close the listener
	_ = listener.Close()

	time.Sleep(50 * time.Millisecond)

	// Connections should be refused
	_, err = net.Dial("unix", sockPath) //nolint:noctx // acceptable in tests
	assert.Error(t, err)
}

func TestSocketListener_InvalidJSON_DoesNotCrash(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "test.sock")

	var received []protocol.Message
	var mu sync.Mutex

	handler := func(msg protocol.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)

	go func() { _ = listener.Serve() }()
	defer func() { _ = listener.Close() }()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath) //nolint:noctx // acceptable in tests
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck // test cleanup

	// Send invalid JSON followed by valid message
	_, err = conn.Write([]byte("not valid json\n"))
	require.NoError(t, err)

	msg := protocol.Message{
		ID:       "valid-1",
		Type:     protocol.MsgNotification,
		SourceID: "hook",
		Payload: protocol.NotificationPayload{
			Project: "api",
			Source:  "Claude",
			Message: "Done",
		},
	}
	data, _ := json.Marshal(msg)
	_, err = conn.Write(append(data, '\n'))
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 1)
	assert.Equal(t, "valid-1", received[0].ID)
}

func TestSocketListener_SocketPath(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := filepath.Join(dir, "test.sock")

	handler := func(msg protocol.Message) {}

	listener, err := NewSocketListener(sockPath, handler)
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	assert.Equal(t, sockPath, listener.Path())
}

func TestSocketListener_InvalidPath(t *testing.T) {
	// Path in a non-existent directory should fail
	handler := func(msg protocol.Message) {}
	_, err := NewSocketListener("/no/such/dir/test.sock", handler)
	require.Error(t, err)
}
