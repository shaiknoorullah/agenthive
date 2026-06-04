// Package tmux houses the TPM-compatible agenthive tmux plugin and helper
// scripts. The Go test file here does not exercise tmux at runtime — that
// requires a live tmux server and is skipped in CI. Instead, it gives the
// Go test runner a way to run cheap, deterministic syntax/static checks on
// the shell scripts so the package never regresses without CI catching it.
package tmux

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scriptFiles is the canonical list of shell entry points the plugin
// installs. They must all be syntactically valid bash and executable.
var scriptFiles = []string{
	"agenthive.tmux",
	"scripts/notification-clear.sh",
}

// TestShellScriptsSyntax verifies every shell script in this package parses
// cleanly under `bash -n`. This guards against typos that would otherwise
// only surface at tmux load time.
func TestShellScriptsSyntax(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not in PATH; skipping syntax check")
	}
	for _, rel := range scriptFiles {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			path := absPath(t, rel)
			cmd := exec.Command("bash", "-n", path)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("bash -n %s failed: %v\nstderr: %s", rel, err, stderr.String())
			}
		})
	}
}

// TestShellScriptsExecutable verifies the entry scripts are marked
// executable so TPM (and `run-shell`) can invoke them directly without
// needing an explicit `bash` prefix from the user's tmux config.
func TestShellScriptsExecutable(t *testing.T) {
	for _, rel := range scriptFiles {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			path := absPath(t, rel)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat %s: %v", rel, err)
			}
			if info.Mode()&0o111 == 0 {
				t.Fatalf("%s is not executable (mode=%v)", rel, info.Mode())
			}
		})
	}
}

// TestPluginInstallsExpectedHooks checks that agenthive.tmux wires up the
// pieces the v0.1.0 plan specifies: a status-right append referencing
// @notif-msg, a pane-focus-in hook that runs the clear script, and the
// @agenthive-installed sentinel that makes re-sourcing idempotent.
func TestPluginInstallsExpectedHooks(t *testing.T) {
	body := readFile(t, "agenthive.tmux")

	checks := []struct {
		name  string
		needs []string
	}{
		{
			name:  "renders @notif-msg in status-right",
			needs: []string{"@notif-msg", "status-right"},
		},
		{
			name:  "installs pane-focus-in hook",
			needs: []string{"set-hook", "pane-focus-in"},
		},
		{
			name:  "invokes the notification-clear helper",
			needs: []string{"notification-clear.sh"},
		},
		{
			name:  "defines the @agenthive-installed sentinel",
			needs: []string{"@agenthive-installed"},
		},
	}

	for _, c := range checks {
		c := c
		t.Run(c.name, func(t *testing.T) {
			for _, needle := range c.needs {
				if !strings.Contains(body, needle) {
					t.Errorf("agenthive.tmux missing %q", needle)
				}
			}
		})
	}
}

// TestPluginIsIdempotent asserts the install path checks the sentinel
// before appending so re-sourcing the plugin does not duplicate the
// status-right segment.
func TestPluginIsIdempotent(t *testing.T) {
	body := readFile(t, "agenthive.tmux")
	if !strings.Contains(body, "@agenthive-installed") {
		t.Fatal("agenthive.tmux is missing the @agenthive-installed sentinel")
	}

	// Strip comment lines before scanning so commentary about the
	// sentinel does not satisfy the check on its own — we want actual
	// executable code that reads the sentinel.
	var codeLines []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		codeLines = append(codeLines, line)
	}
	code := strings.Join(codeLines, "\n")

	if !strings.Contains(code, "@agenthive-installed") {
		t.Fatal("agenthive.tmux only mentions @agenthive-installed in comments — no live guard")
	}

	// Heuristic: the executable portion must both read the sentinel and
	// contain a conditional branch (`if`, `test`, or `[ ... ]`) so we
	// know re-sourcing is actually gated.
	hasConditional := strings.Contains(code, "if ") ||
		strings.Contains(code, "test ") ||
		strings.Contains(code, "[ ")
	if !hasConditional {
		t.Errorf("agenthive.tmux has no conditional gating the sentinel:\n%s", code)
	}
}

// TestClearScriptUnsetsAllNotifOptions verifies the helper script unsets
// every @notif-* option the daemon writes, so a single pane-focus-in
// event clears the entire notification rather than leaking state.
func TestClearScriptUnsetsAllNotifOptions(t *testing.T) {
	body := readFile(t, "scripts/notification-clear.sh")
	required := []string{
		"@notif-msg",
		"@notif-project",
		"@notif-source",
		"@notif-time",
		"@notif-priority",
	}
	for _, opt := range required {
		if !strings.Contains(body, opt) {
			t.Errorf("notification-clear.sh missing unset for %s", opt)
		}
	}
	// Each option should be unset via `set -gu` (the global-unset form).
	if !strings.Contains(body, "set -gu") {
		t.Error("notification-clear.sh does not call `tmux set -gu`")
	}
}

// TestREADMEDocumentsInstall checks that the README covers both TPM and
// manual-source install paths so users can drop the plugin into their
// existing tmux setup without spelunking the source.
func TestREADMEDocumentsInstall(t *testing.T) {
	body := readFile(t, "README.md")
	needles := []string{
		"@plugin",                // TPM install snippet
		"run-shell",              // manual source snippet
		"agenthive.tmux",         // file the user sources
		"@notif-",                // mention the option namespace
		"prefix + I",             // TPM activation hint
	}
	for _, n := range needles {
		if !strings.Contains(body, n) {
			t.Errorf("README.md missing %q", n)
		}
	}
}

// absPath resolves a path relative to this test file's directory. Tests
// run with the package dir as cwd, but using the resolved absolute form
// keeps failures legible.
func absPath(t *testing.T, rel string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, rel)
}

func readFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(absPath(t, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}
