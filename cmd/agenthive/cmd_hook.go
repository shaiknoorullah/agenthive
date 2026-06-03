package main

import (
	"github.com/spf13/cobra"
)

// newHookCmd returns `agenthive hook <event>`. For PreToolUse, it reads a
// Claude Code hook JSON payload from stdin, dials the daemon's Unix socket,
// sends an action_request, waits, and prints the Claude hook output JSON.
//
// If the daemon is not running or the request times out, the hook subcommand
// prints nothing (Claude falls back to its built-in prompt) and exits 0 —
// never fail-closed.
func newHookCmd() *cobra.Command {
	panic("not implemented: agenthive.newHookCmd")
}
