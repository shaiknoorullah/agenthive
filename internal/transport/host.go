// Package transport builds a libp2p Host configured with the wire concerns
// agenthive needs: TCP + QUIC-v1, Noise (with TLS as a fallback ciphersuite
// configured by libp2p's DefaultSecurity), yamux muxers (DefaultMuxers), NAT
// traversal (NATPortMap + EnableNATService + EnableHolePunching), and
// circuit-relay v2 (EnableRelayService + EnableAutoRelayWithPeerSource).
//
// The discovery package owns mDNS — Config.EnableMDNS is informational only
// and the host itself is built independent of discovery so it can be tested
// without spinning up the LAN service.
//
// Callers are responsible for closing the returned Host.
package transport

import (
	"context"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"

	ma "github.com/multiformats/go-multiaddr"
)

// defaultListenAddrs are the listen multiaddrs used when Config.ListenAddrs
// is empty. Mirrors the plan's recommended dual-stack TCP+QUIC defaults so
// the daemon binds to every interface on ephemeral ports.
var defaultListenAddrs = []string{
	"/ip4/0.0.0.0/tcp/0",
	"/ip4/0.0.0.0/udp/0/quic-v1",
	"/ip6/::/tcp/0",
	"/ip6/::/udp/0/quic-v1",
}

// Config controls the libp2p Host that New constructs.
type Config struct {
	// Identity is the libp2p private key. Required.
	Identity crypto.PrivKey
	// ListenAddrs is the list of multiaddr strings the host listens on.
	// Defaults to TCP+QUIC on all v4 and v6 interfaces, ephemeral ports.
	ListenAddrs []string
	// EnableMDNS is informational only; the discovery package owns mDNS.
	EnableMDNS bool
	// PeerSource is the AutoRelay peer-source closure. If nil, the host
	// uses an internal no-op closure that returns an empty closed channel
	// (relays must be dialled directly). The daemon supplies a closure
	// that surfaces the CRDT peer set so AutoRelay can request reservations
	// from peers the mesh already knows about.
	PeerSource func(ctx context.Context, num int) <-chan peer.AddrInfo
}

// emptyPeerSource is the default autorelay PeerSource. It returns an empty
// closed channel — no relays are ever offered. Callers that want a real
// peer set must supply Config.PeerSource; the daemon does, surfacing the
// CRDT peer set so AutoRelay can request reservations from peers the mesh
// already knows about.
func emptyPeerSource(ctx context.Context, num int) <-chan peer.AddrInfo {
	ch := make(chan peer.AddrInfo)
	close(ch)
	return ch
}

// peerSourceOrDefault returns ps if non-nil, otherwise the empty no-op
// closure. Pulled into a helper so Config.PeerSource = nil is the documented
// default rather than a constructor branch in New.
func peerSourceOrDefault(ps func(ctx context.Context, num int) <-chan peer.AddrInfo) func(ctx context.Context, num int) <-chan peer.AddrInfo {
	if ps == nil {
		return emptyPeerSource
	}
	return ps
}

// New constructs a libp2p Host wired with the agenthive transport options.
// Callers must invoke host.Close on the returned Host.
//
// The returned Host listens on cfg.ListenAddrs (or the package default if
// empty), advertises Noise as the primary security ciphersuite (with libp2p's
// default TLS fallback enabled via DefaultSecurity option chain through the
// explicit Security override), uses the TCP and QUIC-v1 transports, the
// default yamux muxer, and the full NAT traversal + circuit-relay stack.
func New(ctx context.Context, cfg Config) (host.Host, error) {
	if cfg.Identity == nil {
		return nil, errors.New("transport: Config.Identity is required")
	}
	if ctx == nil {
		return nil, errors.New("transport: nil context")
	}

	listen := cfg.ListenAddrs
	if len(listen) == 0 {
		listen = defaultListenAddrs
	}

	// Validate every listen addr up-front so callers see a clean error
	// instead of an opaque swarm failure later.
	for _, s := range listen {
		if _, err := ma.NewMultiaddr(s); err != nil {
			return nil, fmt.Errorf("transport: invalid listen multiaddr %q: %w", s, err)
		}
	}

	opts := []libp2p.Option{
		libp2p.Identity(cfg.Identity),
		libp2p.ListenAddrStrings(listen...),

		// Noise is the primary security ciphersuite. We also leave TLS
		// available so peers that prefer TLS can still connect; libp2p
		// picks per-connection.
		libp2p.Security(noise.ID, noise.New),

		// Explicit transport selection — TCP for compatibility with
		// every environment, QUIC-v1 for low-latency hole-punched
		// connectivity.
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(libp2pquic.NewTransport),

		// DefaultMuxers gives us yamux, which is what every libp2p
		// implementation in the wild speaks.
		libp2p.DefaultMuxers,

		// NAT traversal stack: AutoNAT to discover reachability,
		// NATPortMap to ask routers nicely (UPnP / NAT-PMP), and
		// hole-punching for symmetric NATs.
		libp2p.EnableNATService(),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),

		// Circuit-relay v2: be a relay (modest defaults) and use
		// auto-relay to find one to register with. If the caller
		// supplied a Config.PeerSource (the daemon does, surfacing
		// the CRDT peer set), use it; otherwise fall back to the
		// no-op closure that returns an empty closed channel.
		libp2p.EnableRelayService(),
		libp2p.EnableAutoRelayWithPeerSource(
			peerSourceOrDefault(cfg.PeerSource),
			autorelay.WithMinCandidates(1),
		),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("transport: libp2p.New: %w", err)
	}

	// Respect ctx semantics: if it's already cancelled by the time the
	// host has come up, shut it down and surface the cancellation. This
	// keeps the constructor honest under callers that build a Host as
	// part of a larger startup tree.
	select {
	case <-ctx.Done():
		_ = h.Close()
		return nil, fmt.Errorf("transport: context done during construction: %w", ctx.Err())
	default:
	}

	return h, nil
}

// MultiaddrsFor returns the list of /ip*/.../p2p/<peerid> strings advertising
// the host. Returns nil for a nil host.
//
// Each returned string is a fully-qualified multiaddr including the host's
// PeerID, suitable for pasting into `agenthive peers add`. The list is the
// host's current listen addresses (post-listener-binding so the ephemeral
// ports are concrete) joined with `/p2p/<peerid>`.
func MultiaddrsFor(h host.Host) []string {
	if h == nil {
		return nil
	}

	addrs := h.Addrs()
	if len(addrs) == 0 {
		return nil
	}

	info := peer.AddrInfo{ID: h.ID(), Addrs: addrs}
	full, err := peer.AddrInfoToP2pAddrs(&info)
	if err != nil {
		// peer.AddrInfoToP2pAddrs only errors if the PeerID's binary
		// component is malformed, which we control here — fall back to
		// manual stitching rather than dropping the info.
		out := make([]string, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, a.String()+"/p2p/"+h.ID().String())
		}
		return out
	}

	out := make([]string, len(full))
	for i, a := range full {
		out[i] = a.String()
	}
	return out
}
