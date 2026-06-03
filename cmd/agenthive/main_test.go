package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shaiknoorullah/agenthive/internal/daemon"
	"github.com/shaiknoorullah/agenthive/internal/identity"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// runRoot invokes the cobra root command with the given args, capturing
// stdout/stderr into buffers and returning them along with any error. We
// rebuild the root for every invocation so flag state cannot leak between
// tests, and so the global --config-dir parsing is exercised end-to-end on
// every call.
func runRoot(t *testing.T, args []string, stdin string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

// TestRoot_HasAllSubcommands verifies every subcommand the plan requires is
// wired into the root command, with the global --config-dir flag present.
func TestRoot_HasAllSubcommands(t *testing.T) {
	root := newRootCmd()

	if root.PersistentFlags().Lookup("config-dir") == nil {
		t.Fatalf("--config-dir global flag missing")
	}

	want := []string{"init", "id", "peers", "start", "hook", "respond"}
	have := map[string]bool{}
	for _, c := range root.Commands() {
		have[c.Name()] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Fatalf("subcommand %q missing; have %v", name, have)
		}
	}
}

// TestInit_WritesIdentity asserts `agenthive init` creates the config dir and
// writes a usable identity.key.
func TestInit_WritesIdentity(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, "")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	priv, err := identity.Load(dir)
	if err != nil {
		t.Fatalf("identity.Load after init: %v", err)
	}
	if priv == nil {
		t.Fatalf("init wrote nil identity")
	}
}

// TestInit_RefusesToOverwrite ensures init does not silently destroy an
// existing identity. The plan calls init a one-shot bootstrap; a second init
// must be an explicit no-op or error rather than rotating keys.
func TestInit_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init #1: %v", err)
	}
	priv1, err := identity.Load(dir)
	if err != nil {
		t.Fatalf("load #1: %v", err)
	}

	// Second init: must NOT replace the key. Either error or no-op is fine,
	// but the key on disk must be unchanged.
	_, _, _ = runRoot(t, []string{"--config-dir", dir, "init"}, "")
	priv2, err := identity.Load(dir)
	if err != nil {
		t.Fatalf("load #2: %v", err)
	}
	if !priv1.Equals(priv2) {
		t.Fatalf("init overwrote existing identity")
	}
}

// TestID_PrintsMultiaddrsContainingPeerID asserts `agenthive id` prints at
// least one /p2p/<peerid> line and that PeerID matches what the identity key
// would derive to.
func TestID_PrintsMultiaddrsContainingPeerID(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}

	priv, err := identity.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}

	stdout, _, err := runRoot(t, []string{"--config-dir", dir, "id"}, "")
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	if !strings.Contains(stdout, pid.String()) {
		t.Fatalf("id output missing peer id %s; got:\n%s", pid, stdout)
	}
	if !strings.Contains(stdout, "/p2p/") {
		t.Fatalf("id output missing /p2p/ segment; got:\n%s", stdout)
	}
}

// TestID_ErrorsWhenNoIdentity asserts that asking for the ID before init has
// run surfaces a useful error rather than panicking.
func TestID_ErrorsWhenNoIdentity(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRoot(t, []string{"--config-dir", dir, "id"}, "")
	if err == nil {
		t.Fatalf("id: expected error when identity missing, got nil")
	}
}

// TestPeers_AddThenList exercises the add/list round-trip. Adding a multiaddr
// must persist it to state.json, and listing must print it back out.
func TestPeers_AddThenList(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}

	// A real multiaddr with a /p2p/ component — the add command uses this to
	// extract the peer ID for the StateStore key.
	const ma = "/ip4/127.0.0.1/tcp/4001/p2p/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "peers", "add", ma}, ""); err != nil {
		t.Fatalf("peers add: %v", err)
	}

	stdout, _, err := runRoot(t, []string{"--config-dir", dir, "peers", "list"}, "")
	if err != nil {
		t.Fatalf("peers list: %v", err)
	}
	if !strings.Contains(stdout, "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG") {
		t.Fatalf("peers list missing added peer; got:\n%s", stdout)
	}
}

// TestPeers_AddRejectsInvalidMultiaddr ensures the parser surfaces an error
// rather than writing garbage into state.json.
func TestPeers_AddRejectsInvalidMultiaddr(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, _, err := runRoot(t, []string{"--config-dir", dir, "peers", "add", "not-a-multiaddr"}, "")
	if err == nil {
		t.Fatalf("expected error for invalid multiaddr, got nil")
	}
}

// TestRespond_WritesResponseFile asserts the manual-override command drops a
// well-formed response file the gate can pick up.
func TestRespond_WritesResponseFile(t *testing.T) {
	dir := t.TempDir()
	// init so the queue dir layout exists; respond creates the queue if it
	// does not exist yet, but exercising the realistic path is more useful.
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}

	_, _, err := runRoot(t, []string{"--config-dir", dir, "respond", "act-xyz", "allow"}, "")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}

	respPath := filepath.Join(dir, "queue", "act-xyz.response")
	data, err := os.ReadFile(respPath)
	if err != nil {
		t.Fatalf("read response file: %v", err)
	}
	var resp protocols.ActionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ActionID != "act-xyz" || resp.Decision != "allow" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

// TestRespond_RejectsBadDecision asserts the verb (allow|deny) is validated.
func TestRespond_RejectsBadDecision(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runRoot(t, []string{"--config-dir", dir, "respond", "act-1", "maybe"}, "")
	if err == nil {
		t.Fatalf("expected error for invalid decision")
	}
}

// TestHook_FallsBackWhenDaemonAbsent verifies the critical fail-open
// behaviour: if the daemon is not running, hook PreToolUse exits 0 with no
// output (Claude falls back to its built-in prompt).
func TestHook_FallsBackWhenDaemonAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	dir := t.TempDir()
	payload := `{"hook_event":"PreToolUse","tool":"Bash","input":"ls","session_id":"s1","tool_use_id":"t1"}`

	stdout, _, err := runRoot(t, []string{"--config-dir", dir, "hook", "PreToolUse"}, payload)
	if err != nil {
		t.Fatalf("hook with absent daemon must exit cleanly: %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("hook with absent daemon must produce no output; got: %q", stdout)
	}
}

// TestHook_FallsBackOnInvalidStdin guards against accidentally fail-closing
// when the caller pipes garbage to stdin. The contract says: print nothing,
// exit 0.
func TestHook_FallsBackOnInvalidStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}
	dir := t.TempDir()
	stdout, _, err := runRoot(t, []string{"--config-dir", dir, "hook", "PreToolUse"}, "not json")
	if err != nil {
		t.Fatalf("hook with bad stdin must exit cleanly: %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("hook with bad stdin must produce no output; got: %q", stdout)
	}
}

// TestHook_RoundTripAgainstRunningDaemon stands up a real daemon (which boots
// the SocketServer and the gate), runs the hook subcommand pointing at the
// same config-dir, and asserts the hook prints a JSON document containing
// {"permissionDecision":"allow"} once a response is written into the queue.
func TestHook_RoundTripAgainstRunningDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets not supported")
	}

	// We need a config dir whose socket path is short enough to fit in the
	// kernel's sun_path limit (~104 chars on macOS). t.TempDir often returns
	// paths near that limit; create our own short tempdir and use it as
	// both the daemon's config dir AND the hook subcommand's --config-dir
	// so both sides agree on the socket location.
	dir, err := os.MkdirTemp("", "ah-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "agenthive.sock")

	priv := newTestIdentity(t)
	if err := identity.Save(dir, priv); err != nil {
		t.Fatalf("save identity: %v", err)
	}

	// Boot the daemon in-process so the hook subcommand has a real socket
	// to dial. We use loopback-only listen addrs so two daemons in the same
	// test process cannot collide on a fixed port.
	cfg := daemon.Config{
		ConfigDir:   dir,
		Identity:    priv,
		ListenAddrs: []string{"/ip4/127.0.0.1/tcp/0"},
		SocketPath:  socket,
	}
	d, err := daemon.New(cfg)
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- d.Run(ctx) }()

	// Wait until the socket file appears so the hook subcommand's dial does
	// not race the bind.
	if !waitForCondition(func() bool {
		info, err := os.Stat(socket)
		return err == nil && info.Mode()&os.ModeSocket != 0
	}, 5*time.Second) {
		t.Fatalf("socket %s never appeared", socket)
	}

	// Drop the response into the queue as soon as the gate writes the
	// pending file. The hook subcommand will block on Gate.Handle until we
	// do, then return the decision.
	queueDir := filepath.Join(dir, "queue")
	var wg sync.WaitGroup
	wg.Add(1)
	var seenPending atomic.Bool
	go func() {
		defer wg.Done()
		end := time.Now().Add(20 * time.Second)
		for time.Now().Before(end) {
			entries, err := os.ReadDir(queueDir)
			if err == nil {
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".pending") {
						seenPending.Store(true)
						actionID := strings.TrimSuffix(e.Name(), ".pending")
						resp := protocols.ActionResponse{
							ActionID:  actionID,
							Decision:  "allow",
							DecidedBy: "test",
						}
						body, _ := json.Marshal(resp)
						_ = os.WriteFile(filepath.Join(queueDir, actionID+".response"), body, 0o600)
						return
					}
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Confirm the daemon hasn't already exited with an error.
	select {
	case err := <-runErr:
		t.Fatalf("daemon.Run exited before hook ran: %v", err)
	default:
	}

	payload := `{"hook_event":"PreToolUse","tool":"Bash","input":"ls","session_id":"s1","tool_use_id":"t1"}`
	stdout, stderr, hookErr := runRoot(t,
		[]string{"--config-dir", dir, "hook", "PreToolUse"},
		payload,
	)
	if hookErr != nil {
		t.Fatalf("hook: %v (stderr=%s)", hookErr, stderr)
	}
	wg.Wait()

	if !strings.Contains(stdout, `"permissionDecision":"allow"`) {
		entries, _ := os.ReadDir(queueDir)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("hook output missing allow decision; got: %q; saw pending=%v; queue=%v", stdout, seenPending.Load(), names)
	}
	if !strings.Contains(stdout, `"hookSpecificOutput"`) {
		t.Fatalf("hook output missing hookSpecificOutput envelope; got: %q", stdout)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("daemon.Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("daemon.Run did not return after cancel")
	}
}

// newTestIdentity returns a fresh Ed25519 keypair for tests. We reach for
// crypto.GenerateEd25519Key directly so we don't depend on identity.Generate
// being implemented correctly to test the rest of the CLI.
func newTestIdentity(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	return priv
}

// waitForCondition polls fn every 10ms until it returns true or deadline
// elapses. Returns true if the condition was met.
func waitForCondition(fn func() bool, deadline time.Duration) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if fn() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fn()
}

