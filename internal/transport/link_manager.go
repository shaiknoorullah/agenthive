package transport

import (
	"fmt"
	"sort"
	"sync"
)

// LinkManager manages active links, broadcasts outbound messages,
// and aggregates inbound messages from all links into a single channel.
// Safe for concurrent use.
type LinkManager struct {
	mu          sync.RWMutex
	localPeerID string
	links       map[string]Link          // peerID -> Link
	inbound     chan Envelope              // aggregated inbound from all links
	closedCh    chan struct{}
	closed      bool
	wg          sync.WaitGroup
}

// NewLinkManager creates a new LinkManager for the given local peer.
func NewLinkManager(localPeerID string) *LinkManager {
	return &LinkManager{
		localPeerID: localPeerID,
		links:       make(map[string]Link),
		inbound:     make(chan Envelope, 256),
		closedCh:    make(chan struct{}),
	}
}

// AddLink registers a new link. If a link to the same peer already exists,
// the old link is closed and replaced.
func (lm *LinkManager) AddLink(link Link) {
	peerID := link.PeerID()

	lm.mu.Lock()
	if old, exists := lm.links[peerID]; exists {
		old.Close()
	}
	lm.links[peerID] = link
	lm.mu.Unlock()

	// Start forwarding inbound messages from this link
	lm.wg.Add(1)
	go lm.forwardInbound(link)
}

func (lm *LinkManager) forwardInbound(link Link) {
	defer lm.wg.Done()

	recvCh := link.Receive()
	for {
		select {
		case env, ok := <-recvCh:
			if !ok {
				return
			}
			select {
			case lm.inbound <- env:
			case <-lm.closedCh:
				return
			}
		case <-lm.closedCh:
			return
		}
	}
}

// RemoveLink closes and removes the link to the specified peer.
func (lm *LinkManager) RemoveLink(peerID string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if link, exists := lm.links[peerID]; exists {
		link.Close()
		delete(lm.links, peerID)
	}
}

// LinkCount returns the number of active links.
func (lm *LinkManager) LinkCount() int {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return len(lm.links)
}

// ConnectedPeers returns the peer IDs of all active links, sorted.
func (lm *LinkManager) ConnectedPeers() []string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	peers := make([]string, 0, len(lm.links))
	for pid := range lm.links {
		peers = append(peers, pid)
	}
	sort.Strings(peers)
	return peers
}

// Broadcast sends an envelope to all connected peers.
// Returns nil even if no links exist (broadcast to empty set is valid).
// Collects errors but does not stop on first failure.
func (lm *LinkManager) Broadcast(env Envelope) error {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var lastErr error
	for _, link := range lm.links {
		if err := link.Send(env); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// SendTo sends an envelope to a specific peer.
// Returns an error if the peer is not connected.
func (lm *LinkManager) SendTo(peerID string, env Envelope) error {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	link, exists := lm.links[peerID]
	if !exists {
		return fmt.Errorf("no link to peer %q", peerID)
	}
	return link.Send(env)
}

// Inbound returns the channel that aggregates all inbound messages
// from all connected links.
func (lm *LinkManager) Inbound() <-chan Envelope {
	return lm.inbound
}

// GetLinkStatus returns the status of the link to the specified peer.
func (lm *LinkManager) GetLinkStatus(peerID string) (LinkStatus, bool) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	link, exists := lm.links[peerID]
	if !exists {
		return "", false
	}
	return link.Status(), true
}

// Close closes all links and stops the link manager.
func (lm *LinkManager) Close() error {
	lm.mu.Lock()
	if lm.closed {
		lm.mu.Unlock()
		return nil
	}
	lm.closed = true
	close(lm.closedCh)

	for _, link := range lm.links {
		link.Close()
	}
	lm.links = make(map[string]Link)
	lm.mu.Unlock()

	lm.wg.Wait()
	return nil
}
