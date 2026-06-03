// Package transport builds a libp2p Host configured with the wire concerns
// agenthive needs: TCP + QUIC-v1, Noise (with TLS as fallback), yamux/mplex
// muxers, NAT traversal (NATPortMap + EnableNATService + EnableHolePunching),
// circuit-relay v2 (EnableRelayService + EnableAutoRelay).
package transport

import (
	"context"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
)

// Config controls the libp2p Host that New constructs.
type Config struct {
	// Identity is the libp2p private key. Required.
	Identity crypto.PrivKey
	// ListenAddrs is the list of multiaddr strings the host listens on.
	// Defaults to TCP+QUIC on all v4 and v6 interfaces, ephemeral ports.
	ListenAddrs []string
	// EnableMDNS is informational only; the discovery package owns mDNS.
	EnableMDNS bool
}

// New constructs a libp2p Host wired with the agenthive transport options.
// Callers must invoke host.Close on the returned Host.
func New(ctx context.Context, cfg Config) (host.Host, error) {
	panic("not implemented: transport.New")
}

// MultiaddrsFor returns the list of /ip*/.../p2p/<peerid> strings for the
// host's listen and observed addresses.
func MultiaddrsFor(h host.Host) []string {
	panic("not implemented: transport.MultiaddrsFor")
}
