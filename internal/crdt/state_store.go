package crdt

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

// StateStore holds the distributed state for the mesh.
type StateStore struct{}

func NewStateStore(peerID string) *StateStore                          { return nil }
func (s *StateStore) SetPeer(id string, info PeerInfo)                 {}
func (s *StateStore) GetPeer(id string) (PeerInfo, bool)               { return PeerInfo{}, false }
func (s *StateStore) ListPeers() map[string]PeerInfo                   { return nil }
func (s *StateStore) SetRoute(id string, rule RouteRule)               {}
func (s *StateStore) GetRoute(id string) (RouteRule, bool)             { return RouteRule{}, false }
func (s *StateStore) ListRoutes() map[string]RouteRule                 { return nil }
func (s *StateStore) DeleteRoute(id string)                            {}
func (s *StateStore) SetConfig(key string, value string)               {}
func (s *StateStore) GetConfig(key string) (string, bool)              { return "", false }
func (s *StateStore) CurrentTimestamp() Timestamp                      { return Timestamp{} }
func (s *StateStore) PeersMap() *LWWMap[PeerInfo]                      { return nil }
func (s *StateStore) RoutesMap() *LWWMap[RouteRule]                    { return nil }
func (s *StateStore) ConfigMap() *LWWMap[ConfigEntry]                  { return nil }
func (s *StateStore) MergeMaps(peers *LWWMap[PeerInfo], routes *LWWMap[RouteRule], config *LWWMap[ConfigEntry]) {}
func (s *StateStore) DeltaMaps(since Timestamp) (peers *LWWMap[PeerInfo], routes *LWWMap[RouteRule], config *LWWMap[ConfigEntry]) { return nil, nil, nil }
func (s *StateStore) SaveToFile(path string) error                     { return nil }
func (s *StateStore) LoadFromFile(path string) error                   { return nil }
