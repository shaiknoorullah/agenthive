package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shaiknoorullah/agenthive/internal/hooks"
	"github.com/shaiknoorullah/agenthive/internal/protocols"
)

// newRespondCmd returns `agenthive respond <action-id> <allow|deny>`, the
// manual override path: it writes an ActionResponse file into the gate's
// queue directory, racing any out-of-band decision.
//
// This is the escape hatch for when the user wants to approve or block a
// pending action from another shell without involving any surface. The
// queue's O_EXCL semantics guarantee exactly one decision wins for a given
// action ID even when respond races with another source.
func newRespondCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "respond <action-id> <allow|deny>",
		Short: "Write a manual decision into the action queue",
		Long: "Drops an action response file (queue/<id>.response) so a " +
			"pending action gate releases with the supplied decision. The " +
			"daemon does not need to be running; the next daemon start (or " +
			"a currently-waiting Gate.Handle) will pick up the response.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			actionID, decision := args[0], args[1]
			if actionID == "" {
				return fmt.Errorf("action-id must not be empty")
			}
			if decision != "allow" && decision != "deny" {
				return fmt.Errorf("decision must be %q or %q, got %q", "allow", "deny", decision)
			}

			queueDir := filepath.Join(configDir, "queue")
			q, err := hooks.NewQueue(queueDir)
			if err != nil {
				return fmt.Errorf("open queue: %w", err)
			}
			if err := q.WriteResponse(protocols.ActionResponse{
				ActionID:  actionID,
				Decision:  decision,
				DecidedBy: "cli",
			}); err != nil {
				return fmt.Errorf("write response: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "responded to %s: %s\n", actionID, decision)
			return nil
		},
	}
}
