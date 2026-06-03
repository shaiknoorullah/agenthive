package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/shaiknoorullah/agenthive/internal/daemon"
	"github.com/shaiknoorullah/agenthive/internal/identity"
)

// newStartCmd returns `agenthive start`, which constructs a daemon.Daemon
// from the on-disk config and blocks on its Run loop until SIGINT/SIGTERM.
//
// Signals are caught with signal.NotifyContext so a clean Ctrl-C teardown
// fully drains the daemon's goroutines and persists state to state.json
// before this process exits — the daemon's Run loop already does the heavy
// lifting; the CLI just wires the signal source up to the cancel.
func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the agenthive daemon (blocks)",
		Long: "Loads the persisted identity, constructs the libp2p host, " +
			"GossipSub topic, stream handlers, mDNS discovery, action gate, " +
			"and Unix-socket hook IPC, then blocks until SIGINT or SIGTERM.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			priv, err := identity.Load(configDir)
			if err != nil {
				return fmt.Errorf("load identity (did you run `agenthive init`?): %w", err)
			}

			d, err := daemon.New(daemon.Config{
				ConfigDir: configDir,
				Identity:  priv,
			})
			if err != nil {
				return fmt.Errorf("construct daemon: %w", err)
			}

			// signal.NotifyContext returns a context that fires on SIGINT
			// or SIGTERM; the daemon's Run loop treats ctx.Done as the
			// shutdown signal and tears everything down cleanly.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if ctx.Err() != nil {
				// In tests cmd.Context() can already be cancelled.
				ctx = context.Background()
			}

			fmt.Fprintf(cmd.OutOrStdout(), "agenthive daemon starting (config=%s)\n", configDir)
			if err := d.Run(ctx); err != nil {
				return fmt.Errorf("daemon run: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "agenthive daemon stopped cleanly")
			return nil
		},
	}
}
