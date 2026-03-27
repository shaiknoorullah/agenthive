package hooks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateActionID_Returns32HexChars(t *testing.T) {
	id, err := GenerateActionID()
	require.NoError(t, err)
	assert.Len(t, id, 32, "action ID must be 32 hex characters (16 bytes)")
}

func TestGenerateActionID_IsHex(t *testing.T) {
	id, err := GenerateActionID()
	require.NoError(t, err)
	for _, c := range id {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"action ID must be lowercase hex, got %c", c)
	}
}

func TestGenerateActionID_IsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateActionID()
		require.NoError(t, err)
		assert.False(t, seen[id], "duplicate action ID generated")
		seen[id] = true
	}
}

func TestIsExpired_FutureTimestamp_NotExpired(t *testing.T) {
	expiresAt := time.Now().Add(5 * time.Minute)
	assert.False(t, IsExpired(expiresAt))
}

func TestIsExpired_PastTimestamp_Expired(t *testing.T) {
	expiresAt := time.Now().Add(-1 * time.Second)
	assert.True(t, IsExpired(expiresAt))
}

func TestIsExpired_ZeroTimestamp_Expired(t *testing.T) {
	assert.True(t, IsExpired(time.Time{}))
}

func TestComputeExpiry_NormalAction(t *testing.T) {
	now := time.Now()
	expiry := ComputeExpiry("Read", map[string]any{"file_path": "/tmp/foo"}, 300)
	assert.True(t, expiry.After(now.Add(299*time.Second)),
		"normal action should get full TTL")
	assert.True(t, expiry.Before(now.Add(301*time.Second)))
}

func TestComputeExpiry_DestructiveAction_ReducedTTL(t *testing.T) {
	now := time.Now()
	expiry := ComputeExpiry("Bash", map[string]any{"command": "rm -rf /tmp/build"}, 300)
	assert.True(t, expiry.Before(now.Add(31*time.Second)),
		"destructive action should get reduced TTL (30s)")
	assert.True(t, expiry.After(now.Add(29*time.Second)))
}

func TestIsDestructive_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		toolInput map[string]any
		want      bool
	}{
		{
			name:      "rm -rf",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "rm -rf /tmp/build"},
			want:      true,
		},
		{
			name:      "rm with force flag",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "rm -f important.txt"},
			want:      true,
		},
		{
			name:      "git push --force",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "git push --force origin main"},
			want:      true,
		},
		{
			name:      "git push force-with-lease",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "git push --force-with-lease"},
			want:      true,
		},
		{
			name:      "git reset --hard",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "git reset --hard HEAD~3"},
			want:      true,
		},
		{
			name:      "git clean -f",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "git clean -fd"},
			want:      true,
		},
		{
			name:      "drop table SQL",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "psql -c 'DROP TABLE users'"},
			want:      true,
		},
		{
			name:      "delete from SQL",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "mysql -e 'DELETE FROM logs'"},
			want:      true,
		},
		{
			name:      "shutdown",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "sudo shutdown -h now"},
			want:      true,
		},
		{
			name:      "reboot",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "sudo reboot"},
			want:      true,
		},
		{
			name:      "chmod 777",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "chmod 777 /etc/passwd"},
			want:      true,
		},
		{
			name:      "safe npm test",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "npm test"},
			want:      false,
		},
		{
			name:      "safe go build",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "go build ./..."},
			want:      false,
		},
		{
			name:      "safe git push without force",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "git push origin feature-branch"},
			want:      false,
		},
		{
			name:      "safe git status",
			toolName:  "Bash",
			toolInput: map[string]any{"command": "git status"},
			want:      false,
		},
		{
			name:      "Read tool is never destructive",
			toolName:  "Read",
			toolInput: map[string]any{"file_path": "/etc/passwd"},
			want:      false,
		},
		{
			name:      "Grep tool is never destructive",
			toolName:  "Grep",
			toolInput: map[string]any{"pattern": "password"},
			want:      false,
		},
		{
			name:      "Glob tool is never destructive",
			toolName:  "Glob",
			toolInput: map[string]any{"pattern": "**/*.go"},
			want:      false,
		},
		{
			name:      "Write tool is not destructive",
			toolName:  "Write",
			toolInput: map[string]any{"file_path": "/tmp/out.txt", "content": "hello"},
			want:      false,
		},
		{
			name:      "no command field",
			toolName:  "Bash",
			toolInput: map[string]any{},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDestructive(tt.toolName, tt.toolInput)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsDestructive_CaseInsensitive(t *testing.T) {
	assert.True(t, IsDestructive("Bash", map[string]any{
		"command": "RM -RF /tmp/build",
	}))
}
