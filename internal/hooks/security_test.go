package hooks

import (
	"encoding/hex"
	"testing"
)

func TestClassify_DestructivePatterns(t *testing.T) {
	cases := []struct {
		name      string
		toolName  string
		toolInput string
		want      ActionType
	}{
		// Required destructive patterns from the plan.
		{"rm -rf", "Bash", "rm -rf /tmp/foo", ActionDestructive},
		{"rm -fr", "Bash", "rm -fr /tmp/foo", ActionDestructive},
		{"git push --force", "Bash", "git push --force origin main", ActionDestructive},
		{"git push -f", "Bash", "git push -f origin main", ActionDestructive},
		{"git reset --hard", "Bash", "git reset --hard HEAD~1", ActionDestructive},
		{"git clean -f", "Bash", "git clean -f", ActionDestructive},
		{"DROP TABLE", "Postgres", "DROP TABLE users", ActionDestructive},
		{"DELETE FROM", "Postgres", "DELETE FROM users WHERE 1=1", ActionDestructive},
		{"TRUNCATE", "Postgres", "TRUNCATE accounts", ActionDestructive},
		{"reboot", "Bash", "sudo reboot", ActionDestructive},
		{"shutdown", "Bash", "shutdown -h now", ActionDestructive},
		{"chmod 777", "Bash", "chmod 777 /etc", ActionDestructive},
		{"chown -R", "Bash", "chown -R user /var", ActionDestructive},
		{"dd if=", "Bash", "dd if=/dev/zero of=/dev/sda", ActionDestructive},
		{"mkfs", "Bash", "mkfs.ext4 /dev/sda1", ActionDestructive},
		{"> /dev/", "Bash", "cat foo > /dev/sda", ActionDestructive},
		{":>!", "Bash", "echo :>! file", ActionDestructive},
		{"rm -- *", "Bash", "rm -- *", ActionDestructive},

		// Case-insensitive matching.
		{"case insensitive RM -RF", "Bash", "RM -RF /tmp", ActionDestructive},
		{"case insensitive drop table", "SQL", "drop table foo", ActionDestructive},
		{"mixed case Git Push --Force", "Bash", "Git Push --Force origin", ActionDestructive},

		// Pattern in tool name.
		{"pattern in tool name", "rm -rf script", "ls", ActionDestructive},

		// By-design false-positives — substring match is enough.
		{"rm -rf .DS_Store still destructive", "Bash", "rm -rf .DS_Store", ActionDestructive},

		// Normal operations.
		{"ls", "Bash", "ls -la", ActionNormal},
		{"git status", "Bash", "git status", ActionNormal},
		{"git push (no force)", "Bash", "git push origin main", ActionNormal},
		{"SELECT", "Postgres", "SELECT * FROM users", ActionNormal},
		{"empty", "", "", ActionNormal},
		{"read file", "Read", "/etc/hostname", ActionNormal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.toolName, tc.toolInput)
			if got != tc.want {
				t.Fatalf("Classify(%q, %q) = %v, want %v", tc.toolName, tc.toolInput, got, tc.want)
			}
		})
	}
}

func TestGenerateActionID_Format(t *testing.T) {
	id, err := GenerateActionID()
	if err != nil {
		t.Fatalf("GenerateActionID() returned error: %v", err)
	}

	// 16 bytes hex-encoded = 32 hex characters.
	if len(id) != 32 {
		t.Fatalf("expected 32-character hex string, got %d: %q", len(id), id)
	}

	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("expected valid hex string, got %q: %v", id, err)
	}
}

func TestGenerateActionID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1024)
	for i := 0; i < 1024; i++ {
		id, err := GenerateActionID()
		if err != nil {
			t.Fatalf("GenerateActionID() returned error on iter %d: %v", i, err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID generated at iter %d: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}
