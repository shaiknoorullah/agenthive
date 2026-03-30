package transport

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoiseHandshake_MutualAuthentication(t *testing.T) {
	initiatorKey, err := GenerateNoiseKeypair()
	require.NoError(t, err)
	responderKey, err := GenerateNoiseKeypair()
	require.NoError(t, err)

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	var wg sync.WaitGroup
	var initiatorConn *NoiseConn
	var responderConn *NoiseConn
	var errI, errR error

	wg.Add(2)
	go func() {
		defer wg.Done()
		initiatorConn, errI = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		responderConn, errR = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	require.NoError(t, errI, "initiator handshake failed")
	require.NoError(t, errR, "responder handshake failed")

	// Both sides should know the other's public key
	assert.Equal(t, responderKey.Public, initiatorConn.RemoteStatic())
	assert.Equal(t, initiatorKey.Public, responderConn.RemoteStatic())
}

func TestNoiseConn_SendAndReceive(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	var wg sync.WaitGroup
	var iConn, rConn *NoiseConn

	wg.Add(2)
	go func() {
		defer wg.Done()
		iConn, _ = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		rConn, _ = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	// Send from initiator to responder
	message := []byte("hello from initiator")
	err := iConn.WriteMessage(message)
	require.NoError(t, err)

	received, err := rConn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, message, received)

	// Send from responder to initiator
	reply := []byte("hello from responder")
	err = rConn.WriteMessage(reply)
	require.NoError(t, err)

	received2, err := iConn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, reply, received2)
}

func TestNoiseConn_LargeMessage(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	var wg sync.WaitGroup
	var iConn, rConn *NoiseConn

	wg.Add(2)
	go func() {
		defer wg.Done()
		iConn, _ = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		rConn, _ = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	// Send a message larger than Noise's 65535 byte limit -- should be handled by chunking
	bigMsg := make([]byte, 100_000)
	for i := range bigMsg {
		bigMsg[i] = byte(i % 256)
	}

	err := iConn.WriteMessage(bigMsg)
	require.NoError(t, err)

	received, err := rConn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, bigMsg, received)
}

func TestNoiseConn_MultipleMessages(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	var wg sync.WaitGroup
	var iConn, rConn *NoiseConn

	wg.Add(2)
	go func() {
		defer wg.Done()
		iConn, _ = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		rConn, _ = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	for i := 0; i < 20; i++ {
		msg := []byte("message number")
		err := iConn.WriteMessage(msg)
		require.NoError(t, err)

		received, err := rConn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, msg, received)
	}
}

func TestNoiseConn_ReadAfterRemoteClose(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()

	var wg sync.WaitGroup
	var iConn, rConn *NoiseConn

	wg.Add(2)
	go func() {
		defer wg.Done()
		iConn, _ = NoiseHandshakeInitiator(connI, initiatorKey, nil)
	}()
	go func() {
		defer wg.Done()
		rConn, _ = NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	// Close the initiator side
	iConn.Close()
	connI.Close()

	// Responder should get an error on read
	_, err := rConn.ReadMessage()
	assert.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestNoiseHandshake_PeerVerification_Accepted(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	// Verifier that accepts the known key
	verifier := func(remoteKey []byte) error {
		if !bytes.Equal(remoteKey, responderKey.Public) {
			return io.ErrUnexpectedEOF
		}
		return nil
	}

	var wg sync.WaitGroup
	var errI error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errI = NoiseHandshakeInitiator(connI, initiatorKey, verifier)
	}()
	go func() {
		defer wg.Done()
		NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	assert.NoError(t, errI)
}

func TestNoiseHandshake_PeerVerification_Rejected(t *testing.T) {
	initiatorKey, _ := GenerateNoiseKeypair()
	responderKey, _ := GenerateNoiseKeypair()

	connI, connR := net.Pipe()
	defer connI.Close()
	defer connR.Close()

	// Verifier that rejects all keys
	rejecter := func(remoteKey []byte) error {
		return errors.New("untrusted peer")
	}

	var wg sync.WaitGroup
	var errI error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errI = NoiseHandshakeInitiator(connI, initiatorKey, rejecter)
	}()
	go func() {
		defer wg.Done()
		NoiseHandshakeResponder(connR, responderKey, nil)
	}()
	wg.Wait()

	assert.Error(t, errI)
	assert.Contains(t, errI.Error(), "untrusted")
}

func TestGenerateNoiseKeypair(t *testing.T) {
	kp1, err := GenerateNoiseKeypair()
	require.NoError(t, err)
	assert.Len(t, kp1.Private, 32)
	assert.Len(t, kp1.Public, 32)

	kp2, err := GenerateNoiseKeypair()
	require.NoError(t, err)

	// Two generated keypairs must be different
	assert.NotEqual(t, kp1.Private, kp2.Private)
	assert.NotEqual(t, kp1.Public, kp2.Public)
}
