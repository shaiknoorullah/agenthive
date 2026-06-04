package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
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

// parseSelector parses the v0.1.0 selector grammar into a RouteMatch.
//
// Grammar:
//
//	clause   := agent:<v> | project:<v> | session:<v> | window:<v>
//	          | pane:<v> | source:<v> | priority:<v>
//	          | default | *
//	selector := clause ("," clause)*
//
// `default` and `*` are wildcards: they parse to a zero-valued RouteMatch.
// Mixing wildcards with concrete clauses is rejected, because the result
// would be ambiguous (does the wildcard widen the match or narrow it?).
func parseSelector(s string) (crdt.RouteMatch, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return crdt.RouteMatch{}, errors.New("selector is empty")
	}

	clauses := strings.Split(s, ",")
	var match crdt.RouteMatch
	sawWildcard := false
	sawConcrete := false

	for _, raw := range clauses {
		clause := strings.TrimSpace(raw)
		if clause == "" {
			return crdt.RouteMatch{}, fmt.Errorf("selector has empty clause in %q", s)
		}

		switch clause {
		case "*", "default":
			sawWildcard = true
			continue
		}

		key, value, ok := strings.Cut(clause, ":")
		if !ok {
			return crdt.RouteMatch{}, fmt.Errorf("selector clause %q is not key:value", clause)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			return crdt.RouteMatch{}, fmt.Errorf("selector clause %q has empty value", clause)
		}

		switch key {
		case "agent":
			match.Agent = value
		case "project":
			match.Project = value
		case "session":
			match.Session = value
		case "window":
			match.Window = value
		case "pane":
			match.Pane = value
		case "source":
			match.Source = value
		case "priority":
			match.Priority = value
		default:
			return crdt.RouteMatch{}, fmt.Errorf("selector clause %q has unknown key %q", clause, key)
		}
		sawConcrete = true
	}

	if sawWildcard && sawConcrete {
		return crdt.RouteMatch{}, fmt.Errorf("selector %q mixes wildcard with concrete clauses", s)
	}
	return match, nil
}

// renderSelector turns a RouteMatch back into the selector grammar so
// `routes list` round-trips with `routes add`. An empty RouteMatch renders
// as `*` (the wildcard form), which is the form `parseSelector("*")` parses
// to.
func renderSelector(m crdt.RouteMatch) string {
	type kv struct{ k, v string }
	clauses := []kv{
		{"agent", m.Agent},
		{"project", m.Project},
		{"session", m.Session},
		{"window", m.Window},
		{"pane", m.Pane},
		{"source", m.Source},
		{"priority", m.Priority},
	}
	parts := make([]string, 0, len(clauses))
	for _, c := range clauses {
		if c.v == "" {
			continue
		}
		parts = append(parts, c.k+":"+c.v)
	}
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, ",")
}

// parseTargets splits a comma-separated target list, trims whitespace, and
// rejects empty input. Preserves order so `routes list` is deterministic.
func parseTargets(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("targets list is empty")
	}
	raws := strings.Split(s, ",")
	out := make([]string, 0, len(raws))
	for _, t := range raws {
		t = strings.TrimSpace(t)
		if t == "" {
			return nil, fmt.Errorf("targets list %q contains an empty entry", s)
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, errors.New("targets list is empty")
	}
	return out, nil
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
			id := strings.TrimSpace(args[0])
			if id == "" {
				return errors.New("route id is empty")
			}
			match, err := parseSelector(args[1])
			if err != nil {
				return fmt.Errorf("parse selector: %w", err)
			}
			targets, err := parseTargets(args[2])
			if err != nil {
				return fmt.Errorf("parse targets: %w", err)
			}

			state, err := loadPeerState()
			if err != nil {
				return err
			}
			state.SetRoute(id, crdt.RouteRule{
				Match:   match,
				Targets: targets,
			})
			if err := savePeerState(state); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added route %s\n", id)
			return nil
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
			state, err := loadPeerState()
			if err != nil {
				return err
			}
			routes := state.ListRoutes()
			ids := make([]string, 0, len(routes))
			for id := range routes {
				ids = append(ids, id)
			}
			sort.Strings(ids)

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ID\tSELECTOR\tTARGETS")
			for _, id := range ids {
				rule := routes[id]
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n",
					id,
					renderSelector(rule.Match),
					strings.Join(rule.Targets, ","),
				)
			}
			return w.Flush()
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
			id := strings.TrimSpace(args[0])
			if id == "" {
				return errors.New("route id is empty")
			}

			state, err := loadPeerState()
			if err != nil {
				return err
			}
			if _, ok := state.GetRoute(id); !ok {
				return fmt.Errorf("route %q does not exist", id)
			}
			state.DeleteRoute(id)
			if err := savePeerState(state); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "deleted route %s\n", id)
			return nil
		},
	}
}
