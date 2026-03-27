package hooks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHookInput_ValidJSON(t *testing.T) {
	input := `{
		"session_id": "abc123",
		"transcript_path": "/path/to/transcript.jsonl",
		"cwd": "/home/user/my-app",
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": {
			"command": "npm test",
			"description": "Run tests",
			"timeout": 30000
		},
		"tool_use_id": "tool-use-42"
	}`

	hookInput, err := ParseHookInput([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "abc123", hookInput.SessionID)
	assert.Equal(t, "Bash", hookInput.ToolName)
	assert.Equal(t, "npm test", hookInput.ToolInput["command"])
	assert.Equal(t, "/home/user/my-app", hookInput.CWD)
	assert.Equal(t, "tool-use-42", hookInput.ToolUseID)
}

func TestParseHookInput_InvalidJSON(t *testing.T) {
	_, err := ParseHookInput([]byte("not json"))
	assert.Error(t, err)
}

func TestParseHookInput_MissingToolName(t *testing.T) {
	input := `{"session_id": "abc", "cwd": "/tmp"}`
	hookInput, err := ParseHookInput([]byte(input))
	require.NoError(t, err)
	assert.Empty(t, hookInput.ToolName)
}

func TestBuildDecisionJSON_Allow(t *testing.T) {
	output := BuildDecisionJSON("allow", "User approved via phone")
	assert.Contains(t, string(output), `"permissionDecision": "allow"`)
	assert.Contains(t, string(output), `"permissionDecisionReason": "User approved via phone"`)
	assert.Contains(t, string(output), `"hookEventName": "PreToolUse"`)

	// Verify it's valid JSON
	var parsed map[string]any
	err := json.Unmarshal(output, &parsed)
	require.NoError(t, err)

	hookOutput := parsed["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "allow", hookOutput["permissionDecision"])
}

func TestBuildDecisionJSON_Deny(t *testing.T) {
	output := BuildDecisionJSON("deny", "Blocked by admin")
	assert.Contains(t, string(output), `"permissionDecision": "deny"`)
}

func TestBuildDecisionJSON_Ask(t *testing.T) {
	output := BuildDecisionJSON("ask", "")
	assert.Contains(t, string(output), `"permissionDecision": "ask"`)
}

func TestActionGate_RunGate_ImmediateResponse(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)
	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 10,
		PollIntervalMS: 50,
		SocketPath:     "", // no daemon dispatch in this test
	})

	hookInput := &HookInput{
		SessionID: "sess-1",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "npm test"},
		ToolUseID: "tu-1",
		CWD:       "/home/user/app",
	}

	// Pre-write a response before starting the gate
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start gate in goroutine
	resultCh := make(chan *GateResult, 1)
	go func() {
		result := gate.RunGate(ctx, hookInput)
		resultCh <- result
	}()

	// Write response after a short delay to let the gate write the pending action
	time.Sleep(100 * time.Millisecond)

	// Find the action ID from the pending file
	pending, err := q.ListPending()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	actionID := pending[0].ActionID

	resp := ActionResponse{
		Decision:  "allow",
		Reason:    "Test approval",
		Surface:   "test",
		Timestamp: time.Now(),
	}
	err = q.WriteResponse(actionID, resp)
	require.NoError(t, err)

	result := <-resultCh
	assert.Equal(t, "allow", result.Decision)
	assert.Equal(t, "Test approval", result.Reason)
	assert.Equal(t, 0, result.ExitCode)
}

func TestActionGate_RunGate_Timeout(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)
	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 1,
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	hookInput := &HookInput{
		SessionID: "sess-2",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "ls"},
		ToolUseID: "tu-2",
		CWD:       "/tmp",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result := gate.RunGate(ctx, hookInput)

	// On timeout, gate returns "ask" so the normal terminal prompt takes over
	assert.Equal(t, "ask", result.Decision)
	assert.Equal(t, 0, result.ExitCode)
}

func TestActionGate_RunGate_DenyExitCode(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)
	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 10,
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	hookInput := &HookInput{
		SessionID: "sess-3",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "rm -rf /"},
		ToolUseID: "tu-3",
		CWD:       "/tmp",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *GateResult, 1)
	go func() {
		result := gate.RunGate(ctx, hookInput)
		resultCh <- result
	}()

	time.Sleep(100 * time.Millisecond)

	pending, err := q.ListPending()
	require.NoError(t, err)
	require.Len(t, pending, 1)

	resp := ActionResponse{
		Decision:  "deny",
		Reason:    "Dangerous command",
		Surface:   "test",
		Timestamp: time.Now(),
	}
	err = q.WriteResponse(pending[0].ActionID, resp)
	require.NoError(t, err)

	result := <-resultCh
	assert.Equal(t, "deny", result.Decision)
	assert.Equal(t, 2, result.ExitCode, "deny must return exit code 2")
}

func TestActionGate_RunGate_CleansUpAfterResponse(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)
	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 10,
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	hookInput := &HookInput{
		SessionID: "sess-4",
		ToolName:  "Read",
		ToolInput: map[string]any{"file_path": "/tmp/foo"},
		ToolUseID: "tu-4",
		CWD:       "/tmp",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *GateResult, 1)
	go func() {
		result := gate.RunGate(ctx, hookInput)
		resultCh <- result
	}()

	time.Sleep(100 * time.Millisecond)

	pending, err := q.ListPending()
	require.NoError(t, err)
	require.Len(t, pending, 1)

	resp := ActionResponse{Decision: "allow", Timestamp: time.Now()}
	err = q.WriteResponse(pending[0].ActionID, resp)
	require.NoError(t, err)

	<-resultCh

	// Verify cleanup: no pending files should remain
	remaining, err := q.ListPending()
	require.NoError(t, err)
	assert.Empty(t, remaining, "pending actions should be cleaned up after response")
}

func TestActionGate_RunGate_SetsProjectFromCWD(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)
	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 10,
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	hookInput := &HookInput{
		SessionID: "sess-5",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "ls"},
		ToolUseID: "tu-5",
		CWD:       "/home/user/projects/my-api-server",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *GateResult, 1)
	go func() {
		result := gate.RunGate(ctx, hookInput)
		resultCh <- result
	}()

	time.Sleep(100 * time.Millisecond)

	pending, err := q.ListPending()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "my-api-server", pending[0].Project,
		"project should be derived from last component of CWD")

	// Clean up by responding
	q.WriteResponse(pending[0].ActionID, ActionResponse{Decision: "allow", Timestamp: time.Now()})
	<-resultCh
}

func TestActionGate_RunGate_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)
	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 300,
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	hookInput := &HookInput{
		SessionID: "sess-6",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "ls"},
		ToolUseID: "tu-6",
		CWD:       "/tmp",
	}

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan *GateResult, 1)
	go func() {
		result := gate.RunGate(ctx, hookInput)
		resultCh <- result
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	result := <-resultCh
	assert.Equal(t, "ask", result.Decision, "context cancellation should fall through to ask")
}
