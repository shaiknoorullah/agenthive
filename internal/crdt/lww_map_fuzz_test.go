package crdt

import (
	"encoding/json"
	"testing"
)

func FuzzLWWMapJSONRoundTrip(f *testing.F) {
	// Seed corpus
	f.Add(`{"key1":{"value":"hello","ts":{"wall":"2026-03-26T14:00:00Z","counter":0,"peer_id":"a"}}}`)
	f.Add(`{}`)
	f.Add(`{"k":{"value":"","ts":{"wall":"2026-01-01T00:00:00Z","counter":999,"peer_id":"peer-xyz"}}}`)

	f.Fuzz(func(t *testing.T, data string) {
		var m LWWMap[string]
		err := json.Unmarshal([]byte(data), &m)
		if err != nil {
			return // invalid JSON is fine, just skip
		}

		// Re-marshal
		marshaled, err := json.Marshal(&m)
		if err != nil {
			t.Fatalf("marshal after unmarshal failed: %v", err)
		}

		// Re-unmarshal
		var m2 LWWMap[string]
		err = json.Unmarshal(marshaled, &m2)
		if err != nil {
			t.Fatalf("unmarshal after marshal failed: %v", err)
		}

		// Values should match
		for _, k := range m.Keys() {
			v1, ok1 := m.Get(k)
			v2, ok2 := m2.Get(k)
			if ok1 != ok2 || v1 != v2 {
				t.Fatalf("round-trip mismatch for key %q", k)
			}
		}
	})
}

func FuzzTimestampJSONRoundTrip(f *testing.F) {
	f.Add(`{"wall":"2026-03-26T14:00:00Z","counter":0,"peer_id":"a"}`)
	f.Add(`{"wall":"2026-01-01T00:00:00.123456789Z","counter":4294967295,"peer_id":"very-long-peer-id"}`)

	f.Fuzz(func(t *testing.T, data string) {
		var ts Timestamp
		err := ts.UnmarshalJSON([]byte(data))
		if err != nil {
			return
		}

		marshaled, err := ts.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal after unmarshal failed: %v", err)
		}

		var ts2 Timestamp
		err = ts2.UnmarshalJSON(marshaled)
		if err != nil {
			t.Fatalf("unmarshal after marshal failed: %v", err)
		}

		if !ts.Wall.Equal(ts2.Wall) || ts.Counter != ts2.Counter || ts.PeerID != ts2.PeerID {
			t.Fatalf("round-trip mismatch")
		}
	})
}
