package transport

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/flynn/noise"
)

// NoiseKeypair holds a Curve25519 keypair for Noise Protocol.
type NoiseKeypair struct {
	Private []byte
	Public  []byte
}

// NoiseConn wraps a net.Conn with Noise Protocol encryption.
// Writes are dispatched to a background goroutine to avoid deadlocks
// on synchronous transports such as net.Pipe.
type NoiseConn struct {
	writeMu      sync.Mutex
	conn         net.Conn
	send         *noise.CipherState
	recv         *noise.CipherState
	remoteStatic []byte
	writeCh      chan []byte
	done         chan struct{}
	closeOnce    sync.Once
}

// PeerVerifier is called during handshake with the remote peer's static public key.
// Return nil to accept, non-nil error to reject.
type PeerVerifier func(remoteStaticKey []byte) error

// maxNoisePayload is the maximum Noise message payload (before encryption overhead).
const maxNoisePayload = 65535 - 16 // 65519 bytes (minus AEAD tag)

// GenerateNoiseKeypair generates a new Curve25519 keypair for Noise Protocol.
func GenerateNoiseKeypair() (*NoiseKeypair, error) {
	kp, err := noise.DH25519.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate noise keypair: %w", err)
	}
	return &NoiseKeypair{
		Private: kp.Private,
		Public:  kp.Public,
	}, nil
}

func noiseConfig(localKey *NoiseKeypair, initiator bool) noise.Config {
	return noise.Config{
		CipherSuite: noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256),
		Pattern:     noise.HandshakeXX,
		Initiator:   initiator,
		StaticKeypair: noise.DHKey{
			Private: localKey.Private,
			Public:  localKey.Public,
		},
	}
}

func newNoiseConn(conn net.Conn, send, recv *noise.CipherState, remoteStatic []byte) *NoiseConn {
	nc := &NoiseConn{
		conn:         conn,
		send:         send,
		recv:         recv,
		remoteStatic: remoteStatic,
		writeCh:      make(chan []byte, 64),
		done:         make(chan struct{}),
	}
	go nc.writeLoop()
	return nc
}

// writeLoop drains buffered writes to the underlying connection.
func (nc *NoiseConn) writeLoop() {
	for data := range nc.writeCh {
		nc.conn.Write(data) //nolint:errcheck // errors propagate as read failures
	}
}

// NoiseHandshakeInitiator performs the initiator side of a Noise_XX handshake.
func NoiseHandshakeInitiator(conn net.Conn, localKey *NoiseKeypair, verify PeerVerifier) (*NoiseConn, error) {
	hs, err := noise.NewHandshakeState(noiseConfig(localKey, true))
	if err != nil {
		return nil, fmt.Errorf("noise handshake init: %w", err)
	}

	// Message 1: initiator -> responder (e)
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("noise write msg1: %w", err)
	}
	if err := writeFrame(conn, msg1); err != nil {
		return nil, fmt.Errorf("noise send msg1: %w", err)
	}

	// Message 2: responder -> initiator (e, ee, s, es)
	msg2, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("noise read msg2: %w", err)
	}
	_, _, _, err = hs.ReadMessage(nil, msg2)
	if err != nil {
		return nil, fmt.Errorf("noise process msg2: %w", err)
	}

	// Message 3: initiator -> responder (s, se)
	msg3, csI, csR, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("noise write msg3: %w", err)
	}
	if err := writeFrame(conn, msg3); err != nil {
		return nil, fmt.Errorf("noise send msg3: %w", err)
	}

	remoteStatic := hs.PeerStatic()

	if verify != nil {
		if err := verify(remoteStatic); err != nil {
			return nil, err
		}
	}

	return newNoiseConn(conn, csI, csR, remoteStatic), nil
}

// NoiseHandshakeResponder performs the responder side of a Noise_XX handshake.
func NoiseHandshakeResponder(conn net.Conn, localKey *NoiseKeypair, verify PeerVerifier) (*NoiseConn, error) {
	hs, err := noise.NewHandshakeState(noiseConfig(localKey, false))
	if err != nil {
		return nil, fmt.Errorf("noise handshake init: %w", err)
	}

	// Message 1: initiator -> responder (e)
	msg1, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("noise read msg1: %w", err)
	}
	_, _, _, err = hs.ReadMessage(nil, msg1)
	if err != nil {
		return nil, fmt.Errorf("noise process msg1: %w", err)
	}

	// Message 2: responder -> initiator (e, ee, s, es)
	msg2, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("noise write msg2: %w", err)
	}
	if err := writeFrame(conn, msg2); err != nil {
		return nil, fmt.Errorf("noise send msg2: %w", err)
	}

	// Message 3: initiator -> responder (s, se)
	msg3, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("noise read msg3: %w", err)
	}
	_, csR, csI, err := hs.ReadMessage(nil, msg3)
	if err != nil {
		return nil, fmt.Errorf("noise process msg3: %w", err)
	}

	remoteStatic := hs.PeerStatic()

	if verify != nil {
		if err := verify(remoteStatic); err != nil {
			return nil, err
		}
	}

	return newNoiseConn(conn, csI, csR, remoteStatic), nil
}

// WriteMessage encrypts and sends a message. Handles chunking for messages
// larger than the Noise maximum payload size. The encrypted bytes are queued
// to a background writer so callers are never blocked by slow or synchronous
// transports.
func (nc *NoiseConn) WriteMessage(msg []byte) error {
	nc.writeMu.Lock()
	defer nc.writeMu.Unlock()

	var buf bytes.Buffer

	// Encode the total unencrypted message length (4 bytes, big-endian)
	totalLen := make([]byte, 4)
	binary.BigEndian.PutUint32(totalLen, uint32(len(msg)))

	// Encrypt the length as a small Noise message
	encLen, err := nc.send.Encrypt(nil, nil, totalLen)
	if err != nil {
		return fmt.Errorf("noise encrypt length: %w", err)
	}
	appendFrame(&buf, encLen)

	// Encrypt the payload in chunks that fit within Noise's max payload
	for len(msg) > 0 {
		chunk := msg
		if len(chunk) > maxNoisePayload {
			chunk = msg[:maxNoisePayload]
		}
		msg = msg[len(chunk):]

		encrypted, err := nc.send.Encrypt(nil, nil, chunk)
		if err != nil {
			return fmt.Errorf("noise encrypt chunk: %w", err)
		}
		appendFrame(&buf, encrypted)
	}

	// Queue the complete wire message for async delivery
	select {
	case nc.writeCh <- buf.Bytes():
		return nil
	case <-nc.done:
		return fmt.Errorf("noise conn closed")
	}
}

// ReadMessage reads and decrypts a message.
func (nc *NoiseConn) ReadMessage() ([]byte, error) {
	// Read the encrypted length
	encLen, err := readFrame(nc.conn)
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("noise read length: %w", err)
	}

	totalLenBytes, err := nc.recv.Decrypt(nil, nil, encLen)
	if err != nil {
		return nil, fmt.Errorf("noise decrypt length: %w", err)
	}
	totalLen := binary.BigEndian.Uint32(totalLenBytes)

	// Read chunks until we have the full message
	result := make([]byte, 0, totalLen)
	for uint32(len(result)) < totalLen {
		encChunk, err := readFrame(nc.conn)
		if err != nil {
			return nil, fmt.Errorf("noise read chunk: %w", err)
		}

		chunk, err := nc.recv.Decrypt(nil, nil, encChunk)
		if err != nil {
			return nil, fmt.Errorf("noise decrypt chunk: %w", err)
		}
		result = append(result, chunk...)
	}

	return result, nil
}

func (nc *NoiseConn) RemoteStatic() []byte {
	return nc.remoteStatic
}

func (nc *NoiseConn) Close() error {
	nc.closeOnce.Do(func() {
		close(nc.done)
		close(nc.writeCh)
	})
	return nc.conn.Close()
}

// appendFrame appends a length-prefixed frame to a buffer.
func appendFrame(buf *bytes.Buffer, data []byte) {
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	buf.Write(header)
	buf.Write(data)
}

// writeFrame writes a length-prefixed frame to the connection.
// Frame format: [4 bytes big-endian length][payload]
func writeFrame(conn net.Conn, data []byte) error {
	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))
	copy(buf[4:], data)
	_, err := conn.Write(buf)
	return err
}

// readFrame reads a length-prefixed frame from the connection.
func readFrame(conn net.Conn) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(header)
	if length > 10*1024*1024 { // 10 MB sanity limit
		return nil, fmt.Errorf("frame too large: %d bytes", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}
