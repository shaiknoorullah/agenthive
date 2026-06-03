package discovery

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// newTestHost creates an ephemeral libp2p host for a single test, and registers
// a cleanup that closes it.
func newTestHost(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// TestStartMDNS_DoesNotError verifies that StartMDNS can be invoked on a
// freshly-created host with a no-op callback and returns no error and a
// non-nil stop function.
func TestStartMDNS_DoesNotError(t *testing.T) {
	h := newTestHost(t)

	stop, err := StartMDNS(h, func(peer.AddrInfo) {})
	require.NoError(t, err)
	require.NotNil(t, stop)
	t.Cleanup(func() { _ = stop() })
}

// TestStartMDNS_StopFuncWorks verifies that the returned stop function can be
// called without error and that calling it terminates the underlying mDNS
// service cleanly.
func TestStartMDNS_StopFuncWorks(t *testing.T) {
	h := newTestHost(t)

	stop, err := StartMDNS(h, func(peer.AddrInfo) {})
	require.NoError(t, err)
	require.NotNil(t, stop)

	require.NoError(t, stop())
}

// TestStartMDNS_NilCallbackRejected verifies that StartMDNS rejects a nil
// callback rather than installing a panicking notifee.
func TestStartMDNS_NilCallbackRejected(t *testing.T) {
	h := newTestHost(t)

	stop, err := StartMDNS(h, nil)
	require.Error(t, err)
	require.Nil(t, stop)
}

// TestStartMDNS_TwoHostsDiscover wires two libp2p hosts together via mDNS on
// the loopback interface and asserts the callback fires for the peer. This is
// flaky on CI runners that do not allow multicast, so it is skipped under CI.
func TestStartMDNS_TwoHostsDiscover(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("mDNS multicast is unreliable on CI runners")
	}

	h1 := newTestHost(t)
	h2 := newTestHost(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var (
		mu    sync.Mutex
		seen1 = map[peer.ID]struct{}{}
		seen2 = map[peer.ID]struct{}{}
	)

	found1 := make(chan struct{}, 1)
	found2 := make(chan struct{}, 1)

	cb1 := func(pi peer.AddrInfo) {
		mu.Lock()
		defer mu.Unlock()
		if pi.ID == h2.ID() {
			if _, ok := seen1[pi.ID]; !ok {
				seen1[pi.ID] = struct{}{}
				select {
				case found1 <- struct{}{}:
				default:
				}
			}
		}
	}
	cb2 := func(pi peer.AddrInfo) {
		mu.Lock()
		defer mu.Unlock()
		if pi.ID == h1.ID() {
			if _, ok := seen2[pi.ID]; !ok {
				seen2[pi.ID] = struct{}{}
				select {
				case found2 <- struct{}{}:
				default:
				}
			}
		}
	}

	stop1, err := StartMDNS(h1, cb1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stop1() })

	stop2, err := StartMDNS(h2, cb2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stop2() })

	// We don't strictly require both directions to fire — the platform may
	// drop one — but at least one host must see the other within the budget.
	select {
	case <-found1:
	case <-found2:
	case <-ctx.Done():
		t.Fatalf("neither host discovered the other via mDNS within %s", "10s")
	}
}

// TestServiceTag pins the public constant so accidental rename is caught.
func TestServiceTag(t *testing.T) {
	require.Equal(t, "agenthive", ServiceTag)
}
