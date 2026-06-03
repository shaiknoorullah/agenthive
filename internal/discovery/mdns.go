// Package discovery wraps the libp2p mDNS LAN peer-discovery service in a
// callback-based API so the daemon can route discovered peers into its
// StateStore.
package discovery

import (
	"errors"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// PeerFoundCallback is invoked for every peer the mDNS service discovers on
// the local network.
type PeerFoundCallback func(peer.AddrInfo)

// ServiceTag is the mDNS service name agenthive uses. Peers running with the
// same tag on the same LAN will discover each other.
const ServiceTag = "agenthive"

// ErrNilCallback is returned by StartMDNS when the caller passes a nil
// callback. The mDNS notifee would panic on the first discovery otherwise,
// which is much harder to diagnose than an explicit error at startup.
var ErrNilCallback = errors.New("discovery: PeerFoundCallback must not be nil")

// notifee adapts a PeerFoundCallback to the mdns.Notifee interface expected by
// go-libp2p's mDNS implementation.
type notifee struct {
	cb PeerFoundCallback
}

// HandlePeerFound is called by the underlying mDNS service for each peer it
// discovers. We forward the AddrInfo to the user-supplied callback verbatim;
// any filtering (e.g. ignoring self, dedup) is the caller's responsibility.
func (n *notifee) HandlePeerFound(pi peer.AddrInfo) {
	n.cb(pi)
}

// StartMDNS starts a libp2p mDNS discovery service on the given host. The
// callback cb is invoked for every peer the service discovers on the local
// network, including — depending on the platform — the host itself.
//
// StartMDNS returns a stop function that closes the underlying mDNS service.
// It is safe to call the stop function exactly once; calling it more than
// once may return an error from the libp2p layer.
func StartMDNS(h host.Host, cb PeerFoundCallback) (func() error, error) {
	if cb == nil {
		return nil, ErrNilCallback
	}

	svc := mdns.NewMdnsService(h, ServiceTag, &notifee{cb: cb})
	if err := svc.Start(); err != nil {
		return nil, err
	}

	return svc.Close, nil
}
