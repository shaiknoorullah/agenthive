package main

import (
	"context"
	"io"

	"github.com/shaiknoorullah/agenthive/internal/hooks"
)

// HookConfig holds configuration for hook command execution.
type HookConfig struct {
	ActionsDir     string
	SocketPath     string
	TimeoutSeconds int
	PollIntervalMS int
}

// RunHookCommand processes a hook event from Claude Code.
// It reads JSON from stdin, processes the event, and writes output to stdout.
// Returns the exit code (0 = allow/ask, 2 = deny).
//
// Hook events:
//   - PreToolUse: blocks, waits for user decision, returns JSON with permissionDecision
//   - Notification: dispatches to daemon, returns immediately
//   - Stop: dispatches to daemon, returns immediately
//   - Unknown: passes through (exit 0)
func RunHookCommand(ctx context.Context, event string, stdin io.Reader, stdout io.Writer, config HookConfig) int {
	// Read all of stdin
	data, err := io.ReadAll(stdin)
	if err != nil {
		return 0 // fall through on read error
	}

	switch event {
	case "PreToolUse":
		return handlePreToolUse(ctx, data, stdout, config)
	case "Notification":
		return handleNotification(data, config)
	case "Stop":
		return handleStop(data, config)
	default:
		return 0 // unknown events pass through
	}
}

// handlePreToolUse processes a PreToolUse hook event.
// This is the blocking path: it writes a pending action, dispatches to the daemon,
// and polls for a response from any surface.
func handlePreToolUse(ctx context.Context, data []byte, stdout io.Writer, config HookConfig) int {
	input, err := hooks.ParseHookInput(data)
	if err != nil {
		// Cannot parse input -- fall through to normal prompt
		_, _ = stdout.Write(hooks.BuildDecisionJSON("ask", ""))
		return 0
	}

	queue := hooks.NewActionQueue(config.ActionsDir)
	gate := hooks.NewActionGate(queue, hooks.ActionGateConfig{
		TimeoutSeconds: config.TimeoutSeconds,
		PollIntervalMS: config.PollIntervalMS,
		SocketPath:     config.SocketPath,
	})

	result := gate.RunGate(ctx, input)

	_, _ = stdout.Write(result.JSON)
	return result.ExitCode
}

// handleNotification processes a Notification hook event.
// Non-blocking: dispatches to daemon and returns immediately.
func handleNotification(data []byte, config HookConfig) int {
	notif, err := hooks.ParseNotificationInput(data)
	if err != nil {
		return 0
	}

	_ = hooks.HandleNotification(notif, config.SocketPath)
	return 0
}

// handleStop processes a Stop hook event.
// Non-blocking: dispatches to daemon and returns immediately.
func handleStop(data []byte, config HookConfig) int {
	stop, err := hooks.ParseStopInput(data)
	if err != nil {
		return 0
	}

	_ = hooks.HandleStop(stop, config.SocketPath)
	return 0
}
