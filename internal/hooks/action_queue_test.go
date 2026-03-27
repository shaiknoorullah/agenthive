package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionQueue_WritePending_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	action := PendingAction{
		ActionID:  "abc123def456",
		SessionID: "session-1",
		ToolUseID: "tool-1",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "npm test"},
		Project:   "my-app",
		PaneID:    "%42",
		CWD:       "/home/user/my-app",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	err := q.WritePending(action)
	require.NoError(t, err)

	path := filepath.Join(dir, "abc123def456.pending")
	assert.FileExists(t, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var loaded PendingAction
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.Equal(t, "abc123def456", loaded.ActionID)
	assert.Equal(t, "Bash", loaded.ToolName)
	assert.Equal(t, "npm test", loaded.ToolInput["command"])
}

func TestActionQueue_WritePending_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	action := PendingAction{
		ActionID:  "perm-test",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	err := q.WritePending(action)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, "perm-test.pending"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(),
		"pending file must be readable/writable only by owner")
}

func TestActionQueue_ReadPending_ReturnsAction(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	action := PendingAction{
		ActionID:  "read-test",
		ToolName:  "Write",
		ToolInput: map[string]any{"file_path": "/tmp/out.txt"},
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	err := q.WritePending(action)
	require.NoError(t, err)

	loaded, err := q.ReadPending("read-test")
	require.NoError(t, err)
	assert.Equal(t, "Write", loaded.ToolName)
}

func TestActionQueue_ReadPending_NonexistentReturnsError(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	_, err := q.ReadPending("nonexistent")
	assert.Error(t, err)
}

func TestActionQueue_WriteResponse_AtomicFirstWins(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	resp := ActionResponse{
		Decision:  "allow",
		Reason:    "Approved via phone",
		Surface:   "termux",
		User:      "kai",
		Timestamp: time.Now(),
	}

	// First write should succeed
	err := q.WriteResponse("first-wins", resp)
	require.NoError(t, err)

	// Second write should fail (file already exists)
	resp2 := ActionResponse{
		Decision: "deny",
		Reason:   "Second responder",
		Surface:  "slack",
	}
	err = q.WriteResponse("first-wins", resp2)
	assert.Error(t, err, "second response must fail (first-response-wins)")
}

func TestActionQueue_WriteResponse_ContentCorrect(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	resp := ActionResponse{
		Decision:  "deny",
		Reason:    "Looks dangerous",
		Surface:   "desktop",
		User:      "admin",
		Timestamp: time.Now(),
	}

	err := q.WriteResponse("content-check", resp)
	require.NoError(t, err)

	loaded, err := q.ReadResponse("content-check")
	require.NoError(t, err)
	assert.Equal(t, "deny", loaded.Decision)
	assert.Equal(t, "Looks dangerous", loaded.Reason)
	assert.Equal(t, "desktop", loaded.Surface)
}

func TestActionQueue_ReadResponse_NonexistentReturnsError(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	_, err := q.ReadResponse("nonexistent")
	assert.Error(t, err)
}

func TestActionQueue_HasResponse_ReturnsTrueWhenExists(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	assert.False(t, q.HasResponse("check-id"))

	resp := ActionResponse{Decision: "allow"}
	err := q.WriteResponse("check-id", resp)
	require.NoError(t, err)

	assert.True(t, q.HasResponse("check-id"))
}

func TestActionQueue_Cleanup_RemovesPendingAndResponse(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	action := PendingAction{
		ActionID:  "cleanup-test",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	err := q.WritePending(action)
	require.NoError(t, err)

	resp := ActionResponse{Decision: "allow"}
	err = q.WriteResponse("cleanup-test", resp)
	require.NoError(t, err)

	q.Cleanup("cleanup-test")

	assert.NoFileExists(t, filepath.Join(dir, "cleanup-test.pending"))
	assert.NoFileExists(t, filepath.Join(dir, "cleanup-test.response"))
}

func TestActionQueue_CleanupExpired_RemovesOldActions(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	// Write an already-expired action
	expired := PendingAction{
		ActionID:  "expired-1",
		Timestamp: time.Now().Add(-10 * time.Minute),
		ExpiresAt: time.Now().Add(-5 * time.Minute),
	}
	err := q.WritePending(expired)
	require.NoError(t, err)

	// Write a still-valid action
	valid := PendingAction{
		ActionID:  "valid-1",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	err = q.WritePending(valid)
	require.NoError(t, err)

	removed := q.CleanupExpired()
	assert.Equal(t, 1, removed)

	assert.NoFileExists(t, filepath.Join(dir, "expired-1.pending"))
	assert.FileExists(t, filepath.Join(dir, "valid-1.pending"))
}

func TestActionQueue_ListPending_ReturnsNonExpired(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	a1 := PendingAction{
		ActionID:  "list-1",
		ToolName:  "Bash",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	a2 := PendingAction{
		ActionID:  "list-2",
		ToolName:  "Write",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	require.NoError(t, q.WritePending(a1))
	require.NoError(t, q.WritePending(a2))

	pending, err := q.ListPending()
	require.NoError(t, err)
	assert.Len(t, pending, 2)
}
