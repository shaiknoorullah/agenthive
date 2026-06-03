package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
)

// newTestIdentity returns a freshly-generated Ed25519 keypair for use as the
// libp2p host identity in a single test. We deliberately use small but real
// keypairs rather than fixtures so the host's PeerID derivation is exercised
// end-to-end.
func newTestIdentity(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	return priv
}

// newTestConfig builds a daemon Config rooted at a TempDir, with loopback-only
// listen addrs (ephemeral ports) so two daemons in the same test process do not
// collide on a fixed port. We use a separate TempDir per daemon so they cannot
// see each other's state file or socket — they have to actually go through
// libp2p to converge.
func newTestConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		ConfigDir:   dir,
		Identity:    newTestIdentity(t),
		ListenAddrs: []string{"/ip4/127.0.0.1/tcp/0"},
		LogPath:     filepath.Join(dir, "log.jsonl"),
		SocketPath:  socketPath(t, "agenthive.sock"),
	}
}

// TestDaemon_StartStop boots a daemon, lets Run install every subsystem, then
// cancels the context and asserts Run returns promptly with no error. This
// exercises the full Run-loop construction and teardown path without any
// peers, which is the minimal contract from L4 (#1 in Tests).
func TestDaemon_StartStop(t *testing.T) {
	cfg := newTestConfig(t)

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	// Give Run a moment to install handlers and start its goroutines.
	if !waitForCondition(t, func() bool { return d.Host() != nil }, 3*time.Second) {
		t.Fatalf("daemon host never became available")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned err: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Run did not return after cancel")
	}
}

// TestDaemon_New_RejectsMissingIdentity asserts the constructor surfaces a
// usable error when the caller forgets to set Config.Identity, rather than
// panicking deep inside libp2p when Run is later called.
func TestDaemon_New_RejectsMissingIdentity(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Identity = nil
	_, err := New(cfg)
	if err == nil {
		t.Fatalf("expected error for nil identity, got nil")
	}
}

// TestDaemon_PersistsStateOnShutdown verifies the daemon writes its CRDT state
// to <configDir>/state.json on shutdown so a restart can pick up where it left
// off. The plan calls out persistence as step 10 of the Run workflow.
func TestDaemon_PersistsStateOnShutdown(t *testing.T) {
	cfg := newTestConfig(t)

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	if !waitForCondition(t, func() bool { return d.Host() != nil }, 3*time.Second) {
		t.Fatalf("daemon host never came up")
	}

	// Seed some state we can look for after shutdown.
	d.State().SetPeer("peer-a", crdt.PeerInfo{Name: "alpha", Status: "online"})

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}

	// Re-open the persisted state into a fresh store and assert the seeded
	// entry survived the round-trip.
	reload := crdt.NewStateStore("reader")
	if err := reload.LoadFromFile(filepath.Join(cfg.ConfigDir, "state.json")); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	got, ok := reload.GetPeer("peer-a")
	if !ok {
		t.Fatalf("expected peer-a in persisted state")
	}
	if got.Name != "alpha" || got.Status != "online" {
		t.Fatalf("persisted peer wrong: %+v", got)
	}
}

// TestDaemon_TwoPeersExchangeState boots two daemons on different ephemeral
// loopback ports, dials one from the other via the libp2p multiaddr exported
// by the daemon, mutates state on the first, and asserts the change converges
// on the second within 5s. This is the plan's hero convergence assertion.
func TestDaemon_TwoPeersExchangeState(t *testing.T) {
	cfg1 := newTestConfig(t)
	cfg2 := newTestConfig(t)

	d1, err := New(cfg1)
	if err != nil {
		t.Fatalf("New d1: %v", err)
	}
	d2, err := New(cfg2)
	if err != nil {
		t.Fatalf("New d2: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)
	go func() { errCh1 <- d1.Run(ctx) }()
	go func() { errCh2 <- d2.Run(ctx) }()

	if !waitForCondition(t, func() bool { return d1.Host() != nil && d2.Host() != nil }, 5*time.Second) {
		t.Fatalf("daemons never came up")
	}

	// Build an AddrInfo for d2 from one of its listen addrs (with the
	// /p2p/<peerid> suffix appended) and ask d1 to dial it. We deliberately
	// drive the dial through the daemon's Host so this exercises the same
	// connection path the production code uses.
	d2Addrs := d2.Multiaddrs()
	if len(d2Addrs) == 0 {
		t.Fatalf("d2 has no multiaddrs")
	}
	var info *peer.AddrInfo
	for _, s := range d2Addrs {
		mAddr, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(mAddr)
		if err != nil {
			continue
		}
		// Loopback only — skip any IPv6 link-local address that snuck in.
		if !strings.Contains(s, "127.0.0.1") {
			continue
		}
		info = pi
		break
	}
	if info == nil {
		t.Fatalf("could not parse a loopback multiaddr from d2: %v", d2Addrs)
	}

	connectCtx, cancelConnect := context.WithTimeout(ctx, 3*time.Second)
	defer cancelConnect()
	if err := d1.Host().Connect(connectCtx, *info); err != nil {
		t.Fatalf("d1 connect to d2: %v", err)
	}

	// Give pubsub a moment to wire up the mesh between the two newly-connected
	// peers. GossipSub needs a heartbeat or two before publishes propagate
	// reliably.
	time.Sleep(500 * time.Millisecond)

	// Mutate state on d1.
	d1.State().SetPeer("convergence-key", crdt.PeerInfo{
		Name:   "from-d1",
		Status: "online",
	})

	// Assert d2 converges within 5s. The daemon's publish goroutine is
	// debounced at 200ms, so we should see this within ~1-2 seconds in
	// practice.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		info, ok := d2.State().GetPeer("convergence-key")
		if ok && info.Name == "from-d1" {
			cancel()
			<-errCh1
			<-errCh2
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("d2 did not converge on d1's state within 5s")
}

// waitForCondition polls fn every 25ms until it returns true or the deadline
// elapses. Returns true if fn ever returned true within the budget.
func waitForCondition(t *testing.T, fn func() bool, budget time.Duration) bool {
	t.Helper()
	end := time.Now().Add(budget)
	for time.Now().Before(end) {
		if fn() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}
