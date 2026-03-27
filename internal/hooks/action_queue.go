package hooks

import "time"

// PendingAction represents a tool execution waiting for user approval.
type PendingAction struct {
	ActionID  string         `json:"action_id"`
	SessionID string         `json:"session_id"`
	ToolUseID string         `json:"tool_use_id"`
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	Project   string         `json:"project"`
	PaneID    string         `json:"pane_id"`
	CWD       string         `json:"cwd"`
	Timestamp time.Time      `json:"timestamp"`
	ExpiresAt time.Time      `json:"expires_at"`
}

// ActionResponse represents a user's decision on a pending action.
type ActionResponse struct {
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason,omitempty"`
	Surface   string    `json:"surface,omitempty"`
	User      string    `json:"user,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Always    bool      `json:"always,omitempty"`
}

// ActionQueue manages pending action files on disk.
type ActionQueue struct{}

func NewActionQueue(dir string) *ActionQueue                              { return nil }
func (q *ActionQueue) WritePending(action PendingAction) error            { return nil }
func (q *ActionQueue) ReadPending(actionID string) (PendingAction, error) { return PendingAction{}, nil }
func (q *ActionQueue) WriteResponse(actionID string, resp ActionResponse) error { return nil }
func (q *ActionQueue) ReadResponse(actionID string) (ActionResponse, error) { return ActionResponse{}, nil }
func (q *ActionQueue) HasResponse(actionID string) bool                   { return false }
func (q *ActionQueue) Cleanup(actionID string)                            {}
func (q *ActionQueue) CleanupExpired() int                                { return 0 }
func (q *ActionQueue) ListPending() ([]PendingAction, error)              { return nil, nil }
