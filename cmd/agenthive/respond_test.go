package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRespondArg_Allow(t *testing.T) {
	decision, actionID, err := ParseRespondArg("allow:abc123def456")
	require.NoError(t, err)
	assert.Equal(t, "allow", decision)
	assert.Equal(t, "abc123def456", actionID)
}

func TestParseRespondArg_Deny(t *testing.T) {
	decision, actionID, err := ParseRespondArg("deny:xyz789")
	require.NoError(t, err)
	assert.Equal(t, "deny", decision)
	assert.Equal(t, "xyz789", actionID)
}

func TestParseRespondArg_InvalidFormat(t *testing.T) {
	tests := []string{
		"",
		"allow",
		":abc123",
		"unknown:abc123",
		"allow:",
		"allow:deny:abc",
	}
	for _, arg := range tests {
		t.Run(arg, func(t *testing.T) {
			_, _, err := ParseRespondArg(arg)
			assert.Error(t, err, "should reject invalid format: %q", arg)
		})
	}
}

func TestRunRespondCommand_WritesResponse(t *testing.T) {
	dir := t.TempDir()
	actionsDir := filepath.Join(dir, "actions")

	// Write a pending action first
	q := hooks.NewActionQueue(actionsDir)
	action := hooks.PendingAction{
		ActionID:  "respond-test-1",
		ToolName:  "Bash",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	err := q.WritePending(action)
	require.NoError(t, err)

	exitCode := RunRespondCommand("allow:respond-test-1", RespondConfig{
		ActionsDir: actionsDir,
		SocketPath: "",
		Surface:    "test",
		User:       "testuser",
	})

	assert.Equal(t, 0, exitCode)

	// Verify response was written
	resp, err := q.ReadResponse("respond-test-1")
	require.NoError(t, err)
	assert.Equal(t, "allow", resp.Decision)
	assert.Equal(t, "test", resp.Surface)
	assert.Equal(t, "testuser", resp.User)
}

func TestRunRespondCommand_DenyWritesResponse(t *testing.T) {
	dir := t.TempDir()
	actionsDir := filepath.Join(dir, "actions")

	q := hooks.NewActionQueue(actionsDir)
	action := hooks.PendingAction{
		ActionID:  "respond-test-2",
		ToolName:  "Bash",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	err := q.WritePending(action)
	require.NoError(t, err)

	exitCode := RunRespondCommand("deny:respond-test-2", RespondConfig{
		ActionsDir: actionsDir,
		Surface:    "termux",
		User:       "kai",
	})

	assert.Equal(t, 0, exitCode)

	resp, err := q.ReadResponse("respond-test-2")
	require.NoError(t, err)
	assert.Equal(t, "deny", resp.Decision)
}

func TestRunRespondCommand_ExpiredActionRejects(t *testing.T) {
	dir := t.TempDir()
	actionsDir := filepath.Join(dir, "actions")

	q := hooks.NewActionQueue(actionsDir)
	action := hooks.PendingAction{
		ActionID:  "expired-action",
		ToolName:  "Bash",
		Timestamp: time.Now().Add(-10 * time.Minute),
		ExpiresAt: time.Now().Add(-5 * time.Minute), // already expired
	}
	err := q.WritePending(action)
	require.NoError(t, err)

	exitCode := RunRespondCommand("allow:expired-action", RespondConfig{
		ActionsDir: actionsDir,
	})

	assert.Equal(t, 1, exitCode, "expired actions should be rejected")
}

func TestRunRespondCommand_NonexistentActionRejects(t *testing.T) {
	actionsDir := filepath.Join(t.TempDir(), "actions")

	exitCode := RunRespondCommand("allow:nonexistent", RespondConfig{
		ActionsDir: actionsDir,
	})

	assert.Equal(t, 1, exitCode, "nonexistent actions should be rejected")
}

func TestRunRespondCommand_AlreadyRespondedRejects(t *testing.T) {
	dir := t.TempDir()
	actionsDir := filepath.Join(dir, "actions")

	q := hooks.NewActionQueue(actionsDir)
	action := hooks.PendingAction{
		ActionID:  "double-respond",
		ToolName:  "Bash",
		Timestamp: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	err := q.WritePending(action)
	require.NoError(t, err)

	// First response succeeds
	exitCode := RunRespondCommand("allow:double-respond", RespondConfig{
		ActionsDir: actionsDir,
		Surface:    "phone",
	})
	assert.Equal(t, 0, exitCode)

	// Second response fails (first-response-wins)
	exitCode = RunRespondCommand("deny:double-respond", RespondConfig{
		ActionsDir: actionsDir,
		Surface:    "laptop",
	})
	assert.Equal(t, 1, exitCode, "second response should be rejected (first-response-wins)")
}

func TestRunRespondCommand_InvalidArgRejects(t *testing.T) {
	exitCode := RunRespondCommand("garbage", RespondConfig{
		ActionsDir: t.TempDir(),
	})
	assert.Equal(t, 1, exitCode)
}
