package main

import (
	"github.com/spf13/cobra"
)

// newRoutesCmd returns the `agenthive routes` command tree with `add`,
// `list`, and `del` subcommands. Routes are persisted to the local CRDT
// state file, the same one peers add / list operates on.
func newRoutesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "routes",
		Short: "Manage routing rules",
		Long: "Routing rules determine which peers receive a notification " +
			"based on its metadata. Subcommands edit the local CRDT state; " +
			"the running daemon picks changes up via CRDT sync.",
	}
	cmd.AddCommand(newRoutesAddCmd())
	cmd.AddCommand(newRoutesListCmd())
	cmd.AddCommand(newRoutesDelCmd())
	return cmd
}

// newRoutesAddCmd returns `agenthive routes add <id> <selector> <target1,target2,...>`.
func newRoutesAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <id> <selector> <targets>",
		Short: "Add a route rule",
		Long: "Parses the selector grammar (agent:/project:/session:/window:/" +
			"pane:/source:/priority:/default/* separated by ',' for AND) and " +
			"writes a RouteRule into the local CRDT state. Targets is a " +
			"comma-separated list of peer names; the literal ALL fans out to " +
			"every peer.",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			panic("not implemented: cmd.newRoutesAddCmd.RunE")
		},
	}
}

// newRoutesListCmd returns `agenthive routes list`.
func newRoutesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List route rules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			panic("not implemented: cmd.newRoutesListCmd.RunE")
		},
	}
}

// newRoutesDelCmd returns `agenthive routes del <id>`.
func newRoutesDelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "del <id>",
		Short: "Delete a route rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			panic("not implemented: cmd.newRoutesDelCmd.RunE")
		},
	}
}
