package transport

import (
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPLink_ConnectAndSend(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		require.NoError(t, err)
		serverLink, err = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
		require.NoError(t, err)
	}()

	clientLink, err := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	require.NoError(t, err)
	wg.Wait()

	defer clientLink.Close()
	defer serverLink.Close()

	// Client sends to server
	env := Envelope{
		Type: MsgNotification, ID: "tcp-001", From: "peer-client",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{"msg":"hello"}`),
	}
	err = clientLink.Send(env)
	require.NoError(t, err)

	select {
	case received := <-serverLink.Receive():
		assert.Equal(t, "tcp-001", received.ID)
		assert.Equal(t, MsgNotification, received.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTCPLink_Bidirectional(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := listener.Accept()
		serverLink, _ = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
	}()

	clientLink, err := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	require.NoError(t, err)
	wg.Wait()

	defer clientLink.Close()
	defer serverLink.Close()

	// Client -> Server
	clientLink.Send(Envelope{
		Type: MsgHeartbeat, ID: "c2s", From: "peer-client",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})

	// Server -> Client
	serverLink.Send(Envelope{
		Type: MsgHeartbeat, ID: "s2c", From: "peer-server",
		Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	})

	select {
	case msg := <-serverLink.Receive():
		assert.Equal(t, "c2s", msg.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: server did not receive")
	}

	select {
	case msg := <-clientLink.Receive():
		assert.Equal(t, "s2c", msg.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: client did not receive")
	}
}

func TestTCPLink_Status(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := listener.Accept()
		serverLink, _ = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
	}()

	clientLink, _ := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	wg.Wait()

	assert.Equal(t, StatusConnected, clientLink.Status())
	assert.Equal(t, StatusConnected, serverLink.Status())

	clientLink.Close()
	// Give the read goroutine time to detect the close
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, StatusDisconnected, clientLink.Status())
}

func TestTCPLink_PeerID(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := listener.Accept()
		serverLink, _ = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
	}()

	clientLink, _ := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	wg.Wait()

	defer clientLink.Close()
	defer serverLink.Close()

	assert.Equal(t, "peer-server", clientLink.PeerID())
	assert.Equal(t, "peer-client", serverLink.PeerID())
}

func TestTCPLink_MultipleMessages(t *testing.T) {
	serverKey, _ := GenerateNoiseKeypair()
	clientKey, _ := GenerateNoiseKeypair()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	var serverLink *TCPLink
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, _ := listener.Accept()
		serverLink, _ = NewTCPLinkFromConn(conn, serverKey, "peer-client", false, nil)
	}()

	clientLink, _ := DialTCPLink(listener.Addr().String(), clientKey, "peer-server", nil)
	wg.Wait()

	defer clientLink.Close()
	defer serverLink.Close()

	const count = 50
	for i := 0; i < count; i++ {
		clientLink.Send(Envelope{
			Type: MsgNotification, ID: "msg", From: "peer-client",
			Timestamp: time.Now().UTC(), Payload: json.RawMessage(`{}`),
		})
	}

	received := 0
	for received < count {
		select {
		case <-serverLink.Receive():
			received++
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout after receiving %d/%d messages", received, count)
		}
	}
	assert.Equal(t, count, received)
}

func TestTCPLink_ImplementsLinkInterface(t *testing.T) {
	// Compile-time check is in tcp_link.go, but verify here too
	var _ Link = (*TCPLink)(nil)
}
