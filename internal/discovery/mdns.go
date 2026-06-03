// Package discovery wraps the libp2p mDNS LAN peer-discovery service in a
// callback-based API so the daemon can route discovered peers into its
// StateStore.
package discovery

import (
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerFoundCallback is invoked for every peer the mDNS service discovers on
// the local network.
type PeerFoundCallback func(peer.AddrInfo)

// ServiceTag is the mDNS service name agenthive uses.
const ServiceTag = "agenthive"

// StartMDNS starts the mDNS service on the host. cb is invoked for each
// found peer. Returns a stop func that, when called, closes the underlying
// mDNS service.
func StartMDNS(h host.Host, cb PeerFoundCallback) (func() error, error) {
	panic("not implemented: discovery.StartMDNS")
}
