package hooks

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_FullActionCycle(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 10,
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	input := &HookInput{
		SessionID: "integration-sess",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "npm test"},
		ToolUseID: "tu-integration",
		CWD:       "/home/user/projects/my-app",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *GateResult, 1)
	go func() {
		resultCh <- gate.RunGate(ctx, input)
	}()

	// Simulate a remote peer responding after a short delay
	time.Sleep(200 * time.Millisecond)

	// Find the pending action
	pending, err := q.ListPending()
	require.NoError(t, err)
	require.Len(t, pending, 1)

	// Verify the pending action has correct metadata
	assert.Equal(t, "Bash", pending[0].ToolName)
	assert.Equal(t, "my-app", pending[0].Project)
	assert.Equal(t, "npm test", pending[0].ToolInput["command"])
	assert.False(t, IsExpired(pending[0].ExpiresAt))

	// Write response as if from a remote peer
	err = q.WriteResponse(pending[0].ActionID, ActionResponse{
		Decision:  "allow",
		Reason:    "Approved by kai via phone",
		Surface:   "termux",
		User:      "kai",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	// Wait for gate to pick up the response
	result := <-resultCh

	assert.Equal(t, "allow", result.Decision)
	assert.Equal(t, "Approved by kai via phone", result.Reason)
	assert.Equal(t, 0, result.ExitCode)

	// Verify JSON output is valid and correct
	var parsed map[string]any
	err = json.Unmarshal(result.JSON, &parsed)
	require.NoError(t, err)

	hookOutput := parsed["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "allow", hookOutput["permissionDecision"])
	assert.Equal(t, "PreToolUse", hookOutput["hookEventName"])

	// Verify cleanup happened
	remaining, err := q.ListPending()
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestIntegration_DenyBlocksWithExitCode2(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 10,
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	input := &HookInput{
		SessionID: "deny-sess",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "rm -rf /"},
		ToolUseID: "tu-deny",
		CWD:       "/tmp",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *GateResult, 1)
	go func() {
		resultCh <- gate.RunGate(ctx, input)
	}()

	time.Sleep(200 * time.Millisecond)

	pending, err := q.ListPending()
	require.NoError(t, err)
	require.Len(t, pending, 1)

	// Destructive command: verify reduced TTL
	assert.True(t, pending[0].ExpiresAt.Before(time.Now().Add(31*time.Second)),
		"destructive actions should have reduced TTL")

	err = q.WriteResponse(pending[0].ActionID, ActionResponse{
		Decision:  "deny",
		Reason:    "Blocked: destructive command",
		Surface:   "desktop",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	result := <-resultCh

	assert.Equal(t, "deny", result.Decision)
	assert.Equal(t, 2, result.ExitCode)

	var parsed map[string]any
	err = json.Unmarshal(result.JSON, &parsed)
	require.NoError(t, err)
	hookOutput := parsed["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "deny", hookOutput["permissionDecision"])
}

func TestIntegration_TimeoutFallsThrough(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 1, // very short timeout
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	input := &HookInput{
		SessionID: "timeout-sess",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "ls"},
		ToolUseID: "tu-timeout",
		CWD:       "/tmp",
	}

	ctx := context.Background()
	result := gate.RunGate(ctx, input)

	assert.Equal(t, "ask", result.Decision)
	assert.Equal(t, 0, result.ExitCode)

	// Verify JSON output returns "ask"
	var parsed map[string]any
	err := json.Unmarshal(result.JSON, &parsed)
	require.NoError(t, err)
	hookOutput := parsed["hookSpecificOutput"].(map[string]any)
	assert.Equal(t, "ask", hookOutput["permissionDecision"])
}

func TestIntegration_FirstResponseWins_ConcurrentResponses(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 10,
		PollIntervalMS: 50,
		SocketPath:     "",
	})

	input := &HookInput{
		SessionID: "race-sess",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "echo race"},
		ToolUseID: "tu-race",
		CWD:       "/tmp",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *GateResult, 1)
	go func() {
		resultCh <- gate.RunGate(ctx, input)
	}()

	time.Sleep(200 * time.Millisecond)

	pending, err := q.ListPending()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	actionID := pending[0].ActionID

	// Simulate concurrent responses from multiple surfaces
	// Only one should succeed (first-response-wins via O_CREAT|O_EXCL)
	wins := 0
	for i := 0; i < 5; i++ {
		resp := ActionResponse{
			Decision:  "allow",
			Reason:    "Concurrent responder",
			Surface:   "surface",
			Timestamp: time.Now(),
		}
		err := q.WriteResponse(actionID, resp)
		if err == nil {
			wins++
		}
	}
	assert.Equal(t, 1, wins, "exactly one concurrent response must win")

	result := <-resultCh
	assert.Equal(t, "allow", result.Decision)
}

func TestIntegration_DaemonDispatch_ReceivesActionRequest(t *testing.T) {
	dir := t.TempDir()
	q := NewActionQueue(dir)

	socketPath := filepath.Join(dir, "daemon.sock")

	// Start mock daemon
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
		buf := make([]byte, 8192)
		n, _ := conn.Read(buf)
		received <- buf[:n]
	}()

	gate := NewActionGate(q, ActionGateConfig{
		TimeoutSeconds: 2,
		PollIntervalMS: 50,
		SocketPath:     socketPath,
	})

	input := &HookInput{
		SessionID: "dispatch-sess",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "npm test"},
		ToolUseID: "tu-dispatch",
		CWD:       "/home/user/api",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start gate (will dispatch and then timeout)
	go gate.RunGate(ctx, input)

	// Wait for the daemon to receive the dispatch
	select {
	case data := <-received:
		assert.Contains(t, string(data), `"type":"action_request"`)
		assert.Contains(t, string(data), `"npm test"`)

		// Verify it's newline-delimited JSON
		assert.True(t, len(data) > 0 && data[len(data)-1] == '\n')

		// Verify JSON is valid
		var msg map[string]any
		err = json.Unmarshal(data[:len(data)-1], &msg)
		require.NoError(t, err)
		assert.Equal(t, "action_request", msg["type"])

	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not receive dispatch within timeout")
	}
}
