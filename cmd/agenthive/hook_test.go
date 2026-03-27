package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunHook_PreToolUse_ReturnsJSON(t *testing.T) {
	dir := t.TempDir()
	actionsDir := filepath.Join(dir, "actions")

	input := map[string]any{
		"session_id":      "sess-1",
		"cwd":             "/tmp/app",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "echo hello"},
		"tool_use_id":     "tu-1",
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	stdin := bytes.NewReader(inputJSON)
	var stdout bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Pre-seed a response so the gate returns immediately
	q := hooks.NewActionQueue(actionsDir)
	go func() {
		// Wait for the pending file to appear, then write a response
		for i := 0; i < 100; i++ {
			pending, _ := q.ListPending()
			if len(pending) > 0 {
				q.WriteResponse(pending[0].ActionID, hooks.ActionResponse{
					Decision:  "allow",
					Reason:    "test",
					Timestamp: time.Now(),
				})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	exitCode := RunHookCommand(ctx, "PreToolUse", stdin, &stdout, HookConfig{
		ActionsDir:     actionsDir,
		SocketPath:     "",
		TimeoutSeconds: 5,
		PollIntervalMS: 50,
	})

	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout.String(), `"permissionDecision"`)
	assert.Contains(t, stdout.String(), `"allow"`)
}

func TestRunHook_PreToolUse_DenyReturnsExitCode2(t *testing.T) {
	dir := t.TempDir()
	actionsDir := filepath.Join(dir, "actions")

	input := map[string]any{
		"session_id":      "sess-2",
		"cwd":             "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "rm -rf /"},
		"tool_use_id":     "tu-2",
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	stdin := bytes.NewReader(inputJSON)
	var stdout bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	q := hooks.NewActionQueue(actionsDir)
	go func() {
		for i := 0; i < 100; i++ {
			pending, _ := q.ListPending()
			if len(pending) > 0 {
				q.WriteResponse(pending[0].ActionID, hooks.ActionResponse{
					Decision:  "deny",
					Reason:    "too dangerous",
					Timestamp: time.Now(),
				})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	exitCode := RunHookCommand(ctx, "PreToolUse", stdin, &stdout, HookConfig{
		ActionsDir:     actionsDir,
		SocketPath:     "",
		TimeoutSeconds: 5,
		PollIntervalMS: 50,
	})

	assert.Equal(t, 2, exitCode)
}

func TestRunHook_Notification_DispatchesAndExits(t *testing.T) {
	input := map[string]any{
		"session_id":        "sess-3",
		"cwd":               "/tmp",
		"hook_event_name":   "Notification",
		"message":           "Agent waiting",
		"title":             "Permission",
		"notification_type": "permission_prompt",
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	stdin := bytes.NewReader(inputJSON)
	var stdout bytes.Buffer

	ctx := context.Background()

	exitCode := RunHookCommand(ctx, "Notification", stdin, &stdout, HookConfig{
		ActionsDir: t.TempDir(),
		SocketPath: "", // no socket
	})

	assert.Equal(t, 0, exitCode)
}

func TestRunHook_Stop_DispatchesAndExits(t *testing.T) {
	input := map[string]any{
		"session_id":             "sess-4",
		"cwd":                    "/tmp",
		"hook_event_name":        "Stop",
		"stop_hook_active":       false,
		"last_assistant_message": "All done.",
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	stdin := bytes.NewReader(inputJSON)
	var stdout bytes.Buffer

	ctx := context.Background()

	exitCode := RunHookCommand(ctx, "Stop", stdin, &stdout, HookConfig{
		ActionsDir: t.TempDir(),
		SocketPath: "",
	})

	assert.Equal(t, 0, exitCode)
}

func TestRunHook_UnknownEvent_ReturnsZero(t *testing.T) {
	stdin := bytes.NewReader([]byte(`{}`))
	var stdout bytes.Buffer

	ctx := context.Background()

	exitCode := RunHookCommand(ctx, "UnknownEvent", stdin, &stdout, HookConfig{
		ActionsDir: t.TempDir(),
	})

	assert.Equal(t, 0, exitCode,
		"unknown hook events should exit 0 (pass through)")
}

func TestRunHook_InvalidJSON_ReturnsZero(t *testing.T) {
	stdin := bytes.NewReader([]byte("not json"))
	var stdout bytes.Buffer

	ctx := context.Background()

	exitCode := RunHookCommand(ctx, "PreToolUse", stdin, &stdout, HookConfig{
		ActionsDir: t.TempDir(),
	})

	assert.Equal(t, 0, exitCode,
		"invalid input should exit 0 (fall through to normal prompt)")
}

// Suppress unused import
var _ = os.Getenv
