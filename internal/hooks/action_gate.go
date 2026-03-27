package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"time"
)

// HookInput represents the JSON data Claude Code sends to hooks on stdin.
type HookInput struct {
	SessionID      string         `json:"session_id"`
	TranscriptPath string         `json:"transcript_path,omitempty"`
	CWD            string         `json:"cwd"`
	PermissionMode string         `json:"permission_mode,omitempty"`
	HookEventName  string         `json:"hook_event_name"`
	ToolName       string         `json:"tool_name"`
	ToolInput      map[string]any `json:"tool_input"`
	ToolUseID      string         `json:"tool_use_id"`
}

// ActionGateConfig holds configuration for the action gate.
type ActionGateConfig struct {
	TimeoutSeconds int    // Maximum seconds to wait for a response
	PollIntervalMS int    // Milliseconds between response file checks
	SocketPath     string // Unix socket path for daemon dispatch (empty = skip)
}

// GateResult is the outcome of running the action gate.
type GateResult struct {
	Decision string // "allow", "deny", or "ask"
	Reason   string
	ExitCode int    // 0 for allow/ask, 2 for deny
	JSON     []byte // JSON output for stdout
}

// ActionGate implements the PreToolUse hook blocking logic.
// It writes a pending action, dispatches via Unix socket, polls for response.
type ActionGate struct {
	queue  *ActionQueue
	config ActionGateConfig
}

// NewActionGate creates a new action gate with the given queue and config.
func NewActionGate(queue *ActionQueue, config ActionGateConfig) *ActionGate {
	return &ActionGate{
		queue:  queue,
		config: config,
	}
}

// ParseHookInput parses the JSON data Claude Code sends on stdin.
func ParseHookInput(data []byte) (*HookInput, error) {
	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("parse hook input: %w", err)
	}
	return &input, nil
}

// BuildDecisionJSON constructs the JSON output Claude Code expects on stdout.
// This follows the hookSpecificOutput format from Claude Code's hook protocol.
func BuildDecisionJSON(decision, reason string) []byte {
	type hookOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	}
	type output struct {
		HookSpecificOutput hookOutput `json:"hookSpecificOutput"`
	}

	data, err := json.MarshalIndent(output{
		HookSpecificOutput: hookOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: reason,
		},
	}, "", "  ")
	if err != nil {
		// Fallback: return minimal valid JSON
		return []byte(fmt.Sprintf(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"%s"}}`, decision))
	}
	return data
}

// RunGate executes the full action gate lifecycle:
// 1. Generate action ID
// 2. Write pending action to queue
// 3. Dispatch to daemon via Unix socket (best effort)
// 4. Poll for response file
// 5. Return decision or fall through to "ask" on timeout
func (g *ActionGate) RunGate(ctx context.Context, input *HookInput) *GateResult {
	// Generate cryptographically random action ID
	actionID, err := GenerateActionID()
	if err != nil {
		// Cannot generate ID -- fall through to normal prompt
		return g.askResult("failed to generate action ID")
	}

	// Derive project name from CWD
	project := filepath.Base(input.CWD)

	// Compute expiry
	expiresAt := ComputeExpiry(input.ToolName, input.ToolInput, g.config.TimeoutSeconds)

	// Write pending action
	action := PendingAction{
		ActionID:  actionID,
		SessionID: input.SessionID,
		ToolUseID: input.ToolUseID,
		ToolName:  input.ToolName,
		ToolInput: input.ToolInput,
		Project:   project,
		PaneID:    "", // set by caller from TMUX_PANE env var
		CWD:       input.CWD,
		Timestamp: time.Now(),
		ExpiresAt: expiresAt,
	}

	if err := g.queue.WritePending(action); err != nil {
		return g.askResult("failed to write pending action")
	}

	// Dispatch to daemon via Unix socket (best effort, non-blocking)
	if g.config.SocketPath != "" {
		go g.dispatchToDaemon(action)
	}

	// Poll for response
	pollInterval := time.Duration(g.config.PollIntervalMS) * time.Millisecond
	if pollInterval == 0 {
		pollInterval = 500 * time.Millisecond
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	timeout := time.Duration(g.config.TimeoutSeconds) * time.Second
	deadline := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			g.queue.Cleanup(actionID)
			return g.askResult("context cancelled")

		case <-deadline:
			g.queue.Cleanup(actionID)
			return g.askResult("timeout waiting for response")

		case <-ticker.C:
			if g.queue.HasResponse(actionID) {
				resp, err := g.queue.ReadResponse(actionID)
				if err != nil {
					g.queue.Cleanup(actionID)
					return g.askResult("failed to read response")
				}

				g.queue.Cleanup(actionID)

				exitCode := 0
				if resp.Decision == "deny" {
					exitCode = 2
				}

				return &GateResult{
					Decision: resp.Decision,
					Reason:   resp.Reason,
					ExitCode: exitCode,
					JSON:     BuildDecisionJSON(resp.Decision, resp.Reason),
				}
			}
		}
	}
}

// askResult returns a GateResult that falls through to the normal terminal prompt.
func (g *ActionGate) askResult(reason string) *GateResult {
	return &GateResult{
		Decision: "ask",
		Reason:   reason,
		ExitCode: 0,
		JSON:     BuildDecisionJSON("ask", ""),
	}
}

// dispatchToDaemon sends the pending action to the daemon via Unix socket.
// This is best-effort: if the daemon is not running, the action still works
// via local notification surfaces. The daemon handles remote routing.
func (g *ActionGate) dispatchToDaemon(action PendingAction) {
	conn, err := net.DialTimeout("unix", g.config.SocketPath, 2*time.Second)
	if err != nil {
		return // daemon not available, degrade gracefully
	}
	defer conn.Close()

	msg := struct {
		Type   string        `json:"type"`
		Action PendingAction `json:"action"`
	}{
		Type:   "action_request",
		Action: action,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	// Write JSON + newline (newline-delimited JSON protocol)
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write(append(data, '\n'))
}
