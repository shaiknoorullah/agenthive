package transport

import "net"

// Suppress unused import.
var _ net.Conn

// TCPLink is a direct TCP connection with Noise Protocol encryption.
type TCPLink struct{}

func DialTCPLink(addr string, localKey *NoiseKeypair, remotePeerID string, verify PeerVerifier) (*TCPLink, error) {
	return nil, nil
}
func NewTCPLinkFromConn(conn net.Conn, localKey *NoiseKeypair, remotePeerID string, initiator bool, verify PeerVerifier) (*TCPLink, error) {
	return nil, nil
}
func (tl *TCPLink) Send(env Envelope) error    { return nil }
func (tl *TCPLink) Receive() <-chan Envelope    { return nil }
func (tl *TCPLink) Close() error               { return nil }
func (tl *TCPLink) Status() LinkStatus          { return "" }
func (tl *TCPLink) PeerID() string              { return "" }
