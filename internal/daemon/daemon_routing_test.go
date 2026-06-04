package daemon

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// addrInfoForLoopback picks the loopback /p2p/<peerid> multiaddr from a
// daemon's listen addrs. Tests use this to dial peers via the host's Connect.
func addrInfoForLoopback(t *testing.T, d *Daemon) *peer.AddrInfo {
	t.Helper()
	for _, s := range d.Multiaddrs() {
		mAddr, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		if !strings.Contains(s, "127.0.0.1") {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(mAddr)
		if err != nil {
			continue
		}
		return pi
	}
	t.Fatalf("no loopback multiaddr for daemon")
	return nil
}

// connectDaemons asks src to dial dst through the libp2p host so subsequent
// stream opens route over the established connection.
func connectDaemons(t *testing.T, ctx context.Context, src, dst *Daemon) {
	t.Helper()
	info := addrInfoForLoopback(t, dst)
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := src.Host().Connect(dialCtx, *info); err != nil {
		t.Fatalf("dial: %v", err)
	}
}

// notifWatcher hooks the local dispatcher of a daemon by replacing its
// dispatcher with a recording wrapper. Returns a channel of received
// notification messages and a teardown function.
//
// We intercept by adding a recording Surface to the existing dispatcher,
// which receives every Dispatch call (inbound or local-origin) and forwards
// the message via the returned channel. The dispatcher's existing surfaces
// are left intact so the daemon's other behaviour (log surface) is unchanged.
type recordingSurface struct {
	mu       sync.Mutex
	received []protocols.Notification
}

func (r *recordingSurface) Name() string { return "recording" }

func (r *recordingSurface) Dispatch(_ context.Context, n protocols.Notification) error {
	r.mu.Lock()
	r.received = append(r.received, n)
	r.mu.Unlock()
	return nil
}

func (r *recordingSurface) DispatchAction(_ context.Context, _ protocols.ActionRequest) error {
	return nil
}

func (r *recordingSurface) Close() error { return nil }

func (r *recordingSurface) snapshot() []protocols.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]protocols.Notification, len(r.received))
	copy(out, r.received)
	return out
}

// TestDaemon_DispatchNotification_RoutesOnlyToMatchedTargets boots three
// daemons in an in-process libp2p mesh, defines a route on d1 whose targets
// pin only d2, then asks d1 to DispatchNotification and asserts d2 receives
// the message via its libp2p notification stream handler while d3 does not.
//
// This exercises L5.B step 4: "ask the matcher for target peers first; route
// the libp2p stream only to those peers".
func TestDaemon_DispatchNotification_RoutesOnlyToMatchedTargets(t *testing.T) {
	d1, _ := mustStartDaemon(t)
	d2, rec2 := mustStartDaemon(t)
	d3, rec3 := mustStartDaemon(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connectDaemons(t, ctx, d1, d2)
	connectDaemons(t, ctx, d1, d3)

	// Seed d1's route table so a notification with priority "critical" routes
	// only to d2's peer ID.
	d1.State().SetPeer(d2.Host().ID().String(), crdt.PeerInfo{Name: "d2"})
	d1.State().SetPeer(d3.Host().ID().String(), crdt.PeerInfo{Name: "d3"})
	d1.State().SetRoute("only-d2", crdt.RouteRule{
		Match:   crdt.RouteMatch{Priority: "critical"},
		Targets: []string{d2.Host().ID().String()},
	})

	notif := protocols.Notification{
		SessionID: "sess",
		Source:    "claude-code",
		Project:   "demo",
		Priority:  "critical",
		Message:   "matched",
		Timestamp: time.Now().UTC(),
	}
	if err := d1.DispatchNotification(ctx, notif); err != nil {
		t.Fatalf("DispatchNotification: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	gotByD2 := false
	for time.Now().Before(deadline) {
		for _, n := range rec2.snapshot() {
			if n.Message == "matched" {
				gotByD2 = true
				break
			}
		}
		if gotByD2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !gotByD2 {
		t.Fatalf("d2 never received the routed notification")
	}

	// d3 must not have received it — but the notification stream is async so
	// give it a brief grace window before asserting.
	time.Sleep(200 * time.Millisecond)
	for _, n := range rec3.snapshot() {
		if n.Message == "matched" {
			t.Fatalf("d3 received a notification that was not addressed to it: %+v", n)
		}
	}
}

// TestDaemon_DispatchNotification_AlsoFiresLocalDispatcher confirms the
// outbound dispatch path also routes the notification through the originator's
// local dispatcher — the plan says "always also call dispatcher.Dispatch".
func TestDaemon_DispatchNotification_AlsoFiresLocalDispatcher(t *testing.T) {
	d1, rec1 := mustStartDaemon(t)
	d2, _ := mustStartDaemon(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connectDaemons(t, ctx, d1, d2)

	d1.State().SetPeer(d2.Host().ID().String(), crdt.PeerInfo{Name: "d2"})
	d1.State().SetRoute("any", crdt.RouteRule{
		Match:   crdt.RouteMatch{},
		Targets: []string{d2.Host().ID().String()},
	})

	notif := protocols.Notification{
		SessionID: "sess",
		Source:    "claude-code",
		Message:   "local-too",
		Timestamp: time.Now().UTC(),
	}
	if err := d1.DispatchNotification(ctx, notif); err != nil {
		t.Fatalf("DispatchNotification: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range rec1.snapshot() {
			if n.Message == "local-too" {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("local dispatcher never saw the originator's own notification")
}

// mustStartDaemon boots a daemon and attaches a recordingSurface to its
// dispatcher so tests can observe inbound notifications. Returns the daemon
// and the surface. The daemon is registered to be stopped when the test
// finishes.
func mustStartDaemon(t *testing.T) (*Daemon, *recordingSurface) {
	t.Helper()
	cfg := newTestConfig(t)
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	if !waitForCondition(t, func() bool { return d.Host() != nil }, 5*time.Second) {
		cancel()
		<-errCh
		t.Fatalf("daemon never came up")
	}

	rec := &recordingSurface{}
	d.dispatcher.Add(rec)

	t.Cleanup(func() {
		cancel()
		<-errCh
	})
	return d, rec
}
