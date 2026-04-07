package transport

import (
	"errors"
	"sync"
)

// PipeLink is an in-process link for testing. Two PipeLinks form a pair:
// each one's Send delivers to the other's Receive channel.
type PipeLink struct {
	mu       sync.Mutex
	peerID   string
	sendCh   chan Envelope // outbound: this link writes here
	recvCh   chan Envelope // inbound: this link reads from here
	closed   bool
	closedCh chan struct{}
}

// NewPipeLinkPair creates two connected PipeLinks.
// linkA.Send() delivers to linkB.Receive() and vice versa.
func NewPipeLinkPair(peerIDA, peerIDB string) (*PipeLink, *PipeLink) {
	aToB := make(chan Envelope, 64)
	bToA := make(chan Envelope, 64)

	linkA := &PipeLink{
		peerID:   peerIDB, // A's remote peer is B
		sendCh:   aToB,
		recvCh:   bToA,
		closedCh: make(chan struct{}),
	}
	linkB := &PipeLink{
		peerID:   peerIDA, // B's remote peer is A
		sendCh:   bToA,
		recvCh:   aToB,
		closedCh: make(chan struct{}),
	}
	return linkA, linkB
}

func (p *PipeLink) Send(env Envelope) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("link closed")
	}
	p.mu.Unlock()

	select {
	case p.sendCh <- env:
		return nil
	case <-p.closedCh:
		return errors.New("link closed")
	}
}

func (p *PipeLink) Receive() <-chan Envelope {
	return p.recvCh
}

func (p *PipeLink) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	close(p.closedCh)
	return nil
}

func (p *PipeLink) Status() LinkStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return StatusDisconnected
	}
	return StatusConnected
}

func (p *PipeLink) PeerID() string {
	return p.peerID
}

// Compile-time interface check.
var _ Link = (*PipeLink)(nil)
