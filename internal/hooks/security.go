package hooks

import "time"

func GenerateActionID() (string, error)                                   { return "", nil }
func IsExpired(expiresAt time.Time) bool                                  { return false }
func ComputeExpiry(toolName string, toolInput map[string]any, ttl int) time.Time {
	return time.Time{}
}
func IsDestructive(toolName string, toolInput map[string]any) bool        { return false }
