package transport

import (
	"errors"
	"io"
	"net"
)

// Suppress unused import warnings.
var _ = errors.New
var _ io.Reader
var _ net.Conn

// NoiseKeypair holds a Curve25519 keypair for Noise Protocol.
type NoiseKeypair struct {
	Private []byte
	Public  []byte
}

// NoiseConn wraps a net.Conn with Noise Protocol encryption.
type NoiseConn struct{}

// PeerVerifier is called during handshake with the remote peer's static public key.
// Return nil to accept, non-nil error to reject.
type PeerVerifier func(remoteStaticKey []byte) error

func GenerateNoiseKeypair() (*NoiseKeypair, error)                                              { return nil, nil }
func NoiseHandshakeInitiator(conn net.Conn, localKey *NoiseKeypair, verify PeerVerifier) (*NoiseConn, error) { return nil, nil }
func NoiseHandshakeResponder(conn net.Conn, localKey *NoiseKeypair, verify PeerVerifier) (*NoiseConn, error) { return nil, nil }
func (nc *NoiseConn) WriteMessage(msg []byte) error                                              { return nil }
func (nc *NoiseConn) ReadMessage() ([]byte, error)                                               { return nil, nil }
func (nc *NoiseConn) RemoteStatic() []byte                                                        { return nil }
func (nc *NoiseConn) Close() error                                                                { return nil }
