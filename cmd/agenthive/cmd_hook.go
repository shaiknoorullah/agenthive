package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaiknoorullah/agenthive/internal/daemon"
	"github.com/shaiknoorullah/agenthive/internal/hooks"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// hookDialTimeout caps how long the hook subcommand will wait for the daemon
// to accept the socket connection. The daemon's accept loop is fast so any
// meaningful delay here means it is not running.
const hookDialTimeout = 2 * time.Second

// hookOverallTimeout caps the total wall-clock time the hook subcommand will
// wait for a decision (including any out-of-band human approval that has to
// land in the queue). 6 minutes leaves slack on top of the gate's 5-minute
// default so a fast-path decision always arrives before this fires.
const hookOverallTimeout = 6 * time.Minute

// preToolUseEvent is the JSON document Claude Code pipes to the hook
// subcommand on stdin. Only the fields agenthive currently routes on are
// modelled; extra fields are tolerated and ignored.
type preToolUseEvent struct {
	HookEvent string `json:"hook_event"`
	Tool      string `json:"tool"`
	Input     string `json:"input"`
	SessionID string `json:"session_id"`
	ToolUseID string `json:"tool_use_id"`
	Project   string `json:"project,omitempty"`
	CWD       string `json:"cwd,omitempty"`
}

// hookSpecificOutput is the JSON document Claude expects on stdout to drive
// the PreToolUse decision. The schema mirrors Claude's published hook
// contract: a top-level "hookSpecificOutput" object containing a single
// "permissionDecision" string of "allow" | "deny" | "ask".
type hookSpecificOutput struct {
	HookSpecificOutput struct {
		PermissionDecision string `json:"permissionDecision"`
	} `json:"hookSpecificOutput"`
}

// newHookCmd returns `agenthive hook <event>`. For PreToolUse, it reads a
// Claude Code hook JSON payload from stdin, dials the daemon's Unix socket,
// sends an action_request, waits, and prints the Claude hook output JSON.
//
// CRITICAL fail-open contract: if the daemon is not running, the socket dial
// times out, the daemon errors, or stdin is malformed, the hook subcommand
// prints nothing and exits 0. Claude then falls back to its built-in prompt.
// Never fail-closed — that would block every tool call when agenthive is
// offline, which is unacceptable for a tool the user has installed as a
// background convenience.
func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hook <event>",
		Short: "Run a Claude Code hook through the agenthive daemon",
		Long: "Reads a Claude Code hook JSON payload from stdin, asks the " +
			"running daemon for a decision via the local Unix socket, and " +
			"writes the Claude hook output JSON to stdout. Unknown event " +
			"types are a no-op. Daemon unreachable or unresponsive is also " +
			"a no-op — Claude then falls back to its built-in prompt.",
		Args: cobra.ExactArgs(1),
		// We deliberately return nil from RunE in every error path: cobra
		// surfaces a non-nil error as a non-zero exit, which would break
		// our fail-open contract. Errors are still useful to the human
		// running the daemon in debug, but they go nowhere from here.
		RunE: func(cmd *cobra.Command, args []string) error {
			event := args[0]
			if event != "PreToolUse" {
				// Other hook events are not yet routed; print nothing.
				return nil
			}

			body, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return nil
			}
			var ev preToolUseEvent
			if err := json.Unmarshal(body, &ev); err != nil {
				// Malformed stdin → fall back silently.
				return nil
			}

			decision := dialDaemonForDecision(configDir, ev)
			if decision == "" {
				// Daemon unreachable or no decision returned — fall back.
				return nil
			}
			emitHookDecision(cmd.OutOrStdout(), decision)
			return nil
		},
	}
}

// dialDaemonForDecision runs one full request/response over the daemon's
// Unix socket and returns the decision string, or "" if anything went wrong
// (and the caller should fall back to Claude's built-in prompt).
func dialDaemonForDecision(cfgDir string, ev preToolUseEvent) string {
	socketPath := filepath.Join(cfgDir, "agenthive.sock")

	dialer := net.Dialer{Timeout: hookDialTimeout}
	conn, err := dialer.Dial("unix", socketPath)
	if err != nil {
		return ""
	}
	defer func() { _ = conn.Close() }()

	overall, cancel := context.WithTimeout(context.Background(), hookOverallTimeout)
	defer cancel()
	deadline, _ := overall.Deadline()
	_ = conn.SetDeadline(deadline)

	actionID, err := hooks.GenerateActionID()
	if err != nil {
		return ""
	}
	req := protocols.ActionRequest{
		ActionID:  actionID,
		SessionID: ev.SessionID,
		ToolUseID: ev.ToolUseID,
		ToolName:  ev.Tool,
		ToolInput: ev.Input,
		Project:   ev.Project,
		CWD:       ev.CWD,
		Timestamp: time.Now().UTC(),
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return ""
	}
	if err := protocols.WriteFramed(conn, daemon.SocketEnvelope{
		Kind:    daemon.KindActionRequest,
		Payload: payload,
	}); err != nil {
		return ""
	}

	var env daemon.SocketEnvelope
	if err := protocols.ReadFramed(conn, &env); err != nil {
		return ""
	}
	if env.Kind != daemon.KindActionResponse {
		// kind:"error" or anything else → fall back.
		return ""
	}
	var resp protocols.ActionResponse
	if err := json.Unmarshal(env.Payload, &resp); err != nil {
		return ""
	}
	return resp.Decision
}

// emitHookDecision writes the Claude hookSpecificOutput JSON document to w.
// The Decision string is normalised: agenthive's gate may return
// "allow-always", which we collapse to "allow" for Claude's vocabulary, and
// any unknown decision falls through as the raw string (Claude will reject
// it, which is the right signal back to the human).
func emitHookDecision(w io.Writer, decision string) {
	out := hookSpecificOutput{}
	switch decision {
	case "allow", "allow-always":
		out.HookSpecificOutput.PermissionDecision = "allow"
	case "deny":
		out.HookSpecificOutput.PermissionDecision = "deny"
	default:
		out.HookSpecificOutput.PermissionDecision = decision
	}
	body, err := json.Marshal(out)
	if err != nil {
		return
	}
	// Writes to the hook's stdout are best-effort: if Claude has already
	// closed the pipe (e.g. session torn down) there is nothing the hook can
	// usefully do with the error, and we don't want to spam stderr from a
	// process Claude is about to reap. Drop the error explicitly.
	_, _ = fmt.Fprintln(w, string(body))
}
