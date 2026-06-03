package protocols

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReadFramed_ActionRequest_Roundtrip(t *testing.T) {
	t.Parallel()

	in := ActionRequest{
		ActionID:  "abc123",
		SessionID: "sess-1",
		ToolUseID: "tu-42",
		ToolName:  "Bash",
		ToolInput: "rm -rf /tmp/foo",
		Project:   "agenthive",
		CWD:       "/home/me",
		Timestamp: time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 6, 4, 10, 0, 30, 0, time.UTC),
	}

	var buf bytes.Buffer
	require.NoError(t, WriteFramed(&buf, in))

	var out ActionRequest
	require.NoError(t, ReadFramed(&buf, &out))
	assert.Equal(t, in, out)
}

func TestWriteReadFramed_ActionResponse_Roundtrip(t *testing.T) {
	t.Parallel()

	in := ActionResponse{
		ActionID:  "abc123",
		Decision:  "allow",
		DecidedBy: "12D3KooWPeerID",
	}

	var buf bytes.Buffer
	require.NoError(t, WriteFramed(&buf, in))

	var out ActionResponse
	require.NoError(t, ReadFramed(&buf, &out))
	assert.Equal(t, in, out)
}

func TestWriteReadFramed_Notification_Roundtrip(t *testing.T) {
	t.Parallel()

	in := Notification{
		SessionID: "sess-9",
		Source:    "claude-code",
		Project:   "agenthive",
		Priority:  "warning",
		Message:   "task waiting on input",
		Timestamp: time.Date(2026, 6, 4, 11, 30, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	require.NoError(t, WriteFramed(&buf, in))

	var out Notification
	require.NoError(t, ReadFramed(&buf, &out))
	assert.Equal(t, in, out)
}

func TestWriteReadFramed_PeerAnnounce_Roundtrip(t *testing.T) {
	t.Parallel()

	in := PeerAnnounce{
		PeerID:     "12D3KooWSomething",
		Multiaddrs: []string{"/ip4/127.0.0.1/tcp/4001", "/ip6/::1/udp/4001/quic-v1"},
		Timestamp:  time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	require.NoError(t, WriteFramed(&buf, in))

	var out PeerAnnounce
	require.NoError(t, ReadFramed(&buf, &out))
	assert.Equal(t, in, out)
}

func TestWriteReadFramed_StateDelta_Roundtrip(t *testing.T) {
	t.Parallel()

	in := StateDelta{
		From:   "12D3KooWFromPeer",
		Peers:  []byte("peers-bytes"),
		Routes: []byte("routes-bytes"),
		Config: []byte("config-bytes"),
	}

	var buf bytes.Buffer
	require.NoError(t, WriteFramed(&buf, in))

	var out StateDelta
	require.NoError(t, ReadFramed(&buf, &out))
	assert.Equal(t, in, out)
}

func TestWriteFramed_PrependsBigEndianLength(t *testing.T) {
	t.Parallel()

	type small struct {
		X int `json:"x"`
	}
	var buf bytes.Buffer
	require.NoError(t, WriteFramed(&buf, small{X: 1}))

	raw := buf.Bytes()
	require.GreaterOrEqual(t, len(raw), 4, "frame must include a 4-byte length prefix")

	declared := binary.BigEndian.Uint32(raw[:4])
	body := raw[4:]
	assert.Equal(t, uint32(len(body)), declared)
}

func TestReadFramed_RejectsOversizeFrame(t *testing.T) {
	t.Parallel()

	// Just declare a length > 16 MiB; we don't need to actually have the bytes.
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], 16*1024*1024+1)

	var out ActionRequest
	err := ReadFramed(bytes.NewReader(prefix[:]), &out)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(strings.ToLower(err.Error()), "frame"),
		"err should mention frame size; got: %v", err,
	)
}

func TestReadFramed_AllowsMaxFrame(t *testing.T) {
	t.Parallel()

	// Construct a frame exactly at the 16 MiB body limit and verify it decodes.
	// Use a Notification whose Message field is padded out to fill the frame.
	const maxBody = 16 * 1024 * 1024

	// Empirically size the message so the encoded JSON is exactly maxBody bytes.
	// Start with a short message, encode, then pad/shrink by adjusting the message.
	probe := Notification{
		SessionID: "x",
		Source:    "x",
		Priority:  "info",
		Message:   "",
		Timestamp: time.Unix(0, 0).UTC(),
	}
	var probeBuf bytes.Buffer
	require.NoError(t, WriteFramed(&probeBuf, probe))
	headerSize := 4
	probeBodyLen := probeBuf.Len() - headerSize
	pad := maxBody - probeBodyLen
	if pad < 0 {
		t.Fatalf("probe encoding %d already exceeds max %d", probeBodyLen, maxBody)
	}
	probe.Message = strings.Repeat("a", pad)

	var buf bytes.Buffer
	require.NoError(t, WriteFramed(&buf, probe))
	body := buf.Len() - headerSize
	require.Equal(t, maxBody, body, "test setup must produce exactly maxBody-sized body")

	var out Notification
	require.NoError(t, ReadFramed(&buf, &out))
	assert.Equal(t, probe, out)
}

func TestReadFramed_ShortPrefixReturnsError(t *testing.T) {
	t.Parallel()

	var out ActionResponse
	err := ReadFramed(bytes.NewReader([]byte{0x00, 0x01}), &out)
	require.Error(t, err)
	// Anything wrapping io.EOF / io.ErrUnexpectedEOF is acceptable.
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Logf("got: %v (acceptable as long as it's an error)", err)
	}
}

func TestReadFramed_TruncatedBodyReturnsError(t *testing.T) {
	t.Parallel()

	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], 100)
	// Provide only 10 bytes of body when 100 were promised.
	buf := bytes.NewReader(append(prefix[:], make([]byte, 10)...))

	var out ActionRequest
	err := ReadFramed(buf, &out)
	require.Error(t, err)
}

func TestReadFramed_InvalidJSONReturnsError(t *testing.T) {
	t.Parallel()

	body := []byte("not-json")
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(body)))
	buf := bytes.NewReader(append(prefix[:], body...))

	var out ActionRequest
	err := ReadFramed(buf, &out)
	require.Error(t, err)
}

func TestWriteFramed_TwoFramesAreIndependent(t *testing.T) {
	t.Parallel()

	a := ActionResponse{ActionID: "a", Decision: "allow", DecidedBy: "p1"}
	b := ActionResponse{ActionID: "b", Decision: "deny", DecidedBy: "p2"}

	var buf bytes.Buffer
	require.NoError(t, WriteFramed(&buf, a))
	require.NoError(t, WriteFramed(&buf, b))

	var outA, outB ActionResponse
	require.NoError(t, ReadFramed(&buf, &outA))
	require.NoError(t, ReadFramed(&buf, &outB))
	assert.Equal(t, a, outA)
	assert.Equal(t, b, outB)
}

// TestWriteFramed_ConcurrentWritersAreSerialised verifies that WriteFramed is
// safe to call from multiple goroutines targeting independent io.Writers.
// (The function itself need not serialise on a shared writer; callers do.)
func TestWriteFramed_ParallelToIndependentBuffers(t *testing.T) {
	t.Parallel()

	const n = 64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			if err := WriteFramed(&buf, ActionResponse{ActionID: "x", Decision: "allow"}); err != nil {
				t.Errorf("WriteFramed: %v", err)
				return
			}
			var out ActionResponse
			if err := ReadFramed(&buf, &out); err != nil {
				t.Errorf("ReadFramed: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestProtocolIDConstants_AreStable(t *testing.T) {
	t.Parallel()

	// These IDs are wire-visible; locking them down catches accidental edits.
	assert.Equal(t, "/agenthive/action/request/1", ProtoActionRequest)
	assert.Equal(t, "/agenthive/action/response/1", ProtoActionResponse)
	assert.Equal(t, "/agenthive/notification/1", ProtoNotification)
	assert.Equal(t, "/agenthive/peer/announce/1", ProtoPeerAnnounce)
	assert.Equal(t, "/agenthive/state/v1", TopicState)
}
