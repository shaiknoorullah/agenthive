package main

import (
	"github.com/spf13/cobra"
)

// newTUICmd returns the `agenthive tui` subcommand. The TUI connects to the
// local daemon's Unix socket, snapshots peers/routes/actions/logs, launches
// the bubbletea App, and periodically polls for updates.
func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the agenthive terminal UI",
		Long: "Connects to the local daemon socket, snapshots state, and " +
			"launches the bubbletea terminal UI. Exits 1 if the daemon is " +
			"not running.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			panic("not implemented: cmd.newTUICmd.RunE")
		},
	}
}
