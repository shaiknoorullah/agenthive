package hooks

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"time"
)

// NotificationInput represents the JSON data for a Notification hook event.
type NotificationInput struct {
	SessionID        string `json:"session_id"`
	CWD              string `json:"cwd"`
	HookEventName    string `json:"hook_event_name"`
	Message          string `json:"message"`
	Title            string `json:"title,omitempty"`
	NotificationType string `json:"notification_type"`
}

// StopInput represents the JSON data for a Stop hook event.
type StopInput struct {
	SessionID            string `json:"session_id"`
	CWD                  string `json:"cwd"`
	HookEventName        string `json:"hook_event_name"`
	StopHookActive       bool   `json:"stop_hook_active"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

// DaemonMessage is a message dispatched to the daemon via Unix socket.
// Follows the newline-delimited JSON protocol from the architecture spec.
type DaemonMessage struct {
	Type             string `json:"type"`
	SessionID        string `json:"session_id"`
	Project          string `json:"project"`
	Message          string `json:"message"`
	NotificationType string `json:"notification_type,omitempty"`
}

// ParseNotificationInput parses the JSON data for a Notification hook event.
func ParseNotificationInput(data []byte) (*NotificationInput, error) {
	var input NotificationInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("parse notification input: %w", err)
	}
	return &input, nil
}

// ParseStopInput parses the JSON data for a Stop hook event.
func ParseStopInput(data []byte) (*StopInput, error) {
	var input StopInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("parse stop input: %w", err)
	}
	return &input, nil
}

// projectFromCWD extracts the project name from the working directory.
// Returns "unknown" if CWD is empty.
func projectFromCWD(cwd string) string {
	if cwd == "" {
		return "unknown"
	}
	return filepath.Base(cwd)
}

// BuildNotificationMessage builds a DaemonMessage from a Notification hook input.
func BuildNotificationMessage(notif *NotificationInput) DaemonMessage {
	return DaemonMessage{
		Type:             "notification",
		SessionID:        notif.SessionID,
		Project:          projectFromCWD(notif.CWD),
		Message:          notif.Message,
		NotificationType: notif.NotificationType,
	}
}

// BuildStopMessage builds a DaemonMessage from a Stop hook input.
func BuildStopMessage(stop *StopInput) DaemonMessage {
	return DaemonMessage{
		Type:      "stop",
		SessionID: stop.SessionID,
		Project:   projectFromCWD(stop.CWD),
		Message:   stop.LastAssistantMessage,
	}
}

// dispatchMessage sends a DaemonMessage to the daemon via Unix socket.
// Returns nil if the socket is unavailable (graceful degradation).
func dispatchMessage(msg DaemonMessage, socketPath string) error {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second) //nolint:noctx // best-effort fire-and-forget
	if err != nil {
		// Daemon not available -- degrade gracefully
		return nil
	}
	defer conn.Close() //nolint:errcheck // best-effort cleanup

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal daemon message: %w", err)
	}

	// Newline-delimited JSON protocol
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write to daemon socket: %w", err)
	}
	return nil
}

// HandleNotification processes a Notification hook event.
// It dispatches the notification to the daemon via Unix socket.
// Non-blocking: Notification hooks cannot block Claude Code.
func HandleNotification(notif *NotificationInput, socketPath string) error {
	msg := BuildNotificationMessage(notif)
	return dispatchMessage(msg, socketPath)
}

// HandleStop processes a Stop hook event.
// It dispatches the stop notification to the daemon via Unix socket.
func HandleStop(stop *StopInput, socketPath string) error {
	msg := BuildStopMessage(stop)
	return dispatchMessage(msg, socketPath)
}
