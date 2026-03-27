package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
// Files are stored in a single directory with the naming convention:
//   {action_id}.pending  -- pending action request
//   {action_id}.response -- user's decision
type ActionQueue struct {
	dir string
}

// NewActionQueue creates a new ActionQueue backed by the given directory.
// Creates the directory if it does not exist (mode 0700).
func NewActionQueue(dir string) *ActionQueue {
	os.MkdirAll(dir, 0700)
	return &ActionQueue{dir: dir}
}

// pendingPath returns the filesystem path for a pending action file.
func (q *ActionQueue) pendingPath(actionID string) string {
	return filepath.Join(q.dir, actionID+".pending")
}

// responsePath returns the filesystem path for a response file.
func (q *ActionQueue) responsePath(actionID string) string {
	return filepath.Join(q.dir, actionID+".response")
}

// WritePending writes a pending action to disk as JSON.
// File permissions are 0600 (owner read/write only).
func (q *ActionQueue) WritePending(action PendingAction) error {
	data, err := json.MarshalIndent(action, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pending action: %w", err)
	}
	return os.WriteFile(q.pendingPath(action.ActionID), data, 0600)
}

// ReadPending reads a pending action from disk.
func (q *ActionQueue) ReadPending(actionID string) (PendingAction, error) {
	data, err := os.ReadFile(q.pendingPath(actionID))
	if err != nil {
		return PendingAction{}, fmt.Errorf("read pending action %s: %w", actionID, err)
	}
	var action PendingAction
	if err := json.Unmarshal(data, &action); err != nil {
		return PendingAction{}, fmt.Errorf("unmarshal pending action %s: %w", actionID, err)
	}
	return action, nil
}

// WriteResponse atomically writes a response file using O_CREAT|O_EXCL.
// Returns an error if the file already exists (first-response-wins).
// This maps to the noclobber pattern from the architecture research.
func (q *ActionQueue) WriteResponse(actionID string, resp ActionResponse) error {
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	path := q.responsePath(actionID)

	// O_CREAT|O_EXCL|O_WRONLY: create file exclusively, fail if it exists.
	// This is the Go equivalent of bash's `set -o noclobber; echo > file`.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("action %s already responded (first-response-wins): %w", actionID, err)
		}
		return fmt.Errorf("create response file for %s: %w", actionID, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		// Best effort: remove the partially written file
		os.Remove(path)
		return fmt.Errorf("write response for %s: %w", actionID, err)
	}
	return nil
}

// ReadResponse reads a response file from disk.
func (q *ActionQueue) ReadResponse(actionID string) (ActionResponse, error) {
	data, err := os.ReadFile(q.responsePath(actionID))
	if err != nil {
		return ActionResponse{}, fmt.Errorf("read response %s: %w", actionID, err)
	}
	var resp ActionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ActionResponse{}, fmt.Errorf("unmarshal response %s: %w", actionID, err)
	}
	return resp, nil
}

// HasResponse returns true if a response file exists for the given action.
func (q *ActionQueue) HasResponse(actionID string) bool {
	_, err := os.Stat(q.responsePath(actionID))
	return err == nil
}

// Cleanup removes both the pending and response files for an action.
func (q *ActionQueue) Cleanup(actionID string) {
	os.Remove(q.pendingPath(actionID))
	os.Remove(q.responsePath(actionID))
}

// CleanupExpired removes all expired pending actions.
// Returns the number of actions removed.
func (q *ActionQueue) CleanupExpired() int {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return 0
	}

	removed := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".pending") {
			continue
		}

		actionID := strings.TrimSuffix(entry.Name(), ".pending")
		action, err := q.ReadPending(actionID)
		if err != nil {
			continue
		}

		if IsExpired(action.ExpiresAt) {
			q.Cleanup(actionID)
			removed++
		}
	}
	return removed
}

// ListPending returns all non-expired pending actions.
func (q *ActionQueue) ListPending() ([]PendingAction, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list pending actions: %w", err)
	}

	var result []PendingAction
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".pending") {
			continue
		}

		actionID := strings.TrimSuffix(entry.Name(), ".pending")
		action, err := q.ReadPending(actionID)
		if err != nil {
			continue
		}

		if !IsExpired(action.ExpiresAt) {
			result = append(result, action)
		}
	}
	return result, nil
}
