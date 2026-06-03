package main

import (
	"github.com/spf13/cobra"
)

// newRespondCmd returns `agenthive respond <action-id> <allow|deny>`, the
// manual override path: it writes an ActionResponse file into the gate's
// queue directory, racing any out-of-band decision.
func newRespondCmd() *cobra.Command {
	panic("not implemented: agenthive.newRespondCmd")
}
