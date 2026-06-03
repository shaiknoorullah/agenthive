// Package hooks implements the local-device action gate: a file-backed queue
// of pending action requests that out-of-band surfaces (other peers, manual
// CLI overrides) can resolve into allow/deny decisions.
//
// security.go classifies tool invocations into ActionNormal or
// ActionDestructive so the gate can apply a tighter timeout to destructive
// operations.
package hooks

// ActionType tags how risky a tool invocation is.
type ActionType int

const (
	// ActionNormal is the default classification.
	ActionNormal ActionType = iota
	// ActionDestructive marks invocations whose toolName or toolInput matches
	// a known destructive pattern (rm -rf, git push --force, DROP TABLE, ...).
	ActionDestructive
)

// Classify tags shell-style or tool-name patterns matching common destructive
// operations. Case-insensitive substring matching on toolName + toolInput.
func Classify(toolName, toolInput string) ActionType {
	panic("not implemented: hooks.Classify")
}

// GenerateActionID returns a 16-byte hex random ID.
func GenerateActionID() (string, error) {
	panic("not implemented: hooks.GenerateActionID")
}
