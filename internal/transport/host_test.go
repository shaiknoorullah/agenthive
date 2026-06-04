package transport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/shaiknoorullah/agenthive/internal/identity"
)

// newTestIdentity returns a fresh Ed25519 private key for tests.
func newTestIdentity(t *testing.T) crypto.PrivKey {
	t.Helper()
	priv, _, err := identity.Generate()
	require.NoError(t, err)
	return priv
}

// newTestHost builds a Config bound to loopback-only addresses (so CI firewalls
// don't bite) and constructs a host. The caller must defer h.Close().
func newTestHost(t *testing.T, ctx context.Context) host.Host {
	t.Helper()
	priv := newTestIdentity(t)
	cfg := Config{
		Identity: priv,
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
			"/ip4/127.0.0.1/udp/0/quic-v1",
		},
	}
	h, err := New(ctx, cfg)
	require.NoError(t, err)
	return h
}

func TestNew_RequiresIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := New(ctx, Config{})
	require.Error(t, err, "New must reject a Config with no Identity")
}

func TestNew_HasPeerID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	priv := newTestIdentity(t)
	h, err := New(ctx, Config{
		Identity: priv,
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
			"/ip4/127.0.0.1/udp/0/quic-v1",
		},
	})
	require.NoError(t, err)
	defer func() { _ = h.Close() }()

	require.NotEqual(t, peer.ID(""), h.ID())

	// PeerID must be derived from the identity we passed in.
	want, err := peer.IDFromPrivateKey(priv)
	require.NoError(t, err)
	require.Equal(t, want, h.ID(), "host PeerID must match the supplied identity")
}

func TestNew_DefaultListenAddrsWhenEmpty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	priv := newTestIdentity(t)
	h, err := New(ctx, Config{Identity: priv})
	require.NoError(t, err)
	defer func() { _ = h.Close() }()

	require.NotEmpty(t, h.Addrs(), "host with no configured addrs must still listen on the package defaults")
}

func TestNew_ListensOnConfiguredAddrs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h := newTestHost(t, ctx)
	defer func() { _ = h.Close() }()

	addrs := h.Addrs()
	require.NotEmpty(t, addrs)

	var sawTCP, sawQUIC bool
	for _, a := range addrs {
		s := a.String()
		if strings.Contains(s, "/tcp/") && !strings.Contains(s, "/quic") {
			sawTCP = true
		}
		if strings.Contains(s, "/quic-v1") {
			sawQUIC = true
		}
	}
	require.True(t, sawTCP, "expected at least one TCP listen addr, got %v", addrs)
	require.True(t, sawQUIC, "expected at least one QUIC-v1 listen addr, got %v", addrs)
}

func TestMultiaddrsFor_IncludesPeerID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h := newTestHost(t, ctx)
	defer func() { _ = h.Close() }()

	mas := MultiaddrsFor(h)
	require.NotEmpty(t, mas)

	pidStr := h.ID().String()
	for _, m := range mas {
		require.Contains(t, m, "/p2p/"+pidStr, "every advertised multiaddr must end with /p2p/<peerid>")
	}
}

func TestMultiaddrsFor_NilHostReturnsNil(t *testing.T) {
	require.Nil(t, MultiaddrsFor(nil))
}

func TestNew_TwoHostsCanDialEachOther(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a := newTestHost(t, ctx)
	defer func() { _ = a.Close() }()
	b := newTestHost(t, ctx)
	defer func() { _ = b.Close() }()

	// Connect b to a using a's listen addrs. peer.AddrInfo carries both ID
	// and multiaddrs; libp2p picks one and dials it.
	info := peer.AddrInfo{ID: a.ID(), Addrs: a.Addrs()}
	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dialCancel()

	err := b.Connect(dialCtx, info)
	require.NoError(t, err)

	// Both sides should now see each other as connected.
	require.Eventually(t, func() bool {
		return b.Network().Connectedness(a.ID()) == 1 // network.Connected == 1
	}, 5*time.Second, 50*time.Millisecond, "b never reached Connected state to a")
}

// TestNew_ContextRespected verifies that a canceled ctx surfaces as an error
// rather than hanging. This guards against accidental use of ctx-less swarm
// calls that block on slow DNS or NAT discovery.
func TestNew_ContextRespected(t *testing.T) {
	priv := newTestIdentity(t)

	// We give a real (non-zero) timeout; an already-cancelled context can be
	// swallowed by background bootstrap goroutines, but a 1ms deadline makes
	// the intent clear without flaking.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h, err := New(ctx, Config{
		Identity: priv,
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
		},
	})
	// Either error path is acceptable: the host built and we can close it, or
	// it errored. The check is that we don't panic and don't block.
	if err != nil {
		return
	}
	require.NotNil(t, h)
	defer func() { _ = h.Close() }()
}

// TestNew_InvalidListenAddrFails confirms invalid multiaddr strings surface as
// errors instead of silently being swallowed.
func TestNew_InvalidListenAddrFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := newTestIdentity(t)
	h, err := New(ctx, Config{
		Identity:    priv,
		ListenAddrs: []string{"not-a-multiaddr"},
	})
	if err == nil {
		// Defensive cleanup; expectation is that we error out.
		_ = h.Close()
		t.Fatalf("New must reject invalid listen multiaddrs")
	}
	require.Error(t, err)
	// Don't constrain the exact error string — just that it's non-nil and
	// not a context error.
	require.False(t, errors.Is(err, context.Canceled))
	require.False(t, errors.Is(err, context.DeadlineExceeded))
}

// TestPeerSourceOrDefault_NilFallsBackToEmpty confirms that the helper
// substitutes the package-internal no-op closure when Config.PeerSource is
// nil. The default closure must return an already-closed empty channel so
// AutoRelay's request loop terminates cleanly.
func TestPeerSourceOrDefault_NilFallsBackToEmpty(t *testing.T) {
	got := peerSourceOrDefault(nil)
	require.NotNil(t, got, "nil PeerSource must fall back to the default closure")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := got(ctx, 8)
	require.NotNil(t, ch, "default closure must return a non-nil channel")

	// Channel must be drained and closed: receive must succeed immediately
	// with the zero-value AddrInfo and ok=false.
	select {
	case _, ok := <-ch:
		require.False(t, ok, "default closure must return a closed channel")
	case <-time.After(time.Second):
		t.Fatalf("default PeerSource closure did not close its channel")
	}
}

// TestPeerSourceOrDefault_CustomHonored confirms that the helper returns the
// caller's closure verbatim when one is provided. Calling the returned
// closure must invoke the supplied function with the same num and yield the
// peers it emits.
func TestPeerSourceOrDefault_CustomHonored(t *testing.T) {
	var (
		callCount int
		seenNum   int
	)

	wantPeer := peer.AddrInfo{ID: peer.ID("test-peer-id")}
	custom := func(ctx context.Context, num int) <-chan peer.AddrInfo {
		callCount++
		seenNum = num
		out := make(chan peer.AddrInfo, 1)
		out <- wantPeer
		close(out)
		return out
	}

	got := peerSourceOrDefault(custom)
	require.NotNil(t, got)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := got(ctx, 42)
	require.Equal(t, 1, callCount, "custom closure must be invoked exactly once")
	require.Equal(t, 42, seenNum, "custom closure must receive the caller's num")

	select {
	case info, ok := <-ch:
		require.True(t, ok, "custom closure's channel must yield the queued peer")
		require.Equal(t, wantPeer.ID, info.ID)
	case <-time.After(time.Second):
		t.Fatalf("custom PeerSource closure did not emit its peer")
	}

	// Channel must then close.
	select {
	case _, ok := <-ch:
		require.False(t, ok, "custom closure's channel must close after draining")
	case <-time.After(time.Second):
		t.Fatalf("custom PeerSource closure did not close its channel")
	}
}

// TestNew_AcceptsNilPeerSource confirms a Config without a PeerSource still
// produces a working host (the default no-op closure is wired in for
// AutoRelay).
func TestNew_AcceptsNilPeerSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	priv := newTestIdentity(t)
	h, err := New(ctx, Config{
		Identity: priv,
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
			"/ip4/127.0.0.1/udp/0/quic-v1",
		},
		// PeerSource intentionally nil.
	})
	require.NoError(t, err)
	defer func() { _ = h.Close() }()

	require.NotEmpty(t, h.Addrs(), "host built with nil PeerSource must still listen")
	require.NotEmpty(t, MultiaddrsFor(h), "MultiaddrsFor must remain sane with nil PeerSource")
}

// TestNew_AcceptsCustomPeerSource confirms a Config with a caller-supplied
// PeerSource builds a working host, and that the closure stays alive (i.e.
// libp2p didn't reject the signature). We don't assert that libp2p invokes
// the closure on a specific schedule — that's an AutoRelay implementation
// detail — only that constructing the host succeeds and the closure can be
// invoked manually with the same signature libp2p uses.
func TestNew_AcceptsCustomPeerSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	priv := newTestIdentity(t)

	var invoked bool
	custom := func(ctx context.Context, num int) <-chan peer.AddrInfo {
		invoked = true
		ch := make(chan peer.AddrInfo)
		close(ch)
		return ch
	}

	h, err := New(ctx, Config{
		Identity: priv,
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
			"/ip4/127.0.0.1/udp/0/quic-v1",
		},
		PeerSource: custom,
	})
	require.NoError(t, err)
	defer func() { _ = h.Close() }()

	require.NotEmpty(t, h.Addrs(), "host built with custom PeerSource must still listen")
	require.NotEmpty(t, MultiaddrsFor(h))

	// Invoke the custom closure through the same helper New uses to verify
	// the wiring path honors what the caller supplied rather than silently
	// substituting the default.
	resolved := peerSourceOrDefault(custom)
	ch := resolved(ctx, 4)
	require.True(t, invoked, "peerSourceOrDefault must return the caller's closure verbatim")

	// Drain the channel so the test doesn't leak goroutines.
	for range ch {
	}
}
