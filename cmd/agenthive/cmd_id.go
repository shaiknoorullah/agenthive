package main

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	"github.com/shaiknoorullah/agenthive/internal/identity"
)

// newIDCmd returns `agenthive id`, which loads the persisted identity and
// prints every multiaddr the host would listen on, each suffixed with the
// libp2p /p2p/<peerid> component so peers can paste them directly into
// `agenthive peers add`.
//
// The command does not start a host — it computes the listen addresses from
// the persisted key alone. This keeps `id` fast and side-effect free (it
// never binds a port, never opens a socket) so it can be scripted safely in
// shell pipelines.
func newIDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "id",
		Short: "Print this daemon's multiaddrs",
		Long: "Loads the persisted identity from <config-dir>/identity.key, " +
			"derives the PeerID, and prints one /p2p/<peerid>-suffixed line " +
			"per default listen address. Run `agenthive init` first.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			priv, err := identity.Load(configDir)
			if err != nil {
				return fmt.Errorf("load identity: %w", err)
			}
			pid, err := peer.IDFromPrivateKey(priv)
			if err != nil {
				return fmt.Errorf("derive peer id: %w", err)
			}

			// Default listen multiaddrs mirror internal/transport's defaults
			// but with the /p2p/<peerid> component appended so the output
			// is paste-ready. We don't bind a host here; the addresses are
			// the templates a real start would listen on.
			templates := []string{
				"/ip4/0.0.0.0/tcp/0",
				"/ip4/0.0.0.0/udp/0/quic-v1",
				"/ip6/::/tcp/0",
				"/ip6/::/udp/0/quic-v1",
			}
			for _, t := range templates {
				// Cobra's OutOrStdout is os.Stdout in normal use; writes to it
				// can only fail on closed-pipe conditions where there is no
				// useful action to take. Drop the error explicitly so errcheck
				// is happy without losing information.
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s/p2p/%s\n", t, pid)
			}
			return nil
		},
	}
}
