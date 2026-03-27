package main

// RespondConfig holds configuration for the respond command.
type RespondConfig struct {
	ActionsDir string
	SocketPath string
	Surface    string
	User       string
}

// ParseRespondArg parses "allow:<id>" or "deny:<id>" from the command argument.
func ParseRespondArg(arg string) (decision, actionID string, err error) {
	return "", "", nil
}

// RunRespondCommand executes the respond subcommand.
// Returns 0 on success, 1 on error.
func RunRespondCommand(arg string, config RespondConfig) int {
	return 0
}
