package main

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/shaiknoorullah/agenthive/internal/crdt"
)

// loadRoutesFor returns the routes persisted in the StateStore file for a
// freshly-bootstrapped config directory. Tests use it to assert what the
// `routes add` / `routes del` commands actually wrote to disk, independent of
// what `routes list` prints.
func loadRoutesFor(t *testing.T, dir string) map[string]crdt.RouteRule {
	t.Helper()
	state := crdt.NewStateStore("test")
	if err := state.LoadFromFile(filepath.Join(dir, peersStateFile)); err != nil {
		t.Fatalf("load state: %v", err)
	}
	return state.ListRoutes()
}

// TestRoutes_AddPersistsSelectorAndTargets verifies that a single selector
// clause with multiple targets round-trips into the CRDT state file with the
// correct fields populated.
func TestRoutes_AddPersistsSelectorAndTargets(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}

	_, _, err := runRoot(t, []string{
		"--config-dir", dir, "routes", "add",
		"critical-phone",
		"priority:critical,source:claude-code",
		"phone,laptop",
	}, "")
	if err != nil {
		t.Fatalf("routes add: %v", err)
	}

	routes := loadRoutesFor(t, dir)
	got, ok := routes["critical-phone"]
	if !ok {
		t.Fatalf("route critical-phone missing; have %v", routes)
	}
	want := crdt.RouteRule{
		Match: crdt.RouteMatch{
			Priority: "critical",
			Source:   "claude-code",
		},
		Targets: []string{"phone", "laptop"},
	}
	if !reflect.DeepEqual(got.Match, want.Match) {
		t.Fatalf("match mismatch: got %+v want %+v", got.Match, want.Match)
	}
	if !reflect.DeepEqual(got.Targets, want.Targets) {
		t.Fatalf("targets mismatch: got %v want %v", got.Targets, want.Targets)
	}
}

// TestRoutes_AddWildcardSelectors verifies that the wildcard selectors `*`
// and `default` parse to an empty RouteMatch.
func TestRoutes_AddWildcardSelectors(t *testing.T) {
	for _, sel := range []string{"*", "default"} {
		t.Run(sel, func(t *testing.T) {
			dir := t.TempDir()
			if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
				t.Fatalf("init: %v", err)
			}
			if _, _, err := runRoot(t, []string{
				"--config-dir", dir, "routes", "add",
				"any", sel, "ALL",
			}, ""); err != nil {
				t.Fatalf("routes add: %v", err)
			}
			routes := loadRoutesFor(t, dir)
			got := routes["any"]
			if got.Match != (crdt.RouteMatch{}) {
				t.Fatalf("wildcard %q should yield empty RouteMatch, got %+v", sel, got.Match)
			}
			if !reflect.DeepEqual(got.Targets, []string{"ALL"}) {
				t.Fatalf("targets: got %v want [ALL]", got.Targets)
			}
		})
	}
}

// TestRoutes_AddAllSelectorClauses covers every clause in the v0.1.0 grammar
// so a regression in the parser surfaces immediately.
func TestRoutes_AddAllSelectorClauses(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	selector := "agent:claude,project:agenthive,session:s1,window:0,pane:%1,source:claude-code,priority:critical"
	if _, _, err := runRoot(t, []string{
		"--config-dir", dir, "routes", "add",
		"full", selector, "phone",
	}, ""); err != nil {
		t.Fatalf("routes add: %v", err)
	}
	routes := loadRoutesFor(t, dir)
	got := routes["full"].Match
	want := crdt.RouteMatch{
		Agent:    "claude",
		Project:  "agenthive",
		Session:  "s1",
		Window:   "0",
		Pane:     "%1",
		Source:   "claude-code",
		Priority: "critical",
	}
	if got != want {
		t.Fatalf("selector parse: got %+v want %+v", got, want)
	}
}

// TestRoutes_AddRejectsUnknownClause guards the grammar — unknown clauses
// must error rather than silently degrade to a wildcard rule.
func TestRoutes_AddRejectsUnknownClause(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, _, err := runRoot(t, []string{
		"--config-dir", dir, "routes", "add",
		"bad", "wat:nope", "phone",
	}, "")
	if err == nil {
		t.Fatalf("expected error for unknown selector clause")
	}
}

// TestRoutes_AddRejectsEmptyTargets ensures we don't persist a route that
// would never match anything useful.
func TestRoutes_AddRejectsEmptyTargets(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, _, err := runRoot(t, []string{
		"--config-dir", dir, "routes", "add",
		"empty", "*", "",
	}, "")
	if err == nil {
		t.Fatalf("expected error for empty targets")
	}
}

// TestRoutes_ListPrintsAddedRoutesSorted verifies the round-trip of add → list
// and that the output is sorted by route ID for stability.
func TestRoutes_ListPrintsAddedRoutesSorted(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	for _, args := range [][]string{
		{"routes", "add", "zeta", "priority:low", "phone"},
		{"routes", "add", "alpha", "priority:critical", "phone,laptop"},
	} {
		full := append([]string{"--config-dir", dir}, args...)
		if _, _, err := runRoot(t, full, ""); err != nil {
			t.Fatalf("routes add: %v", err)
		}
	}

	stdout, _, err := runRoot(t, []string{"--config-dir", dir, "routes", "list"}, "")
	if err != nil {
		t.Fatalf("routes list: %v", err)
	}

	// Pull non-header lines and verify they are sorted.
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected header + 2 rows, got %d lines:\n%s", len(lines), stdout)
	}
	header := lines[0]
	if !strings.Contains(header, "ID") || !strings.Contains(header, "SELECTOR") || !strings.Contains(header, "TARGETS") {
		t.Fatalf("missing header columns; got: %q", header)
	}
	rows := lines[1:]
	ids := make([]string, len(rows))
	for i, row := range rows {
		// First whitespace-separated token is the ID.
		ids[i] = strings.Fields(row)[0]
	}
	sorted := append([]string{}, ids...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(ids, sorted) {
		t.Fatalf("routes list not sorted: got %v want %v", ids, sorted)
	}

	if !strings.Contains(stdout, "alpha") || !strings.Contains(stdout, "zeta") {
		t.Fatalf("list missing added routes; got:\n%s", stdout)
	}
	// Selector for alpha should be re-serialized in the same grammar.
	if !strings.Contains(stdout, "priority:critical") {
		t.Fatalf("list missing rendered selector; got:\n%s", stdout)
	}
	// Targets should print as a comma-joined list.
	if !strings.Contains(stdout, "phone,laptop") {
		t.Fatalf("list missing rendered targets; got:\n%s", stdout)
	}
}

// TestRoutes_ListEmptyOnlyPrintsHeader confirms list works with zero rows.
func TestRoutes_ListEmptyOnlyPrintsHeader(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	stdout, _, err := runRoot(t, []string{"--config-dir", dir, "routes", "list"}, "")
	if err != nil {
		t.Fatalf("routes list: %v", err)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected only header line, got %d lines:\n%s", len(lines), stdout)
	}
}

// TestRoutes_DelRemovesRoute confirms the delete subcommand tombstones the
// entry so a subsequent list does not show it.
func TestRoutes_DelRemovesRoute(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runRoot(t, []string{
		"--config-dir", dir, "routes", "add", "doomed", "*", "phone",
	}, ""); err != nil {
		t.Fatalf("routes add: %v", err)
	}
	if _, _, err := runRoot(t, []string{
		"--config-dir", dir, "routes", "del", "doomed",
	}, ""); err != nil {
		t.Fatalf("routes del: %v", err)
	}

	routes := loadRoutesFor(t, dir)
	if _, present := routes["doomed"]; present {
		t.Fatalf("route still present after del: %v", routes)
	}

	stdout, _, err := runRoot(t, []string{"--config-dir", dir, "routes", "list"}, "")
	if err != nil {
		t.Fatalf("routes list: %v", err)
	}
	if strings.Contains(stdout, "doomed") {
		t.Fatalf("list still shows deleted route:\n%s", stdout)
	}
}

// TestRoutes_DelMissingRouteErrors ensures deleting a route that was never
// added surfaces a useful error rather than silently succeeding (which would
// hide typos).
func TestRoutes_DelMissingRouteErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := runRoot(t, []string{"--config-dir", dir, "init"}, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, _, err := runRoot(t, []string{
		"--config-dir", dir, "routes", "del", "ghost",
	}, "")
	if err == nil {
		t.Fatalf("expected error deleting nonexistent route")
	}
}
