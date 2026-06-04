export const meta = {
  name: 'ship-agenthive-v0.1.0',
  description: 'Ship agenthive v0.1.0 — TUI, tmux surface + plugin, desktop surface, route matcher, AutoRelay peer source, GoReleaser. Per docs/superpowers/plans/2026-06-04-v0.1.0.md. CI green, merge, tag, release. No Claude attribution.',
  phases: [
    { title: 'Setup' },
    { title: 'Foundation' },
    { title: 'Scaffold' },
    { title: 'Implement L2' },
    { title: 'Implement L3 (TUI tabs)' },
    { title: 'Implement L4 (TUI app)' },
    { title: 'Wire L5' },
    { title: 'Release infra' },
    { title: 'Verify' },
    { title: 'Push' },
    { title: 'CI loop' },
    { title: 'Merge' },
    { title: 'Tag + Release' },
  ],
}

const REPO = args?.repo ?? '/home/devsupreme/work/agenthive'
const BRANCH = args?.branch ?? 'feat/v0.1.0-tui-tmux-release'
const DATE = args?.date ?? '2026-06-04'
const VERSION = args?.version ?? 'v0.1.0'
const PLAN = `${REPO}/docs/superpowers/plans/${DATE}-v0.1.0.md`
const MAX_CI_ATTEMPTS = 8

const PREAMBLE = `
You are working on the agenthive repository at ${REPO}.

Source of truth: ${PLAN}.
RFC the project implements: ${REPO}/docs/rfcs/adopt-libp2p.md.
Current branch: ${BRANCH}.
Target release tag: ${VERSION}.

CRITICAL COMMIT POLICY — applies to every commit you make:
- Conventional Commits format (feat:, fix:, chore:, test:, docs:, refactor:).
- NEVER include "Co-Authored-By: Claude" or any author trailer naming Claude.
- NEVER include "🤖 Generated with Claude Code" or any AI-attribution footer.
- NEVER include a "Generated with" line of any kind.
- Commit body = WHAT and WHY in plain prose. No emoji preambles. No Claude refs.

Always run commands from ${REPO}. Never use --no-verify on commits.

When verifying current libp2p, bubbletea, lipgloss, teatest, or GoReleaser APIs, use the Skill/ToolSearch path to load context7 (library IDs: /libp2p/go-libp2p, /libp2p/go-libp2p-pubsub) or grep go.mod for what's installed. Do not invent imports.
`

// ============================================================================
// Schemas
// ============================================================================
const FOUNDATION_SCHEMA = {
  type: 'object',
  required: ['commits', 'buildOk'],
  properties: {
    commits: { type: 'array', items: { type: 'string' } },
    buildOk: { type: 'boolean' },
    notes: { type: 'string' },
  },
}

const PACKAGE_SCHEMA = {
  type: 'object',
  required: ['pkg', 'files', 'testsPass', 'commitSha'],
  properties: {
    pkg: { type: 'string' },
    files: { type: 'array', items: { type: 'string' } },
    testsPass: { type: 'boolean' },
    commitSha: { type: 'string' },
    notes: { type: 'string' },
  },
}

const VERIFY_SCHEMA = {
  type: 'object',
  required: ['buildOk', 'testOk', 'vetOk', 'allGreen'],
  properties: {
    buildOk: { type: 'boolean' },
    testOk: { type: 'boolean' },
    vetOk: { type: 'boolean' },
    fuzzOk: { type: 'boolean' },
    lintOk: { type: 'boolean' },
    shellLintOk: { type: 'boolean' },
    allGreen: { type: 'boolean' },
    fixCommits: { type: 'array', items: { type: 'string' } },
    failureSummary: { type: 'string' },
  },
}

const PUSH_SCHEMA = {
  type: 'object',
  required: ['prUrl', 'branch', 'headSha'],
  properties: {
    prUrl: { type: 'string' },
    branch: { type: 'string' },
    headSha: { type: 'string' },
  },
}

const CI_SCHEMA = {
  type: 'object',
  required: ['status'],
  properties: {
    status: { enum: ['green', 'red', 'unknown'] },
    failingChecks: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          name: { type: 'string' },
          conclusion: { type: 'string' },
          url: { type: 'string' },
        },
      },
    },
    logSummary: { type: 'string' },
    runUrl: { type: 'string' },
  },
}

const RELEASE_SCHEMA = {
  type: 'object',
  required: ['tag', 'released'],
  properties: {
    tag: { type: 'string' },
    released: { type: 'boolean' },
    artifacts: { type: 'array', items: { type: 'string' } },
    releaseUrl: { type: 'string' },
    notes: { type: 'string' },
  },
}

// ============================================================================
// Phase 1: Setup — branch + cleanup (cherry-pick + develop delete)
// ============================================================================
phase('Setup')
log('Branch + cleanup phase')

await agent(`
${PREAMBLE}

Phase: Setup.

1. cd ${REPO}
2. Confirm on main: \`git rev-parse --abbrev-ref HEAD\` must print "main".
3. \`git fetch origin --prune\`
4. \`git pull origin main\` — sync.
5. Create the feature branch: \`git checkout -b ${BRANCH}\`
6. Cherry-pick the two QoL files from develop:
   \`\`\`
   git checkout origin/develop -- lefthook.yml scripts/install.sh
   \`\`\`
   Inspect each — they should be plain config/script files with no Go/agenthive-specific assumptions baked in. If lefthook.yml references stale tooling, leave it alone for now (the workflow will fix it later if needed). scripts/install.sh: read it to make sure it's safe (no auto-sudo, no remote curl-execute).
7. Commit:
   \`\`\`
   git add lefthook.yml scripts/install.sh
   git commit -m "chore: import lefthook config and install script from the retired develop branch

Both files are universally useful infrastructure: lefthook.yml wires
go vet + golangci-lint + tests + commit-msg verification into git hooks,
and scripts/install.sh helps end-users install the binary. They were
written on the now-superseded develop branch; this brings them across
before develop is deleted."
   \`\`\`
8. Delete the develop branch on the remote (it's architecturally stale — pre-libp2p design): \`git push origin --delete develop\`. If this fails because of branch protection, log the failure and continue — develop deletion is nice-to-have, not blocking.

Return: a short text summary noting which files were imported and whether develop was deleted.
`, { label: 'setup', phase: 'Setup' })

// ============================================================================
// Phase 2: Foundation — add deps
// ============================================================================
phase('Foundation')

const foundation = await agent(`
${PREAMBLE}

Phase: Foundation (Layer 0).

Add the TUI deps:
\`\`\`
cd ${REPO}
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/x/exp/teatest@latest
go get github.com/muesli/termenv@latest
go mod tidy
go build ./...   # should still succeed — no imports yet
\`\`\`

Commit:
\`\`\`
git add go.mod go.sum
git commit -m "chore(deps): add bubbletea, lipgloss, teatest, termenv

Required by the v0.1.0 TUI implementation (internal/tui)."
\`\`\`

Also add the v0.1.0 plan to the branch:
\`\`\`
git add docs/superpowers/plans/${DATE}-v0.1.0.md .claude/workflows/ship-v0.1.0.workflow.js
git commit -m "docs: implementation plan and workflow for v0.1.0

Adds the source-of-truth plan for TUI, tmux surface, desktop surface,
route matcher, AutoRelay peer source, and GoReleaser. Plus the workflow
script driving the implementation."
\`\`\`

Return per FOUNDATION_SCHEMA. buildOk = whether \`go build ./...\` succeeded.
`, { schema: FOUNDATION_SCHEMA, label: 'foundation', phase: 'Foundation' })

if (!foundation || !foundation.buildOk) {
  log('Foundation failed: ' + (foundation?.notes ?? 'unknown'))
  return { aborted: 'foundation_failed', foundation }
}

// ============================================================================
// Phase 3: Scaffold — one barrier task writing every empty-body file
// ============================================================================
phase('Scaffold')

await agent(`
${PREAMBLE}

Phase: Scaffold.

Read the plan (${PLAN}) for tasks L2.A through L5.C. For every package listed, create the Go files with:
- Correct package clauses.
- Correct imports (verify libp2p, bubbletea, lipgloss specifics — \`go doc github.com/charmbracelet/bubbletea Model\` if uncertain; pkg.go.dev for current API).
- Full struct field definitions with JSON tags exactly as specified.
- Public function/method signatures exactly as specified.
- Bodies = panic("not implemented: <Pkg.Fn>") for Go; empty shell scripts that 'echo "not implemented"' for tmux assets.

Files to create:
- internal/router/router.go
- internal/dispatch/tmux_surface.go
- internal/dispatch/desktop_surface.go
- internal/tui/styles.go
- internal/tui/app.go
- internal/tui/peers.go
- internal/tui/routes.go
- internal/tui/actions.go
- internal/tui/logs.go
- cmd/agenthive/cmd_tui.go
- cmd/agenthive/cmd_routes.go
- tmux/agenthive.tmux (skeleton)
- tmux/scripts/notification-clear.sh (skeleton)
- tmux/README.md (just a stub explaining what's in this directory)
- .goreleaser.yml (full content per the plan — this isn't logic, just config)
- .github/workflows/release.yml (full content per the plan)

Also: introduce a \`CmdExecutor\` interface in internal/dispatch/dispatch.go (the tmux and desktop surfaces both need it). Add a concrete \`OSExecutor\` that calls os/exec.Command. Both go in this scaffold.

Also: add package-level vars \`version\`, \`commit\`, \`date\` to cmd/agenthive/main.go (initialised to "dev", "none", "unknown") and a \`--version\` root flag that prints them.

Also: update internal/transport/host.go Config to add the new PeerSource field; the existing default behaviour stays (nil → no-op closure).

CRITICAL: do NOT write logic yet — only types, signatures, panic bodies. Goal: \`go build ./...\` and \`go vet ./...\` exit 0 across the whole repo with these new files present.

Commit:
\`\`\`
git add internal/ cmd/ tmux/ .goreleaser.yml .github/workflows/release.yml
git commit -m "feat: scaffold v0.1.0 packages (TUI, tmux+desktop surfaces, router, release infra)

Adds empty-body Go files for all packages introduced by v0.1.0: router,
tmux surface, desktop surface, TUI styles + app + four tab models, two
new cmd subcommands. Adds skeleton tmux plugin and helper script. Adds
.goreleaser.yml and .github/workflows/release.yml in their final form
(those files are config, not logic).

Bodies panic with 'not implemented' so the build stays green while
parallel implementers fill them in."
\`\`\`

Return a brief summary including the file list and the go build/vet results.

REMINDER: No Claude attribution. Plain conventional-commit bodies only.
`, { label: 'scaffold', phase: 'Scaffold' })

// ============================================================================
// Helper: implementer prompt builder
// ============================================================================
function pkgPrompt(pkgKey, taskRef, extra = '') {
  const scope = pkgKey.split('/').join('-')
  return `
${PREAMBLE}

Phase: Implementation — ${pkgKey} (plan task ${taskRef}).

Strict TDD:
1. Read the plan section for ${taskRef} — public surface + tests.
2. Read the scaffolded file(s) for this package.
3. Write the failing tests first (in *_test.go). Run; confirm failure.
4. Implement minimum code to green. Verify with \`go test -race -count=1\`.
5. Refactor for clarity. Re-verify.
6. \`go vet ./...\` from repo root must stay clean.
7. If a shared type signature change is required, STOP and report. Do not bleed into other packages.

${extra}

Verify libp2p / bubbletea API specifics via go doc, pkg.go.dev, or the context7 MCP server. Do NOT guess imports or signatures.

For TUI tests that use teatest: force ASCII color profile in TestMain via \`lipgloss.SetColorProfile(termenv.Ascii)\` or the equivalent for the currently installed termenv version. Golden files go in internal/tui/testdata/.

Commit (one commit for the package):
\`\`\`
git add <relevant paths>
git commit -m "feat(${scope}): <one-line summary>

<one or two paragraphs describing what this package does and why. NO
Claude attribution. NO co-authored-by. NO generated-with footer.>"
\`\`\`

Return per PACKAGE_SCHEMA. commitSha = SHA of the commit you just made. testsPass = whether \`go test -race -count=1 ./<pkg path>/...\` exits 0.

CRITICAL: No AI-attribution string anywhere.
`
}

// ============================================================================
// Phase 4: Implement L2 — leaf packages
// ============================================================================
phase('Implement L2')

const l2 = await parallel([
  () => agent(pkgPrompt('router',                       'L2.A'), { schema: PACKAGE_SCHEMA, label: 'impl:router',         phase: 'Implement L2' }),
  () => agent(pkgPrompt('transport',                    'L2.B', 'Scope: update internal/transport/host.go to plumb the new Config.PeerSource. Add Tests verifying both nil (default no-op closure) and a custom PeerSource closure get honored. Do NOT change anything in other packages.'), { schema: PACKAGE_SCHEMA, label: 'impl:transport/peersource', phase: 'Implement L2' }),
  () => agent(pkgPrompt('dispatch',                     'L2.C', 'Scope: ONLY internal/dispatch/tmux_surface.go and tmux_surface_test.go. Use a mock CmdExecutor (the interface lives in dispatch package). Do not touch desktop_surface.go.'), { schema: PACKAGE_SCHEMA, label: 'impl:dispatch/tmux', phase: 'Implement L2' }),
  () => agent(pkgPrompt('dispatch',                     'L2.D', 'Scope: ONLY internal/dispatch/desktop_surface.go and desktop_surface_test.go. Mock CmdExecutor. Detect OS via runtime.GOOS. Other OS values are not errors — just no-op.'), { schema: PACKAGE_SCHEMA, label: 'impl:dispatch/desktop', phase: 'Implement L2' }),
  () => agent(pkgPrompt('cmd/agenthive',                'L2.E', 'Scope: ONLY cmd/agenthive/cmd_routes.go and cmd_routes_test.go. Implement add/list/del subcommands. Selector grammar per plan §L2.E.'), { schema: PACKAGE_SCHEMA, label: 'impl:cmd/routes', phase: 'Implement L2' }),
])

if (l2.some(r => r === null || !r.testsPass)) {
  await agent(`
${PREAMBLE}
Phase: L2 fix-up.
L2 results: ${JSON.stringify(l2.map(r => r === null ? 'null' : { pkg: r.pkg, ok: r.testsPass, notes: r.notes }))}.
Diagnose and fix. \`go test -race -count=1 ./...\` must pass. Commit fixes (no Claude attribution).
`, { label: 'l2-fixup', phase: 'Implement L2' })
}

// ============================================================================
// Phase 5: Implement L3 — TUI tab models (parallel)
// ============================================================================
phase('Implement L3 (TUI tabs)')

const l3 = await parallel([
  () => agent(pkgPrompt('tui',                          'L3', 'Scope: ONLY internal/tui/styles.go and styles_test.go. TokyoNight palette via lipgloss. Tests construct the Styles struct and assert non-nil per field.'),                     { schema: PACKAGE_SCHEMA, label: 'impl:tui/styles', phase: 'Implement L3 (TUI tabs)' }),
  () => agent(pkgPrompt('tui',                          'L3', 'Scope: ONLY internal/tui/peers.go and peers_test.go. PeersModel with cursor navigation, status icons (● online / ○ offline), and a window-resize handler. Use teatest + golden file.'), { schema: PACKAGE_SCHEMA, label: 'impl:tui/peers',  phase: 'Implement L3 (TUI tabs)' }),
  () => agent(pkgPrompt('tui',                          'L3', 'Scope: ONLY internal/tui/routes.go and routes_test.go. RoutesModel listing routes; key handlers for cursor + a placeholder add/delete affordance (modal not required for v0.1.0). teatest + golden file.'), { schema: PACKAGE_SCHEMA, label: 'impl:tui/routes',  phase: 'Implement L3 (TUI tabs)' }),
  () => agent(pkgPrompt('tui',                          'L3', 'Scope: ONLY internal/tui/actions.go and actions_test.go. ActionsModel listing pending actions; y/n keys approve/deny by writing through the action queue. teatest + golden file.'), { schema: PACKAGE_SCHEMA, label: 'impl:tui/actions', phase: 'Implement L3 (TUI tabs)' }),
  () => agent(pkgPrompt('tui',                          'L3', 'Scope: ONLY internal/tui/logs.go and logs_test.go. LogsModel listing recent events; keys 1/2/3 filter by level (info/warn/crit); scroll with j/k. teatest + golden file.'),     { schema: PACKAGE_SCHEMA, label: 'impl:tui/logs',    phase: 'Implement L3 (TUI tabs)' }),
])

if (l3.some(r => r === null || !r.testsPass)) {
  await agent(`
${PREAMBLE}
Phase: L3 fix-up.
L3 results: ${JSON.stringify(l3.map(r => r === null ? 'null' : { pkg: r.pkg, ok: r.testsPass, notes: r.notes }))}.
Diagnose and fix. Commit fixes (no Claude attribution).
`, { label: 'l3-fixup', phase: 'Implement L3 (TUI tabs)' })
}

// ============================================================================
// Phase 6: Implement L4 — TUI root app
// ============================================================================
phase('Implement L4 (TUI app)')

await agent(pkgPrompt('tui', 'L4', 'Scope: ONLY internal/tui/app.go and app_test.go. Root App model wiring all four tabs; tab key bindings (p/r/a/l); quit (q, ctrl+c); window-resize broadcasts. teatest integration test asserts the initial tab is Peers, pressing r switches to Routes, etc.'),
  { schema: PACKAGE_SCHEMA, label: 'impl:tui/app', phase: 'Implement L4 (TUI app)' })

// ============================================================================
// Phase 7: Wire L5 — daemon integration + cmd_tui + tmux plugin
// ============================================================================
phase('Wire L5')

const l5 = await parallel([
  () => agent(pkgPrompt('daemon',         'L5.B', 'Scope: modify internal/daemon/daemon.go. Wire router.NewMatcher; build dispatch.TmuxSurface conditionally (only if tmux is in PATH — use exec.LookPath); build dispatch.DesktopSurface unconditionally; pass real peer-source closure to transport.New; expose query API on Unix socket (list_peers/list_routes/list_actions/list_logs). Add daemon_test.go assertions for the query API and matched-target routing using an in-memory 3-host mock.'), { schema: PACKAGE_SCHEMA, label: 'impl:daemon/wire', phase: 'Wire L5' }),
  () => agent(pkgPrompt('cmd/agenthive',  'L5.A', 'Scope: ONLY cmd/agenthive/cmd_tui.go and cmd_tui_test.go. Connect to local daemon Unix socket; query for initial state; launch bubbletea App; poll every 2s for updates. Clear error if daemon unreachable.'), { schema: PACKAGE_SCHEMA, label: 'impl:cmd/tui',   phase: 'Wire L5' }),
  () => agent(pkgPrompt('tmux',           'L5.C', 'Scope: tmux/agenthive.tmux + tmux/scripts/notification-clear.sh + tmux/README.md. Idempotent status-right append (sentinel @agenthive-installed); pane-focus-in hook calling the clear script. Tests: a Go test file at tmux/tmux_test.go that uses bash -n to syntax-check the scripts and greps for expected hook strings; skip live-tmux tests in CI.'), { schema: PACKAGE_SCHEMA, label: 'impl:tmux',       phase: 'Wire L5' }),
])

if (l5.some(r => r === null || !r.testsPass)) {
  await agent(`
${PREAMBLE}
Phase: L5 fix-up.
L5 results: ${JSON.stringify(l5.map(r => r === null ? 'null' : { pkg: r.pkg, ok: r.testsPass, notes: r.notes }))}.
Diagnose and fix. Commit fixes (no Claude attribution).
`, { label: 'l5-fixup', phase: 'Wire L5' })
}

// ============================================================================
// Phase 8: Release infra check + README install update
// ============================================================================
phase('Release infra')

await agent(`
${PREAMBLE}

Phase: Release infra and README install update.

1. Verify .goreleaser.yml and .github/workflows/release.yml are present from the scaffold and unchanged. If scaffold left placeholder values, fill them per the plan §L6 specification.
2. Verify cmd/agenthive/main.go exposes \`version\`, \`commit\`, \`date\` package-level vars and a \`--version\` flag that prints them.
3. Locally test the GoReleaser config syntactically: \`go run github.com/goreleaser/goreleaser/v2@latest check\` (if available; otherwise skip).
4. Update README.md: replace the install section with the v0.1.0 install instructions per plan §L6.B. Flip the status matrix: \`TUI\` → \`shipped\`, \`tmux per-pane surface\` → \`shipped\`, \`Desktop notifications\` → \`shipped\`. Move the no-longer-true line \`No pre-built binaries yet — build from source.\` and replace with: \`Pre-built binaries available on the Releases page from v0.1.0 onward.\`
5. Add a 'TUI' subsection to the Quick Start that demonstrates \`agenthive tui\`.
6. Add a 'tmux plugin' subsection demonstrating \`set -g @plugin 'shaiknoorullah/agenthive'\` and \`prefix + I\`.

Commit:
\`\`\`
git add README.md .goreleaser.yml .github/workflows/release.yml cmd/agenthive/main.go
git commit -m "docs+release(v0.1.0): GoReleaser config, release workflow, README install and status updates

Adds production GoReleaser config (linux+darwin × amd64+arm64 tarballs,
checksums, GitHub release upload) plus the tag-triggered release
workflow. Updates the README install methods to include go install
@v0.1.0, prebuilt binary download, and TPM plugin install. Flips the
status matrix rows for TUI, tmux surface, and desktop surface from
'next' to 'shipped'."
\`\`\`

Return a brief summary. No Claude attribution.
`, { label: 'release-infra', phase: 'Release infra' })

// ============================================================================
// Phase 9: Local verify
// ============================================================================
phase('Verify')

let verifyAttempt = 0
let verify = null
while (verifyAttempt < 3) {
  verify = await agent(`
${PREAMBLE}

Phase: Verify.

Run, from ${REPO}, all of:
\`\`\`
go build ./...
go test -race -count=1 ./...
go vet ./...
go test -fuzz=Fuzz -fuzztime=10s -run=^$ ./internal/crdt/...
bash -n tmux/agenthive.tmux tmux/scripts/*.sh
which golangci-lint >/dev/null 2>&1 && golangci-lint run --timeout=5m || echo "lint-skipped"
\`\`\`

Each must exit 0 (lintOk=true if skipped because not installed).

If anything fails, diagnose and fix in-place. Commit per logical fix (no Claude attribution). Re-run the gauntlet. Cap fixes at 5 commits in this phase; if still failing after that, return allGreen=false with a clear failureSummary.

Return per VERIFY_SCHEMA.
`, { schema: VERIFY_SCHEMA, label: `verify:attempt-${verifyAttempt + 1}`, phase: 'Verify' })

  if (verify && verify.allGreen) break
  verifyAttempt++
  log(`Verify attempt ${verifyAttempt} failed: ${verify?.failureSummary ?? 'unknown'}`)
}

if (!verify || !verify.allGreen) {
  log('Local verify could not be made green in 3 attempts. Aborting before push.')
  return { aborted: 'local_verify_failed', verify }
}

// ============================================================================
// Phase 10: Push + PR
// ============================================================================
phase('Push')

const push = await agent(`
${PREAMBLE}

Phase: Push and open PR.

1. cd ${REPO}; git status must be clean.
2. git log --oneline main..HEAD (review the commits riding this branch).
3. git push -u origin ${BRANCH}
4. Open PR:

\`\`\`
gh pr create --title "feat(v0.1.0): TUI, tmux surface, desktop surface, router, release infrastructure" --body "$(cat <<'EOF'
## Summary

v0.1.0 — the first user-installable agenthive release. Lands the surface layer on top of the libp2p substrate that shipped in #11.

## Subsystems shipping

- **internal/router/** — RouteMatcher: walks CRDT routes, returns target peer IDs per notification
- **internal/transport/** — Config.PeerSource: CRDT-driven AutoRelay peer source (closes the no-op closure gap from the libp2p RFC §10)
- **internal/dispatch/tmux_surface.go** — per-pane native tmux options writer (zero shell forks on the status-line hot path)
- **internal/dispatch/desktop_surface.go** — notify-send (Linux) / osascript (macOS)
- **internal/tui/** — bubbletea TUI: peers / routes / actions / logs tabs, tab switching, key handling, golden-file-tested
- **cmd/agenthive/cmd_tui.go** — \`agenthive tui\` subcommand
- **cmd/agenthive/cmd_routes.go** — \`agenthive routes add\\|list\\|del\`
- **tmux/** — TPM-compatible plugin entry script + helper scripts + README
- **.goreleaser.yml** — linux/darwin × amd64/arm64 tarballs, checksums, signed GitHub release
- **.github/workflows/release.yml** — tag-triggered release workflow
- **lefthook.yml**, **scripts/install.sh** — imported from the now-deleted develop branch

## README updates

- Install via \`go install @v0.1.0\`, prebuilt binary download, TPM plugin install
- Status matrix flipped: TUI / tmux surface / desktop surface → shipped
- Quick Start expanded with \`agenthive tui\` and the TPM line

## Out of scope (v0.2.0+)

Termux push, ntfy/Slack/Discord/audio surfaces, notification grouping/DND, per-message GossipSub signature validation, brew/scoop/AUR.

## Test plan

- [x] \`go build ./...\`
- [x] \`go test -race -count=1 ./...\`
- [x] \`go vet ./...\`
- [x] \`go test -fuzz=Fuzz -fuzztime=10s ./internal/crdt\`
- [x] \`bash -n\` on all tmux shell scripts
- [ ] CI green
- [ ] Tag \`v0.1.0\` cuts a release with 4 platform tarballs + checksums.txt
EOF
)"
\`\`\`

NO Claude attribution anywhere. The PR body above is the literal body.

5. Capture the PR URL and the HEAD SHA. Return per PUSH_SCHEMA.
`, { schema: PUSH_SCHEMA, label: 'push-and-pr', phase: 'Push' })

if (!push?.prUrl) {
  log('Push or PR creation failed — aborting.')
  return { aborted: 'push_failed', push }
}

log(`PR opened: ${push.prUrl}`)

// ============================================================================
// Phase 11: CI watch + fix loop
// ============================================================================
phase('CI loop')

let attempt = 0
let lastStatus = null
while (attempt < MAX_CI_ATTEMPTS) {
  attempt++
  log(`CI watch attempt ${attempt}/${MAX_CI_ATTEMPTS}…`)

  lastStatus = await agent(`
${PREAMBLE}

Phase: CI watch.

PR: ${push.prUrl}
Branch: ${BRANCH}

1. cd ${REPO}
2. \`gh pr checks ${BRANCH} --watch --interval 15\` — blocks until all checks complete.
3. After it returns: \`gh pr checks ${BRANCH} --json name,state,conclusion,detailsUrl\` — parse to identify failing checks.
4. If all checks have conclusion=SUCCESS: return status=green, failingChecks=[], logSummary="all green".
5. Otherwise: for each failing check, fetch failing-job logs:
   - \`gh run view <run-id> --log-failed | tail -200\`
   - Summarize the relevant error lines (test failures, vet errors, build errors).
6. Return per CI_SCHEMA. logSummary ≤ 4000 chars.
`, { schema: CI_SCHEMA, label: `ci-watch:${attempt}`, phase: 'CI loop' })

  if (!lastStatus) {
    log(`CI watch attempt ${attempt} returned null — retrying.`)
    continue
  }

  if (lastStatus.status === 'green') {
    log('CI is GREEN.')
    break
  }

  log(`CI is RED on attempt ${attempt}. Dispatching fix.`)

  await agent(`
${PREAMBLE}

Phase: CI fix (attempt ${attempt}).

PR: ${push.prUrl}
Branch: ${BRANCH}
Failing checks: ${JSON.stringify(lastStatus.failingChecks ?? [])}

Log summary from failing jobs:

\`\`\`
${(lastStatus.logSummary ?? '').slice(0, 4000)}
\`\`\`

Diagnose root cause. Reproduce locally:
\`\`\`
cd ${REPO}
git pull --rebase origin ${BRANCH}
go build ./...
go test -race -count=1 ./...
go vet ./...
\`\`\`

If the local run reproduces, fix it. If not (CI-only failure — missing tool, environment difference), examine .github/workflows/ci.yml and adjust either the workflow or code, without weakening tests. Acceptable: \`if os.Getenv("CI") != "" { t.Skip("requires real tmux") }\` for tests that genuinely cannot run in CI. NOT acceptable: blanket t.Skip on tests that should pass.

Make focused fix commits. NO Claude attribution.

\`\`\`
git push origin ${BRANCH}
\`\`\`

Return a short text summary: theory of root cause + commits made.
`, { label: `ci-fix:${attempt}`, phase: 'CI loop' })
}

if (lastStatus?.status !== 'green') {
  log(`CI not green after ${MAX_CI_ATTEMPTS} attempts. Stopping before merge.`)
  return { aborted: 'ci_loop_exhausted', attempts: attempt, lastStatus, prUrl: push.prUrl }
}

// ============================================================================
// Phase 12: Merge
// ============================================================================
phase('Merge')

const merge = await agent(`
${PREAMBLE}

Phase: Merge.

PR: ${push.prUrl}
Branch: ${BRANCH}

CI is green. Branch protection is now aligned (5 required checks, 0 reviews). Squash-merge:

\`\`\`
cd ${REPO}
gh pr merge ${BRANCH} --squash --delete-branch
git checkout main
git pull origin main
git log --oneline -5
\`\`\`

Capture the new HEAD commit on main. NO Claude attribution in the squash commit; if gh pulled one from anywhere, edit it.

Return: { merged: true, mainHead: "<sha>" } as plain text JSON in your final reply.
`, { label: 'merge', phase: 'Merge' })

// ============================================================================
// Phase 13: Tag v0.1.0 + verify release
// ============================================================================
phase('Tag + Release')

const release = await agent(`
${PREAMBLE}

Phase: Tag and release.

Tag the freshly-merged HEAD on main as ${VERSION} and let the release workflow publish artifacts.

\`\`\`
cd ${REPO}
git checkout main
git pull origin main
git tag -a ${VERSION} -m "${VERSION}: first agenthive release

Adds the surface layer on top of the libp2p substrate from #11.
Functional TUI, tmux per-pane surface + plugin, desktop notifications
(notify-send / osascript), CRDT-driven route matcher, AutoRelay peer
source closure, GoReleaser-published binaries for linux/darwin ×
amd64/arm64."
git push origin ${VERSION}
\`\`\`

Then watch the release workflow:

\`\`\`
sleep 5
RUN_ID=$(gh run list --workflow=release.yml --branch=${VERSION} --limit 1 --json databaseId --jq '.[0].databaseId')
if [ -z "$RUN_ID" ]; then
  RUN_ID=$(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId')
fi
gh run watch "$RUN_ID" --interval 15
\`\`\`

If the release workflow fails, fetch its log:
\`\`\`
gh run view "$RUN_ID" --log-failed | tail -200
\`\`\`
and diagnose. Likely issues: missing GH_TOKEN scope, .goreleaser.yml syntax error, missing main package entrypoint. Fix in a follow-up commit on main (or open a PR for it), re-trigger by deleting and re-pushing the tag (\`git push origin :${VERSION}\` then re-tag and push). Cap retries at 2.

When the release workflow is green:
\`\`\`
gh release view ${VERSION} --json name,tagName,assets,url
\`\`\`

Verify the asset list contains at least:
- agenthive_0.1.0_linux_amd64.tar.gz
- agenthive_0.1.0_linux_arm64.tar.gz
- agenthive_0.1.0_darwin_amd64.tar.gz
- agenthive_0.1.0_darwin_arm64.tar.gz
- checksums.txt

Sanity-check a binary:
\`\`\`
TMP=$(mktemp -d)
cd "$TMP"
curl -sL "https://github.com/shaiknoorullah/agenthive/releases/download/${VERSION}/agenthive_0.1.0_linux_amd64.tar.gz" | tar -xz
file agenthive | head -1
./agenthive --version
\`\`\`

Return per RELEASE_SCHEMA. released=true if the release page has at least 4 platform tarballs and checksums.txt. Include the release URL.

NO Claude attribution in the tag message, release notes, or anywhere else.
`, { schema: RELEASE_SCHEMA, label: 'tag-and-release', phase: 'Tag + Release' })

log(`Done. Release: ${release?.releaseUrl ?? 'unknown'}`)

return {
  status: release?.released ? 'shipped' : 'merged-but-release-failed',
  branch: BRANCH,
  prUrl: push.prUrl,
  ciAttempts: attempt,
  tag: VERSION,
  releaseUrl: release?.releaseUrl,
  artifacts: release?.artifacts,
}
