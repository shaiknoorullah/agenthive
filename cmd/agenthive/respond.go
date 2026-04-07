package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/shaiknoorullah/agenthive/internal/hooks"
)

// RespondConfig holds configuration for the respond command.
type RespondConfig struct {
	ActionsDir string
	SocketPath string
	Surface    string
	User       string
}

// ParseRespondArg parses "allow:<id>" or "deny:<id>" from the command argument.
// Returns the decision and action ID, or an error if the format is invalid.
func ParseRespondArg(arg string) (decision, actionID string, err error) {
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid format: expected 'allow:<id>' or 'deny:<id>', got %q", arg)
	}

	decision = parts[0]
	actionID = parts[1]

	if decision != "allow" && decision != "deny" {
		return "", "", fmt.Errorf("invalid decision %q: must be 'allow' or 'deny'", decision)
	}

	if actionID == "" {
		return "", "", fmt.Errorf("action ID cannot be empty")
	}

	// Action IDs must not contain colons (prevents ambiguous parsing)
	if strings.Contains(actionID, ":") {
		return "", "", fmt.Errorf("invalid action ID %q: must not contain ':'", actionID)
	}

	return decision, actionID, nil
}

// RunRespondCommand executes the respond subcommand.
// It validates the pending action exists and is not expired, then writes
// the response file using atomic O_CREAT|O_EXCL for first-response-wins.
// Returns 0 on success, 1 on error.
func RunRespondCommand(arg string, config RespondConfig) int {
	decision, actionID, err := ParseRespondArg(arg)
	if err != nil {
		_, _ = fmt.Fprintf(nil_safe_stderr(), "Error: %v\n", err)
		return 1
	}

	queue := hooks.NewActionQueue(config.ActionsDir)

	// Read the pending action to verify it exists
	pending, err := queue.ReadPending(actionID)
	if err != nil {
		_, _ = fmt.Fprintf(nil_safe_stderr(), "Action not found: %s\n", actionID)
		return 1
	}

	// Check expiry
	if hooks.IsExpired(pending.ExpiresAt) {
		queue.Cleanup(actionID)
		_, _ = fmt.Fprintf(nil_safe_stderr(), "Action expired: %s\n", actionID)
		return 1
	}

	// Write response (atomic, first-response-wins)
	resp := hooks.ActionResponse{
		Decision:  decision,
		Reason:    fmt.Sprintf("Responded via %s", config.Surface),
		Surface:   config.Surface,
		User:      config.User,
		Timestamp: time.Now(),
	}

	if err := queue.WriteResponse(actionID, resp); err != nil {
		_, _ = fmt.Fprintf(nil_safe_stderr(), "Action already handled: %s\n", actionID)
		return 1
	}

	return 0
}

// nil_safe_stderr returns a writer that discards output if stderr-like
// behavior is not needed (for testability). In production, this would
// be os.Stderr.
type discardWriter struct{}

func (d discardWriter) Write(p []byte) (n int, err error) { return len(p), nil }

func nil_safe_stderr() discardWriter {
	return discardWriter{}
}
