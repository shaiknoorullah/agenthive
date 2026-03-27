package hooks

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNotificationInput_Valid(t *testing.T) {
	input := `{
		"session_id": "abc123",
		"cwd": "/home/user/my-app",
		"hook_event_name": "Notification",
		"message": "Claude is waiting for permission to run: npm test",
		"title": "Permission Required",
		"notification_type": "permission_prompt"
	}`

	notif, err := ParseNotificationInput([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "abc123", notif.SessionID)
	assert.Equal(t, "Permission Required", notif.Title)
	assert.Equal(t, "permission_prompt", notif.NotificationType)
	assert.Contains(t, notif.Message, "npm test")
}

func TestParseNotificationInput_InvalidJSON(t *testing.T) {
	_, err := ParseNotificationInput([]byte("{broken"))
	assert.Error(t, err)
}

func TestParseStopInput_Valid(t *testing.T) {
	input := `{
		"session_id": "abc123",
		"cwd": "/home/user/my-app",
		"hook_event_name": "Stop",
		"stop_hook_active": false,
		"last_assistant_message": "I've completed the refactoring. All tests pass."
	}`

	stop, err := ParseStopInput([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "abc123", stop.SessionID)
	assert.Contains(t, stop.LastAssistantMessage, "refactoring")
	assert.False(t, stop.StopHookActive)
}

func TestParseStopInput_InvalidJSON(t *testing.T) {
	_, err := ParseStopInput([]byte("nope"))
	assert.Error(t, err)
}

func TestHandleNotification_DispatchesToSocket(t *testing.T) {
	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "test.sock")

	// Start a mock daemon listening on the socket
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	received := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		received <- buf[:n]
	}()

	notif := &NotificationInput{
		SessionID:        "sess-1",
		CWD:              "/home/user/my-app",
		Message:          "Agent finished task",
		Title:            "Task Complete",
		NotificationType: "idle_prompt",
	}

	err = HandleNotification(notif, socketPath)
	require.NoError(t, err)

	data := <-received
	assert.Contains(t, string(data), `"type":"notification"`)
	assert.Contains(t, string(data), `"Agent finished task"`)
}

func TestHandleNotification_NoSocket_ReturnsNil(t *testing.T) {
	notif := &NotificationInput{
		SessionID: "sess-2",
		Message:   "test",
	}
	// Non-existent socket -- should not error (graceful degradation)
	err := HandleNotification(notif, "/tmp/nonexistent-agenthive-test.sock")
	assert.NoError(t, err)
}

func TestHandleStop_DispatchesToSocket(t *testing.T) {
	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	received := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		received <- buf[:n]
	}()

	stop := &StopInput{
		SessionID:            "sess-3",
		CWD:                  "/home/user/my-app",
		LastAssistantMessage: "All tests pass. Refactoring complete.",
	}

	err = HandleStop(stop, socketPath)
	require.NoError(t, err)

	data := <-received
	assert.Contains(t, string(data), `"type":"stop"`)
	assert.Contains(t, string(data), "All tests pass")
}

func TestHandleStop_WritesToTmuxOption(t *testing.T) {
	// This test verifies the notification message struct is built correctly,
	// not that tmux is actually called (tmux may not be available in CI).
	stop := &StopInput{
		SessionID:            "sess-4",
		CWD:                  "/home/user/projects/api-server",
		LastAssistantMessage: "Done.",
	}

	msg := BuildStopMessage(stop)
	assert.Equal(t, "stop", msg.Type)
	assert.Equal(t, "api-server", msg.Project)
	assert.Contains(t, msg.Message, "Done.")
}

func TestBuildNotificationMessage_ProjectFromCWD(t *testing.T) {
	notif := &NotificationInput{
		SessionID:        "sess-5",
		CWD:              "/home/user/projects/frontend-app",
		Message:          "Permission needed",
		NotificationType: "permission_prompt",
	}

	msg := BuildNotificationMessage(notif)
	assert.Equal(t, "notification", msg.Type)
	assert.Equal(t, "frontend-app", msg.Project)
	assert.Equal(t, "permission_prompt", msg.NotificationType)
}

func TestBuildNotificationMessage_EmptyCWD(t *testing.T) {
	notif := &NotificationInput{
		SessionID: "sess-6",
		CWD:       "",
		Message:   "test",
	}

	msg := BuildNotificationMessage(notif)
	assert.Equal(t, "unknown", msg.Project)
}

func TestHandleNotification_MessageIsNewlineDelimitedJSON(t *testing.T) {
	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	received := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		received <- buf[:n]
	}()

	notif := &NotificationInput{
		SessionID:        "sess-7",
		CWD:              "/tmp",
		Message:          "test",
		NotificationType: "idle_prompt",
	}

	err = HandleNotification(notif, socketPath)
	require.NoError(t, err)

	data := <-received
	// Must end with newline (NDJSON protocol)
	assert.True(t, len(data) > 0 && data[len(data)-1] == '\n',
		"message must be newline-delimited JSON")

	// Must be valid JSON (without the trailing newline)
	var parsed map[string]any
	err = json.Unmarshal(data[:len(data)-1], &parsed)
	assert.NoError(t, err, "message must be valid JSON")
}

// Suppress unused import warning
var _ = os.ReadFile
