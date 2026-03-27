package hooks

import "context"

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
	TimeoutSeconds int
	PollIntervalMS int
	SocketPath     string
}

// GateResult is the outcome of running the action gate.
type GateResult struct {
	Decision string // "allow", "deny", or "ask"
	Reason   string
	ExitCode int    // 0 for allow/ask, 2 for deny
	JSON     []byte // JSON output for stdout
}

// ActionGate implements the PreToolUse hook blocking logic.
type ActionGate struct{}

func NewActionGate(queue *ActionQueue, config ActionGateConfig) *ActionGate { return nil }
func ParseHookInput(data []byte) (*HookInput, error)                       { return nil, nil }
func BuildDecisionJSON(decision, reason string) []byte                     { return nil }
func (g *ActionGate) RunGate(ctx context.Context, input *HookInput) *GateResult {
	return nil
}
