package crdt

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// PeerInfo holds metadata about a peer in the mesh.
type PeerInfo struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Addr     string `json:"addr,omitempty"`
	LinkType string `json:"link_type,omitempty"`
	LastSeen string `json:"last_seen,omitempty"`
}

// RouteMatch defines the selector for a routing rule.
type RouteMatch struct {
	Agent    string `json:"agent,omitempty"`
	Project  string `json:"project,omitempty"`
	Session  string `json:"session,omitempty"`
	Window   string `json:"window,omitempty"`
	Pane     string `json:"pane,omitempty"`
	Source   string `json:"source,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// RouteRule defines where to send notifications matching the selector.
type RouteRule struct {
	Match   RouteMatch `json:"match"`
	Targets []string   `json:"targets"`
	Action  string     `json:"action,omitempty"` // "notify" or "notify+action"
}

// ConfigEntry holds a config value with provenance metadata.
type ConfigEntry struct {
	Value     string `json:"value"`
	UpdatedBy string `json:"updated_by,omitempty"`
}

// StateStore holds the distributed CRDT state for the mesh.
// It wraps three LWW-Maps: peers, routes, and config.
// CRITICAL: Merge operations happen at the LWWMap level to preserve
// CRDT timestamps. Never re-stamp entries with fresh local timestamps
// during merge -- that would make the last merger always win instead
// of the last writer.
type StateStore struct {
	clock  *HLC
	peerID string
	peers  *LWWMap[PeerInfo]
	routes *LWWMap[RouteRule]
	config *LWWMap[ConfigEntry]
}

// NewStateStore creates a new state store with the given peer ID for the HLC.
func NewStateStore(peerID string) *StateStore {
	return &StateStore{
		clock:  NewHLC(peerID),
		peerID: peerID,
		peers:  NewLWWMap[PeerInfo](),
		routes: NewLWWMap[RouteRule](),
		config: NewLWWMap[ConfigEntry](),
	}
}

func (s *StateStore) SetPeer(id string, info PeerInfo) {
	s.peers.Set(id, info, s.clock.Now())
}

func (s *StateStore) GetPeer(id string) (PeerInfo, bool) {
	return s.peers.Get(id)
}

func (s *StateStore) ListPeers() map[string]PeerInfo {
	result := make(map[string]PeerInfo)
	for _, k := range s.peers.Keys() {
		if v, ok := s.peers.Get(k); ok {
			result[k] = v
		}
	}
	return result
}

func (s *StateStore) SetRoute(id string, rule RouteRule) {
	s.routes.Set(id, rule, s.clock.Now())
}

func (s *StateStore) GetRoute(id string) (RouteRule, bool) {
	return s.routes.Get(id)
}

func (s *StateStore) ListRoutes() map[string]RouteRule {
	result := make(map[string]RouteRule)
	for _, k := range s.routes.Keys() {
		if v, ok := s.routes.Get(k); ok {
			result[k] = v
		}
	}
	return result
}

func (s *StateStore) DeleteRoute(id string) {
	s.routes.Delete(id, s.clock.Now())
}

func (s *StateStore) SetConfig(key string, value string) {
	s.config.Set(key, ConfigEntry{Value: value, UpdatedBy: s.peerID}, s.clock.Now())
}

func (s *StateStore) GetConfig(key string) (string, bool) {
	entry, ok := s.config.Get(key)
	if !ok {
		return "", false
	}
	return entry.Value, true
}

func (s *StateStore) CurrentTimestamp() Timestamp {
	return s.clock.Now()
}

// PeersMap returns the underlying CRDT map for peer state.
// Used for merge operations that preserve timestamps.
func (s *StateStore) PeersMap() *LWWMap[PeerInfo] {
	return s.peers.Snapshot()
}

// RoutesMap returns the underlying CRDT map for route state.
func (s *StateStore) RoutesMap() *LWWMap[RouteRule] {
	return s.routes.Snapshot()
}

// ConfigMap returns the underlying CRDT map for config state.
func (s *StateStore) ConfigMap() *LWWMap[ConfigEntry] {
	return s.config.Snapshot()
}

// MergeMaps merges remote CRDT maps into the local store.
// This preserves the original HLC timestamps from the remote peer,
// ensuring that the true last-writer wins (not the last merger).
func (s *StateStore) MergeMaps(peers *LWWMap[PeerInfo], routes *LWWMap[RouteRule], config *LWWMap[ConfigEntry]) {
	if peers != nil {
		s.peers.Merge(peers)
	}
	if routes != nil {
		s.routes.Merge(routes)
	}
	if config != nil {
		s.config.Merge(config)
	}
}

// DeltaMaps returns LWW-Maps containing only entries changed since the given timestamp.
// Includes tombstones (deleted entries) so deletions propagate.
func (s *StateStore) DeltaMaps(since Timestamp) (*LWWMap[PeerInfo], *LWWMap[RouteRule], *LWWMap[ConfigEntry]) {
	return s.peers.Delta(since), s.routes.Delta(since), s.config.Delta(since)
}

func (s *StateStore) SaveToFile(path string) error {
	type persistedState struct {
		Peers  *LWWMap[PeerInfo]    `json:"peers"`
		Routes *LWWMap[RouteRule]   `json:"routes"`
		Config *LWWMap[ConfigEntry] `json:"config"`
	}

	data, err := json.MarshalIndent(persistedState{
		Peers:  s.peers,
		Routes: s.routes,
		Config: s.config,
	}, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *StateStore) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	type persistedState struct {
		Peers  *LWWMap[PeerInfo]    `json:"peers"`
		Routes *LWWMap[RouteRule]   `json:"routes"`
		Config *LWWMap[ConfigEntry] `json:"config"`
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if state.Peers != nil {
		s.peers.Merge(state.Peers)
	}
	if state.Routes != nil {
		s.routes.Merge(state.Routes)
	}
	if state.Config != nil {
		s.config.Merge(state.Config)
	}
	return nil
}
