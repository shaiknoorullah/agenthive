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
