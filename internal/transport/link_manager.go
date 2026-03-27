package transport

// LinkManager manages active links, broadcasts outbound messages,
// and aggregates inbound messages from all links.
type LinkManager struct{}

func NewLinkManager(localPeerID string) *LinkManager                        { return nil }
func (lm *LinkManager) AddLink(link Link)                                   {}
func (lm *LinkManager) RemoveLink(peerID string)                            {}
func (lm *LinkManager) LinkCount() int                                      { return 0 }
func (lm *LinkManager) ConnectedPeers() []string                            { return nil }
func (lm *LinkManager) Broadcast(env Envelope) error                        { return nil }
func (lm *LinkManager) SendTo(peerID string, env Envelope) error            { return nil }
func (lm *LinkManager) Inbound() <-chan Envelope                             { return nil }
func (lm *LinkManager) GetLinkStatus(peerID string) (LinkStatus, bool)      { return "", false }
func (lm *LinkManager) Close() error                                         { return nil }
