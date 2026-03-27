package hooks

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
type DaemonMessage struct {
	Type             string `json:"type"`
	SessionID        string `json:"session_id"`
	Project          string `json:"project"`
	Message          string `json:"message"`
	NotificationType string `json:"notification_type,omitempty"`
}

func ParseNotificationInput(data []byte) (*NotificationInput, error)  { return nil, nil }
func ParseStopInput(data []byte) (*StopInput, error)                  { return nil, nil }
func HandleNotification(notif *NotificationInput, socketPath string) error { return nil }
func HandleStop(stop *StopInput, socketPath string) error                  { return nil }
func BuildNotificationMessage(notif *NotificationInput) DaemonMessage     { return DaemonMessage{} }
func BuildStopMessage(stop *StopInput) DaemonMessage                      { return DaemonMessage{} }
