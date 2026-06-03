// Package hooks implements the local-device action gate: a file-backed queue
// of pending action requests that out-of-band surfaces (other peers, manual
// CLI overrides) can resolve into allow/deny decisions.
//
// security.go classifies tool invocations into ActionNormal or
// ActionDestructive so the gate can apply a tighter timeout to destructive
// operations.
package hooks

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// ActionType tags how risky a tool invocation is.
type ActionType int

const (
	// ActionNormal is the default classification.
	ActionNormal ActionType = iota
	// ActionDestructive marks invocations whose toolName or toolInput matches
	// a known destructive pattern (rm -rf, git push --force, DROP TABLE, ...).
	ActionDestructive
)

// destructivePatterns is the set of lower-cased substrings that, if found in
// either toolName or toolInput, mark the action as destructive. The list is
// intentionally a superset of what most tools care about — false positives
// (e.g. `rm -rf .DS_Store`) are acceptable because the cost of an extra
// confirmation is far lower than the cost of an unconfirmed destructive op.
var destructivePatterns = []string{
	"rm -rf",
	"rm -fr",
	"git push --force",
	"git push -f",
	"git reset --hard",
	"git clean -f",
	"drop table",
	"delete from",
	"truncate",
	"reboot",
	"shutdown",
	"chmod 777",
	"chown -r",
	"dd if=",
	"mkfs",
	"> /dev/",
	":>!",
	"rm -- *",
}

// Classify tags shell-style or tool-name patterns matching common destructive
// operations. Case-insensitive substring matching on toolName + toolInput.
func Classify(toolName, toolInput string) ActionType {
	haystack := strings.ToLower(toolName + " " + toolInput)
	for _, p := range destructivePatterns {
		if strings.Contains(haystack, p) {
			return ActionDestructive
		}
	}
	return ActionNormal
}

// GenerateActionID returns a 16-byte hex random ID.
func GenerateActionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
