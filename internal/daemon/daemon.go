// Package daemon coordinates the libp2p Host, CRDT StateStore, GossipSub
// topic, stream handlers, mDNS discovery, dispatcher, action gate, and the
// Unix-socket hook IPC into a single Run loop.
//
// The Run loop is the integration seam for every other internal package in
// agenthive: nothing else in the codebase wires the subsystems together.
// Construction (New) only builds the dependency graph and validates the
// supplied Config; no goroutines start, no listeners bind, no files are
// written until Run is called. Run is single-shot — a Daemon cannot be
// re-Run after its Stop has fired. This mirrors how a process supervisor
// would treat the daemon binary: one start, one stop, no recycling.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/discovery"
	"github.com/shaiknoorullah/agenthive/internal/dispatch"
	"github.com/shaiknoorullah/agenthive/internal/hooks"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
	"github.com/shaiknoorullah/agenthive/internal/transport"
)

// stateFileName is the name of the persisted CRDT state file inside the
// daemon's ConfigDir. Plan L4 step 10 ("Persist state to state.json").
const stateFileName = "state.json"

// publishDebounce is the debounce window for the StateDelta publish goroutine.
// Multiple local mutations inside this window coalesce into one publish.
// Plan L4 step 8 says "debounced 200ms".
const publishDebounce = 200 * time.Millisecond

// publishMinInterval is the rate ceiling on publishes when the local state is
// actively churning. Plan L4 step 8 says "max once per 2s when active".
const publishMinInterval = 2 * time.Second

// streamReadTimeout caps how long a stream handler will wait for the framed
// request body. A peer that opens a stream and then sits idle should not be
// allowed to pin a goroutine forever.
const streamReadTimeout = 30 * time.Second

// Config is the daemon's startup configuration. Every field is required
// except ListenAddrs (which falls back to the transport package's loopback
// + ephemeral defaults).
type Config struct {
	ConfigDir   string
	Identity    crypto.PrivKey
	ListenAddrs []string
	LogPath     string
	SocketPath  string
}

// Daemon is the long-running coordinator. Construction sets up every
// component up-front so Run has nothing to do but start goroutines and
// block on ctx.Done().
//
// Field lifecycle:
//   - cfg/state/dispatcher/gate/socket: created by New. State outlives Run
//     because tests inspect it both before Run starts and after it returns.
//   - host/gossipsub/topic/sub/mdnsStop: created inside Run; nil before Run
//     starts and after Run returns.
//   - stopOnce/stopCh: drive Stop. stopCh is closed by Stop to act as a
//     secondary cancellation signal for Run, on top of the caller's ctx.
type Daemon struct {
	cfg        Config
	host       host.Host
	state      *crdt.StateStore
	gossipsub  *pubsub.PubSub
	topic      *pubsub.Topic
	sub        *pubsub.Subscription
	dispatcher *dispatch.Dispatcher
	gate       *hooks.Gate
	socket     *SocketServer
	mdnsStop   func() error

	// mu protects the mutable fields above against concurrent access from
	// accessor methods (Host, State, Multiaddrs) called by callers in
	// other goroutines (notably tests) while Run is still mid-construction.
	mu sync.RWMutex

	stopOnce sync.Once
	stopCh   chan struct{}
}

// New constructs the Daemon's component graph but does not start any
// goroutines. Call Run to start the daemon.
//
// New validates the supplied Config (identity must be non-nil, ConfigDir
// must be non-empty), then builds the CRDT StateStore, on-disk hooks queue,
// dispatcher with a single LogSurface, the action gate, and the
// SocketServer. None of these touch the network. The libp2p Host and
// GossipSub are deferred to Run because they need the ctx the caller will
// pass and because Host construction is the most likely thing to fail at
// startup — surfacing that failure from Run (with the rest of the wiring
// already torn down) is cleaner than failing in New and leaving the caller
// to clean up the dispatcher's file descriptors.
func New(cfg Config) (*Daemon, error) {
	if cfg.Identity == nil {
		return nil, errors.New("daemon: Config.Identity is required")
	}
	if cfg.ConfigDir == "" {
		return nil, errors.New("daemon: Config.ConfigDir is required")
	}
	if cfg.SocketPath == "" {
		cfg.SocketPath = filepath.Join(cfg.ConfigDir, "agenthive.sock")
	}
	if cfg.LogPath == "" {
		cfg.LogPath = filepath.Join(cfg.ConfigDir, "agenthive.log.jsonl")
	}

	// PeerID derives deterministically from the identity key; we use it as
	// the HLC's peer ID so timestamps minted on different daemons can be
	// ordered consistently across the mesh.
	pid, err := peer.IDFromPrivateKey(cfg.Identity)
	if err != nil {
		return nil, fmt.Errorf("daemon: derive peer ID from identity: %w", err)
	}
	state := crdt.NewStateStore(pid.String())

	// Best-effort: if a previous run wrote state.json, pull it in. A missing
	// file is fine. A corrupt file is fatal — better to refuse to start than
	// silently lose state.
	if err := state.LoadFromFile(filepath.Join(cfg.ConfigDir, stateFileName)); err != nil {
		return nil, fmt.Errorf("daemon: load persisted state: %w", err)
	}

	// Build the dispatcher with a single log surface so every action and
	// notification at minimum lands on disk. Future surfaces (tmux, desktop,
	// Slack) plug in via Dispatcher.Add — out of scope for this push per the
	// plan.
	logSurface, err := dispatch.NewLogSurface(cfg.LogPath)
	if err != nil {
		return nil, fmt.Errorf("daemon: open log surface: %w", err)
	}
	dispatcher := dispatch.New([]dispatch.Surface{logSurface})

	// The hooks queue is the rendezvous between socket requests and any
	// surface that can produce a decision. It is rooted under ConfigDir so
	// pending/response files survive a daemon crash.
	queueDir := filepath.Join(cfg.ConfigDir, "queue")
	queue, err := hooks.NewQueue(queueDir)
	if err != nil {
		// Best-effort: don't leak the log file descriptor if we bail early.
		_ = dispatcher.Close()
		return nil, fmt.Errorf("daemon: create hooks queue: %w", err)
	}
	gate := hooks.NewGate(queue, dispatcher)
	socket := NewSocketServer(cfg.SocketPath, gate)

	return &Daemon{
		cfg:        cfg,
		state:      state,
		dispatcher: dispatcher,
		gate:       gate,
		socket:     socket,
		stopCh:     make(chan struct{}),
	}, nil
}

// Host returns the libp2p host, or nil if Run has not been called yet (or
// has already returned). The accessor is read-locked so callers in other
// goroutines see a consistent view; the field itself is set exactly once by
// Run and cleared exactly once at shutdown.
func (d *Daemon) Host() host.Host {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.host
}

// State returns the CRDT StateStore. Always non-nil after New.
func (d *Daemon) State() *crdt.StateStore { return d.state }

// Multiaddrs returns the host's listen addresses as /p2p/<peerid>-suffixed
// strings, suitable for sharing with peers. Returns nil before Run has built
// the host.
func (d *Daemon) Multiaddrs() []string {
	d.mu.RLock()
	h := d.host
	d.mu.RUnlock()
	if h == nil {
		return nil
	}
	return transport.MultiaddrsFor(h)
}

// Run blocks until ctx is done or Stop is called, then tears down every
// component in reverse order and persists state to <configDir>/state.json.
//
// Workflow (matches plan L4):
//  1. Build the libp2p Host via transport.New(ctx, cfg).
//  2. Build GossipSub: NewGossipSub(ctx, host), Join(TopicState),
//     Subscribe().
//  3. Register stream handlers on the Host for the four protocol IDs.
//  4. Start mDNS via discovery.StartMDNS with a callback that adds
//     discovered peers to the StateStore and to the libp2p peerstore.
//  5. Start the SocketServer goroutine.
//  6. Start a goroutine that applies incoming StateDelta messages from the
//     GossipSub topic to the StateStore.
//  7. Start a goroutine that periodically publishes deltas of the
//     StateStore via the topic (debounced 200ms, max once per 2s active).
//  8. Connect to all peers in StateStore.ListPeers() (best-effort).
//  9. Block on ctx.Done(). On cancel, close everything in reverse order
//     and persist state to state.json.
func (d *Daemon) Run(ctx context.Context) error {
	// Derive a cancel-able child so Stop can shut Run down even when the
	// caller's ctx is still live.
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	go func() {
		select {
		case <-d.stopCh:
			cancelRun()
		case <-runCtx.Done():
		}
	}()

	// Step 1: build the libp2p Host.
	h, err := transport.New(runCtx, transport.Config{
		Identity:    d.cfg.Identity,
		ListenAddrs: d.cfg.ListenAddrs,
	})
	if err != nil {
		return fmt.Errorf("daemon: build host: %w", err)
	}

	// Step 2: build GossipSub and join the agenthive state topic.
	gs, err := pubsub.NewGossipSub(runCtx, h)
	if err != nil {
		_ = h.Close()
		return fmt.Errorf("daemon: build gossipsub: %w", err)
	}
	topic, err := gs.Join(protocols.TopicState)
	if err != nil {
		_ = h.Close()
		return fmt.Errorf("daemon: join %s: %w", protocols.TopicState, err)
	}
	sub, err := topic.Subscribe()
	if err != nil {
		_ = topic.Close()
		_ = h.Close()
		return fmt.Errorf("daemon: subscribe %s: %w", protocols.TopicState, err)
	}

	d.mu.Lock()
	d.host = h
	d.gossipsub = gs
	d.topic = topic
	d.sub = sub
	d.mu.Unlock()

	// Step 3: register stream handlers. The host routes incoming streams by
	// protocol ID to these closures; each one decodes a single framed JSON
	// message, dispatches it through the appropriate domain object, and
	// returns. Errors are logged but never propagated — a misbehaving peer
	// must not be able to kill the daemon.
	h.SetStreamHandler(protocol.ID(protocols.ProtoActionRequest), d.handleActionRequestStream)
	h.SetStreamHandler(protocol.ID(protocols.ProtoActionResponse), d.handleActionResponseStream)
	h.SetStreamHandler(protocol.ID(protocols.ProtoNotification), d.handleNotificationStream)
	h.SetStreamHandler(protocol.ID(protocols.ProtoPeerAnnounce), d.handlePeerAnnounceStream)

	// Step 4: mDNS discovery. Discovered peers go into the libp2p peerstore
	// so subsequent dials work, and into the CRDT StateStore so other peers
	// learn about them via gossip.
	mdnsStop, err := discovery.StartMDNS(h, d.onMDNSPeer)
	if err != nil {
		// mDNS is best-effort; some platforms (containers, CI) block
		// multicast. Log and keep running rather than failing startup.
		log.Printf("daemon: mDNS startup failed (continuing without LAN discovery): %v", err)
	} else {
		d.mu.Lock()
		d.mdnsStop = mdnsStop
		d.mu.Unlock()
	}

	// Step 5-7: launch the long-running goroutines. We use a WaitGroup so
	// the shutdown sequence can wait for every goroutine to settle before we
	// close the host (which is what unblocks them).
	var wg sync.WaitGroup
	wg.Add(3)
	socketErrCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		// SocketServer.Run returns nil on a clean ctx-cancel.
		if err := d.socket.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			socketErrCh <- err
		}
	}()
	go func() {
		defer wg.Done()
		d.runSubscriptionLoop(runCtx)
	}()
	go func() {
		defer wg.Done()
		d.runPublishLoop(runCtx)
	}()

	// Step 8: dial any peers we already know about from persisted state.
	// Best-effort — a peer being offline must not stop the daemon.
	d.dialKnownPeers(runCtx)

	// Step 9: block.
	<-runCtx.Done()

	// Teardown in reverse order: cancel the subscription first so the
	// loop goroutine returns, then close the topic, then close the
	// gossipsub publish surface (implicitly, by closing the host), then
	// close mDNS, then close the host. Goroutines are joined under the
	// waitgroup so the persistence write below happens after every
	// component has fully released its resources.
	sub.Cancel()
	_ = topic.Close()
	d.mu.Lock()
	mdnsStopFn := d.mdnsStop
	d.mdnsStop = nil
	d.mu.Unlock()
	if mdnsStopFn != nil {
		_ = mdnsStopFn()
	}
	_ = h.Close()

	wg.Wait()

	d.mu.Lock()
	d.host = nil
	d.gossipsub = nil
	d.topic = nil
	d.sub = nil
	d.mu.Unlock()

	// Persist state to state.json. This is the one shutdown step the plan
	// explicitly calls out: a restart must not lose what we learned.
	if err := d.state.SaveToFile(filepath.Join(d.cfg.ConfigDir, stateFileName)); err != nil {
		log.Printf("daemon: persist state.json: %v", err)
	}

	// Close the dispatcher last so any late-arriving log lines from the
	// goroutines above (in particular the subscription loop) made it to
	// disk before the file descriptor closes.
	if err := d.dispatcher.Close(); err != nil {
		log.Printf("daemon: close dispatcher: %v", err)
	}

	// If the socket goroutine produced a non-cancellation error, surface it.
	select {
	case err := <-socketErrCh:
		return err
	default:
	}

	// A caller's ctx.Done() is the expected path; report nil for that case
	// so the caller can tell "I cancelled" from "the daemon crashed".
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil
	}
	if ctx.Err() == nil {
		// Run returned due to Stop — also a clean exit.
		return nil
	}
	return ctx.Err()
}

// Stop signals the Run loop to begin shutting down. Safe to call multiple
// times concurrently. Stop returns immediately; the actual shutdown happens
// inside Run.
func (d *Daemon) Stop() error {
	d.stopOnce.Do(func() { close(d.stopCh) })
	return nil
}

// onMDNSPeer is the discovery callback. It records the peer in both the
// libp2p peerstore (so future dials work) and the CRDT StateStore (so other
// peers learn about it via gossip).
func (d *Daemon) onMDNSPeer(pi peer.AddrInfo) {
	d.mu.RLock()
	h := d.host
	d.mu.RUnlock()
	if h == nil {
		return
	}
	// Ignore ourself — mDNS may echo our own announcements on some
	// platforms.
	if pi.ID == h.ID() {
		return
	}

	// Tell the libp2p peerstore about the addresses so subsequent dials work.
	h.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.RecentlyConnectedAddrTTL)

	// Record in the CRDT for gossip propagation.
	addrs := make([]string, 0, len(pi.Addrs))
	for _, a := range pi.Addrs {
		addrs = append(addrs, a.String())
	}
	var addrStr string
	if len(addrs) > 0 {
		addrStr = addrs[0]
	}
	d.state.SetPeer(pi.ID.String(), crdt.PeerInfo{
		Status:   "discovered",
		Addr:     addrStr,
		LinkType: "mdns",
		LastSeen: time.Now().UTC().Format(time.RFC3339),
	})
}

// runSubscriptionLoop pulls StateDelta messages off the GossipSub
// subscription and merges them into the local StateStore. Plan L4 step 7.
//
// A nil sub.Next error after ctx.Done is the normal shutdown signal.
func (d *Daemon) runSubscriptionLoop(ctx context.Context) {
	d.mu.RLock()
	sub := d.sub
	hostID := d.host.ID()
	d.mu.RUnlock()
	if sub == nil {
		return
	}

	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			// Subscription was cancelled (clean shutdown) or ctx fired.
			return
		}
		// Skip our own messages — pubsub delivers them by default.
		if msg.ReceivedFrom == hostID {
			continue
		}

		var delta protocols.StateDelta
		if err := json.Unmarshal(msg.Data, &delta); err != nil {
			log.Printf("daemon: subscription: decode StateDelta: %v", err)
			continue
		}
		d.applyDelta(delta)
	}
}

// applyDelta merges a remote StateDelta into the local StateStore. The wire
// format carries each CRDT map as a JSON blob, so we unmarshal into fresh
// LWWMaps and ask the StateStore to merge them; that preserves the original
// HLC timestamps from the remote peer so last-writer-wins works correctly.
func (d *Daemon) applyDelta(delta protocols.StateDelta) {
	peersMap := crdt.NewLWWMap[crdt.PeerInfo]()
	routesMap := crdt.NewLWWMap[crdt.RouteRule]()
	configMap := crdt.NewLWWMap[crdt.ConfigEntry]()

	if len(delta.Peers) > 0 {
		if err := json.Unmarshal(delta.Peers, peersMap); err != nil {
			log.Printf("daemon: applyDelta: peers: %v", err)
			return
		}
	}
	if len(delta.Routes) > 0 {
		if err := json.Unmarshal(delta.Routes, routesMap); err != nil {
			log.Printf("daemon: applyDelta: routes: %v", err)
			return
		}
	}
	if len(delta.Config) > 0 {
		if err := json.Unmarshal(delta.Config, configMap); err != nil {
			log.Printf("daemon: applyDelta: config: %v", err)
			return
		}
	}

	d.state.MergeMaps(peersMap, routesMap, configMap)
}

// runPublishLoop periodically publishes a snapshot of the local state to
// the GossipSub topic. Plan L4 step 8: debounced 200ms, max once per 2s.
//
// We use a "publish-on-tick" strategy with two timers:
//   - The debounce timer fires 200ms after the loop wakes up, ensuring a
//     burst of local mutations coalesces into one publish.
//   - The minInterval timer enforces the rate ceiling: after publishing,
//     the next publish cannot happen until 2s has elapsed.
//
// The loop publishes a full snapshot rather than a strict delta. A genuine
// delta requires per-peer "since" timestamps which we do not yet track — the
// LWWMap.Delta API takes a since timestamp but we have no per-recipient
// bookkeeping. Publishing the full state is correct (CRDT merge is
// idempotent) and over-the-wire cost is bounded by the small state size of
// a personal-scale mesh. A follow-up push can tighten this once we have
// per-peer ACK tracking.
func (d *Daemon) runPublishLoop(ctx context.Context) {
	d.mu.RLock()
	topic := d.topic
	host := d.host
	d.mu.RUnlock()
	if topic == nil || host == nil {
		return
	}
	from := host.ID().String()

	// Tick the debounce window. We publish every debounce window so newly-
	// mutated state propagates quickly; the rate ceiling stops us if we are
	// firing too often.
	ticker := time.NewTicker(publishDebounce)
	defer ticker.Stop()

	var lastPublish time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !lastPublish.IsZero() && time.Since(lastPublish) < publishMinInterval-publishDebounce {
				// Within the rate-ceiling window. We still tick at the
				// debounce cadence so a long quiescent period followed by a
				// mutation is picked up promptly.
				if time.Since(lastPublish) < publishMinInterval {
					// Only enforce if there was a publish recently.
					// The combination of ticker + this check yields a
					// schedule where publishes happen no more often than
					// publishMinInterval but always at least every
					// publishDebounce-ish when there's something to send.
					continue
				}
			}
			if err := d.publishState(ctx, topic, from); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("daemon: publish state: %v", err)
				continue
			}
			lastPublish = time.Now()
		}
	}
}

// publishState marshals the current CRDT snapshot into a StateDelta and
// publishes it via the topic.
func (d *Daemon) publishState(ctx context.Context, topic *pubsub.Topic, from string) error {
	peersBytes, err := json.Marshal(d.state.PeersMap())
	if err != nil {
		return fmt.Errorf("marshal peers: %w", err)
	}
	routesBytes, err := json.Marshal(d.state.RoutesMap())
	if err != nil {
		return fmt.Errorf("marshal routes: %w", err)
	}
	configBytes, err := json.Marshal(d.state.ConfigMap())
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	delta := protocols.StateDelta{
		From:   from,
		Peers:  peersBytes,
		Routes: routesBytes,
		Config: configBytes,
	}
	body, err := json.Marshal(delta)
	if err != nil {
		return fmt.Errorf("marshal delta: %w", err)
	}
	return topic.Publish(ctx, body)
}

// dialKnownPeers iterates the StateStore's known peers and asks the host to
// connect to each. Best-effort: failures are logged but do not stop the
// daemon. The dial is bounded by a short context so a long-running connect
// to a dead peer cannot block startup.
func (d *Daemon) dialKnownPeers(ctx context.Context) {
	d.mu.RLock()
	h := d.host
	d.mu.RUnlock()
	if h == nil {
		return
	}

	for id, info := range d.state.ListPeers() {
		pid, err := peer.Decode(id)
		if err != nil {
			continue
		}
		if pid == h.ID() {
			continue
		}
		if info.Addr == "" {
			continue
		}
		mAddr, err := ma.NewMultiaddr(info.Addr)
		if err != nil {
			continue
		}
		dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = h.Connect(dialCtx, peer.AddrInfo{ID: pid, Addrs: []ma.Multiaddr{mAddr}})
		cancel()
		if err != nil {
			log.Printf("daemon: dial %s: %v", id, err)
		}
	}
}

// handleActionRequestStream decodes a framed ActionRequest, runs it through
// the local gate, and writes the framed ActionResponse back.
func (d *Daemon) handleActionRequestStream(s network.Stream) {
	defer s.Close()
	_ = s.SetReadDeadline(time.Now().Add(streamReadTimeout))

	var req protocols.ActionRequest
	if err := protocols.ReadFramed(s, &req); err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("daemon: ProtoActionRequest: read: %v", err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	resp, err := d.gate.Handle(ctx, req)
	if err != nil {
		log.Printf("daemon: ProtoActionRequest: gate: %v", err)
		return
	}
	if err := protocols.WriteFramed(s, resp); err != nil {
		log.Printf("daemon: ProtoActionRequest: write: %v", err)
	}
}

// handleActionResponseStream decodes a framed ActionResponse and writes it
// into the local queue so a waiting Gate.Handle on this daemon can pick it
// up. This is the path that lets a remote peer's decision unblock a local
// hook.
func (d *Daemon) handleActionResponseStream(s network.Stream) {
	defer s.Close()
	_ = s.SetReadDeadline(time.Now().Add(streamReadTimeout))

	var resp protocols.ActionResponse
	if err := protocols.ReadFramed(s, &resp); err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("daemon: ProtoActionResponse: read: %v", err)
		}
		return
	}
	// We rely on hooks.Queue's WriteResponse via the gate's queue. Reach in
	// through the gate-level abstraction via a separate accessor: the gate
	// itself does not expose its queue, so we route through a thin helper.
	if err := d.deliverResponse(resp); err != nil {
		log.Printf("daemon: ProtoActionResponse: deliver: %v", err)
	}
}

// handleNotificationStream decodes a framed Notification and fans it out to
// the local dispatcher.
func (d *Daemon) handleNotificationStream(s network.Stream) {
	defer s.Close()
	_ = s.SetReadDeadline(time.Now().Add(streamReadTimeout))

	var n protocols.Notification
	if err := protocols.ReadFramed(s, &n); err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("daemon: ProtoNotification: read: %v", err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if errs := d.dispatcher.Dispatch(ctx, n); len(errs) > 0 {
		for _, err := range errs {
			if err == nil {
				continue
			}
			log.Printf("daemon: ProtoNotification: dispatch: %v", err)
		}
	}
}

// handlePeerAnnounceStream decodes a framed PeerAnnounce and records the
// peer's addresses in both the libp2p peerstore and the CRDT StateStore.
func (d *Daemon) handlePeerAnnounceStream(s network.Stream) {
	defer s.Close()
	_ = s.SetReadDeadline(time.Now().Add(streamReadTimeout))

	var ann protocols.PeerAnnounce
	if err := protocols.ReadFramed(s, &ann); err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("daemon: ProtoPeerAnnounce: read: %v", err)
		}
		return
	}
	d.mu.RLock()
	h := d.host
	d.mu.RUnlock()
	if h == nil {
		return
	}
	pid, err := peer.Decode(ann.PeerID)
	if err != nil {
		log.Printf("daemon: ProtoPeerAnnounce: invalid peer ID %q: %v", ann.PeerID, err)
		return
	}
	addrs := make([]ma.Multiaddr, 0, len(ann.Multiaddrs))
	for _, s := range ann.Multiaddrs {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		addrs = append(addrs, a)
	}
	h.Peerstore().AddAddrs(pid, addrs, peerstore.RecentlyConnectedAddrTTL)
	primary := ""
	if len(ann.Multiaddrs) > 0 {
		primary = ann.Multiaddrs[0]
	}
	d.state.SetPeer(ann.PeerID, crdt.PeerInfo{
		Status:   "announced",
		Addr:     primary,
		LinkType: "p2p",
		LastSeen: ann.Timestamp.UTC().Format(time.RFC3339),
	})
}

// deliverResponse persists a remote ActionResponse so the local Gate.Handle
// can pick it up. We re-derive the queue dir from the daemon's ConfigDir
// (same convention as New).
func (d *Daemon) deliverResponse(resp protocols.ActionResponse) error {
	queueDir := filepath.Join(d.cfg.ConfigDir, "queue")
	q, err := hooks.NewQueue(queueDir)
	if err != nil {
		return err
	}
	return q.WriteResponse(resp)
}
