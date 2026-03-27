package transport

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// TCPLink is a direct TCP connection with Noise Protocol encryption.
// Implements the Link interface. Used for LAN peers where SSH overhead
// is unnecessary.
type TCPLink struct {
	mu       sync.Mutex
	nconn    *NoiseConn
	peerID   string
	recvCh   chan Envelope
	status   LinkStatus
	closedCh chan struct{}
	closed   bool
}

// DialTCPLink connects to a remote peer and performs a Noise handshake as initiator.
func DialTCPLink(addr string, localKey *NoiseKeypair, remotePeerID string, verify PeerVerifier) (*TCPLink, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}
	return NewTCPLinkFromConn(conn, localKey, remotePeerID, true, verify)
}

// NewTCPLinkFromConn wraps an existing connection with Noise encryption.
// If initiator is true, performs the initiator side of the handshake.
func NewTCPLinkFromConn(conn net.Conn, localKey *NoiseKeypair, remotePeerID string, initiator bool, verify PeerVerifier) (*TCPLink, error) {
	var nconn *NoiseConn
	var err error

	if initiator {
		nconn, err = NoiseHandshakeInitiator(conn, localKey, verify)
	} else {
		nconn, err = NoiseHandshakeResponder(conn, localKey, verify)
	}
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("noise handshake: %w", err)
	}

	tl := &TCPLink{
		nconn:    nconn,
		peerID:   remotePeerID,
		recvCh:   make(chan Envelope, 64),
		status:   StatusConnected,
		closedCh: make(chan struct{}),
	}

	go tl.readLoop()
	return tl, nil
}

func (tl *TCPLink) readLoop() {
	defer func() {
		tl.mu.Lock()
		tl.status = StatusDisconnected
		tl.mu.Unlock()
		close(tl.recvCh)
	}()

	for {
		msg, err := tl.nconn.ReadMessage()
		if err != nil {
			return
		}

		var env Envelope
		if err := json.Unmarshal(msg, &env); err != nil {
			continue // skip malformed messages
		}

		select {
		case tl.recvCh <- env:
		case <-tl.closedCh:
			return
		}
	}
}

func (tl *TCPLink) Send(env Envelope) error {
	tl.mu.Lock()
	if tl.closed {
		tl.mu.Unlock()
		return fmt.Errorf("link closed")
	}
	tl.mu.Unlock()

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	return tl.nconn.WriteMessage(data)
}

func (tl *TCPLink) Receive() <-chan Envelope {
	return tl.recvCh
}

func (tl *TCPLink) Close() error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if tl.closed {
		return nil
	}
	tl.closed = true
	tl.status = StatusDisconnected
	close(tl.closedCh)
	return tl.nconn.Close()
}

func (tl *TCPLink) Status() LinkStatus {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.status
}

func (tl *TCPLink) PeerID() string {
	return tl.peerID
}

// Compile-time interface check.
var _ Link = (*TCPLink)(nil)
