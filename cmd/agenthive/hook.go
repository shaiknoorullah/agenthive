package main

import (
	"context"
	"io"
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
func RunHookCommand(ctx context.Context, event string, stdin io.Reader, stdout io.Writer, config HookConfig) int {
	return 0
}
