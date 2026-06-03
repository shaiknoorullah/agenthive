// Package daemon coordinates the libp2p Host, CRDT StateStore, GossipSub
// topic, stream handlers, mDNS discovery, dispatcher, action gate, and the
// Unix-socket hook IPC into a single Run loop.
package daemon

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
	"github.com/shaiknoorullah/agenthive/internal/dispatch"
	"github.com/shaiknoorullah/agenthive/internal/hooks"
)

// Config is the daemon's startup configuration.
type Config struct {
	ConfigDir   string
	Identity    crypto.PrivKey
	ListenAddrs []string
	LogPath     string
	SocketPath  string
}

// Daemon is the long-running coordinator. New constructs and wires every
// subsystem; Run blocks until ctx is done.
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
}

// New constructs the Daemon's component graph but does not start any
// goroutines. Call Run to start the daemon.
func New(cfg Config) (*Daemon, error) {
	panic("not implemented: daemon.New")
}

// Run blocks until ctx is done, then tears down every component in reverse
// order and persists state to <configDir>/state.json.
//
// Workflow (see plan L4):
//  1. Build the libp2p Host via transport.New(ctx, cfg).
//  2. Build the CRDT StateStore; load <configDir>/state.json if present.
//  3. Build GossipSub: pubsub.NewGossipSub(ctx, host), Join(TopicState),
//     Subscribe().
//  4. Register stream handlers on the Host for ProtoActionRequest,
//     ProtoActionResponse, ProtoNotification, ProtoPeerAnnounce.
//  5. Start mDNS via discovery.StartMDNS with a callback that adds
//     discovered peers to the StateStore.
//  6. Start the SocketServer goroutine.
//  7. Start a goroutine that applies incoming StateDelta messages from the
//     GossipSub topic to the StateStore.
//  8. Start a goroutine that periodically publishes deltas of the
//     StateStore via the topic (debounced 200ms, max once per 2s when
//     active).
//  9. Connect to all peers in StateStore.ListPeers().
//  10. Block on ctx.Done(). On cancel, close everything in reverse order.
//      Persist state to state.json.
func (d *Daemon) Run(ctx context.Context) error {
	panic("not implemented: daemon.Daemon.Run")
}

// Stop signals the Run loop to begin shutting down. Safe to call multiple
// times.
func (d *Daemon) Stop() error {
	panic("not implemented: daemon.Daemon.Stop")
}
