package transport

// PipeLink is an in-process link for testing. Two PipeLinks form a pair.
type PipeLink struct{}

func NewPipeLinkPair(peerIDA, peerIDB string) (*PipeLink, *PipeLink) { return nil, nil }
func (p *PipeLink) Send(env Envelope) error                          { return nil }
func (p *PipeLink) Receive() <-chan Envelope                          { return nil }
func (p *PipeLink) Close() error                                      { return nil }
func (p *PipeLink) Status() LinkStatus                                { return "" }
func (p *PipeLink) PeerID() string                                    { return "" }
