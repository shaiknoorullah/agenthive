package hooks

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

const (
	// ActionIDBytes is the number of random bytes in an action ID (16 bytes = 32 hex chars).
	ActionIDBytes = 16

	// DestructiveTTL is the TTL in seconds for destructive actions.
	DestructiveTTL = 30
)

// GenerateActionID returns a cryptographically random 32-character hex string.
// Uses crypto/rand, not math/rand -- safe for security-sensitive identifiers.
func GenerateActionID() (string, error) {
	b := make([]byte, ActionIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IsExpired returns true if the given expiry time is in the past or zero.
func IsExpired(expiresAt time.Time) bool {
	if expiresAt.IsZero() {
		return true
	}
	return time.Now().After(expiresAt)
}

// ComputeExpiry returns the expiry time for an action.
// Destructive actions get a reduced TTL (30 seconds).
// Normal actions get the full TTL (in seconds).
func ComputeExpiry(toolName string, toolInput map[string]any, ttl int) time.Time {
	if IsDestructive(toolName, toolInput) {
		return time.Now().Add(time.Duration(DestructiveTTL) * time.Second)
	}
	return time.Now().Add(time.Duration(ttl) * time.Second)
}

// destructivePatterns are substrings that indicate a destructive Bash command.
// Matching is case-insensitive.
var destructivePatterns = []string{
	"rm -rf",
	"rm -f",
	"git push --force",
	"git push --force-with-lease",
	"git reset --hard",
	"git clean -f",
	"drop table",
	"delete from",
	"shutdown",
	"reboot",
	"chmod 777",
}

// IsDestructive returns true if the tool invocation is classified as destructive.
// Only Bash commands are checked -- read-only tools (Read, Grep, Glob) are never
// destructive. Write/Edit could be, but are not flagged since Claude Code already
// shows file diffs for review.
func IsDestructive(toolName string, toolInput map[string]any) bool {
	// Only Bash commands can be destructive
	if toolName != "Bash" {
		return false
	}

	cmdRaw, ok := toolInput["command"]
	if !ok {
		return false
	}

	cmd, ok := cmdRaw.(string)
	if !ok {
		return false
	}

	lower := strings.ToLower(cmd)
	for _, pattern := range destructivePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
