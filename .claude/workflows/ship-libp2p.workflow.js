export const meta = {
  name: 'ship-agenthive-libp2p',
  description: 'Implement libp2p-based agenthive (identity, transport, discovery, protocols, hooks, dispatch, daemon, cmd) per docs/superpowers/plans/2026-06-04-libp2p-impl.md, ship to CI green, merge. No Claude attribution.',
  phases: [
    { title: 'Setup' },
    { title: 'Foundation' },
    { title: 'Scaffold' },
    { title: 'Implement L1' },
    { title: 'Implement L2' },
    { title: 'Implement L3' },
    { title: 'Implement L4' },
    { title: 'Implement L5' },
    { title: 'Verify' },
    { title: 'Push' },
    { title: 'CI loop' },
    { title: 'Merge' },
  ],
}

// ============================================================================
// Args (passed at launch). Date is baked in because Date.now() is blocked.
// ============================================================================
const REPO = args?.repo ?? '/home/devsupreme/work/agenthive'
const BRANCH = args?.branch ?? 'feat/libp2p-impl'
const DATE = args?.date ?? '2026-06-04'
const PLAN = `${REPO}/docs/superpowers/plans/${DATE}-libp2p-impl.md`
const RFC = `${REPO}/docs/rfcs/adopt-libp2p.md`
const MAX_CI_ATTEMPTS = 8

// ============================================================================
// Universal preamble injected into every agent prompt. Enforces the commit
// policy and gives every agent the same context anchor.
// ============================================================================
const PREAMBLE = `
You are working on the agenthive repository at ${REPO}.

The source-of-truth plan for this entire effort is ${PLAN}.
The architectural RFC it implements is ${RFC}.
The current branch is ${BRANCH}.

CRITICAL COMMIT POLICY — applies to every commit you make:
- Conventional Commits format (feat:, fix:, chore:, test:, docs:, refactor:).
- NEVER include "Co-Authored-By: Claude" or any author trailer naming Claude.
- NEVER include "🤖 Generated with Claude Code" or any AI-attribution footer.
- NEVER include a "Generated with" line of any kind.
- Commit message body should describe WHAT and WHY, plainly. No agent attribution, no emoji preambles, no Claude references.
- If you find yourself about to add an attribution line, stop and remove it.

Always run commands from ${REPO} (use cd or absolute paths). Do not use --no-verify on commits.
`

// ============================================================================
// Schemas for structured agent returns
// ============================================================================
const FOUNDATION_SCHEMA = {
  type: 'object',
  required: ['commits', 'buildOk', 'testOk'],
  properties: {
    commits: { type: 'array', items: { type: 'string' } },
    buildOk: { type: 'boolean' },
    testOk: { type: 'boolean' },
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

const MERGE_SCHEMA = {
  type: 'object',
  required: ['merged', 'mainHead'],
  properties: {
    merged: { type: 'boolean' },
    mainHead: { type: 'string' },
    prUrl: { type: 'string' },
  },
}

// ============================================================================
// Phase 1: Setup — create branch, sanity-check workspace
// ============================================================================
phase('Setup')
log('Creating feature branch and verifying workspace…')

await agent(`
${PREAMBLE}

Phase: Setup.

1. cd ${REPO}
2. Confirm we are on main: \`git rev-parse --abbrev-ref HEAD\` must print "main". If not, abort with an error message.
3. Confirm working tree state: \`git status --porcelain\` should list the 4 untracked RFC docs from the design phase (adopt-libp2p.md and 3 debate-*-advocate.md files) AND the plan doc and this workflow file in .claude/workflows/. No other unexpected changes. If unexpected modifications exist, abort.
4. Create the feature branch: \`git checkout -b ${BRANCH}\`
5. Stage the untracked design docs and plan: \`git add docs/rfcs/adopt-libp2p.md docs/rfcs/debate-libp2p-advocate.md docs/rfcs/debate-quic-mtls-advocate.md docs/rfcs/debate-yggdrasil-advocate.md docs/superpowers/plans/${DATE}-libp2p-impl.md\` (don't commit yet — first commit comes in the next phase as part of the foundation).
6. Confirm: \`git status\`

Return a short text summary. Do not commit anything.
`, { label: 'setup', phase: 'Setup' })

// ============================================================================
// Phase 2: Foundation — fix HLC bug, add libp2p/pubsub/cobra deps, first commit
// ============================================================================
phase('Foundation')
log('Layer 0: fixing HLC bug and adding libp2p/pubsub/cobra deps…')

const foundation = await agent(`
${PREAMBLE}

Phase: Foundation (Layer 0).

This phase has two sub-tasks. Do them in order and commit separately.

==== L0.A — Fix HLC zero-time monotonic bug ====

Read ${REPO}/internal/crdt/hlc.go. The HLC.Now method can return two equal timestamps when wall time is zero (because h.last is initialized to Timestamp{} all-zeros, and the first call hits a branch that doesn't actually advance counter). The .fail corpus under ${REPO}/internal/crdt/testdata/rapid/ proves it for TestHLC_Property_Monotonic.

Fix design:
- Add a private \`initialized bool\` field to HLC.
- In Now(): on the first call OR when the new wall <= h.last.Wall, force the counter to advance such that the returned Timestamp is strictly After h.last.
- Maintain backward-compatible API.

Verification:
\`\`\`
cd ${REPO}
rm -rf internal/crdt/testdata/rapid
go test -race -count=1 -rapid.checks=1000 ./internal/crdt/...
[ ! -d internal/crdt/testdata/rapid ] || ls internal/crdt/testdata/rapid
\`\`\`
The test must pass and the testdata/rapid directory must NOT contain any .fail files afterward.

Also clean any other stale .fail files generated during the test run.

Commit:
\`\`\`
git add internal/crdt/hlc.go internal/crdt/hlc_test.go
git rm -r --cached internal/crdt/testdata/rapid 2>/dev/null || true
git commit -m "fix(crdt): HLC.Now monotonic guarantee from cold-start zero wall

The previous implementation could return two equal timestamps when the
wall clock returned zero (e.g., in tests using a constant clock), because
h.last was initialized to Timestamp{} and the first Now() call hit the
'wall ahead, reset counter' branch with no actual advance.

Track an initialized flag and ensure every Now() returns a strictly After
timestamp. Property test TestHLC_Property_Monotonic confirms with 1000+
rapid checks."
\`\`\`

==== L0.B — Add libp2p, pubsub, cobra deps ====

\`\`\`
cd ${REPO}
go get github.com/libp2p/go-libp2p@latest
go get github.com/libp2p/go-libp2p-pubsub@latest
go get github.com/spf13/cobra@latest
go mod tidy
go build ./...
\`\`\`

go build must succeed (nothing imports the new deps yet). If it fails, fix imports until clean.

Commit:
\`\`\`
git add go.mod go.sum
git commit -m "chore(deps): add libp2p, libp2p-pubsub, cobra

Required by the libp2p adoption RFC (docs/rfcs/adopt-libp2p.md) and the
implementation plan (docs/superpowers/plans/${DATE}-libp2p-impl.md)."
\`\`\`

==== Also commit the design docs + plan ====

\`\`\`
git add docs/rfcs/adopt-libp2p.md docs/rfcs/debate-libp2p-advocate.md docs/rfcs/debate-quic-mtls-advocate.md docs/rfcs/debate-yggdrasil-advocate.md docs/superpowers/plans/${DATE}-libp2p-impl.md .claude/workflows/ship-libp2p.workflow.js
git commit -m "docs: adopt libp2p transport; debate papers and implementation plan

Adoption RFC supersedes the SSH+gossip transport judgment. Includes three
advocate debate papers (libp2p, QUIC+mTLS, Yggdrasil) and the workflow
script that drives this implementation."
\`\`\`

Return per FOUNDATION_SCHEMA. Include the three commit subjects in commits[].

REMINDER: No Claude attribution in any commit. No 'Co-Authored-By: Claude'. No 'Generated with Claude Code'. No emoji preambles.
`, { schema: FOUNDATION_SCHEMA, label: 'foundation', phase: 'Foundation' })

if (!foundation || !foundation.buildOk || !foundation.testOk) {
  log('Foundation failed — aborting. ' + (foundation?.notes ?? ''))
  return { aborted: 'foundation_failed', foundation }
}

// ============================================================================
// Phase 3: Scaffold — write all empty-body interfaces in one barrier task
// so that parallel implementers in subsequent phases can compile against
// stable shared types.
// ============================================================================
phase('Scaffold')
log('Scaffolding interfaces, types, and panic-bodied files for all packages…')

const scaffold = await agent(`
${PREAMBLE}

Phase: Scaffold.

Read the plan (${PLAN}) tasks L1.A through L5. For every package listed, create the .go files with:
- Correct package clause.
- Correct imports (use context7 via mcp__plugin_context7_context7__resolve-library-id / query-docs to verify any libp2p import paths you are unsure of — library IDs: /libp2p/go-libp2p, /libp2p/go-libp2p-pubsub).
- Full struct field definitions with JSON tags exactly as specified in the plan.
- Public function signatures exactly as specified.
- Function bodies = panic("not implemented: <Pkg.Fn>") for now.
- Methods on interfaces fully declared.
- For cmd/agenthive/main.go: a cobra root command that registers all subcommand stubs.

Files to create (one or more per package):
- internal/identity/identity.go
- internal/transport/host.go
- internal/discovery/mdns.go
- internal/protocols/protocols.go
- internal/protocols/messages.go
- internal/hooks/security.go
- internal/hooks/queue.go
- internal/hooks/gate.go
- internal/dispatch/dispatch.go
- internal/dispatch/log_surface.go
- internal/daemon/daemon.go
- internal/daemon/socket.go
- cmd/agenthive/main.go
- cmd/agenthive/cmd_init.go
- cmd/agenthive/cmd_id.go
- cmd/agenthive/cmd_peers.go
- cmd/agenthive/cmd_start.go
- cmd/agenthive/cmd_hook.go
- cmd/agenthive/cmd_respond.go

After scaffolding:
\`\`\`
cd ${REPO}
go build ./...
go vet ./...
\`\`\`
Both must exit 0. Fix any compilation issues. Do NOT write logic yet — only types, interfaces, and panic bodies.

Commit:
\`\`\`
git add internal/ cmd/
git commit -m "feat: scaffold libp2p-based packages (types, interfaces, signatures)

Adds empty-body Go files for all packages introduced by the libp2p
adoption: identity, transport, discovery, protocols, hooks, dispatch,
daemon, and the cmd/agenthive CLI surface. Bodies panic with
'not implemented' to keep the build green while parallel implementers
fill them in."
\`\`\`

Return: a short summary noting the files created. The downstream phases will dispatch implementer agents per package.

REMINDER: No Claude attribution. Conventional commits only.
`, { label: 'scaffold', phase: 'Scaffold' })

// ============================================================================
// Helper: build an implementer prompt for one package
// ============================================================================
function pkgPrompt(pkgKey, taskRef, extraGuidance = '') {
  return `
${PREAMBLE}

Phase: Implementation — package ${pkgKey}.

Your job: replace the scaffolded panic bodies in this package with a real implementation that conforms exactly to the contract in ${PLAN} task ${taskRef}. Follow strict TDD:

1. Open the plan section for task ${taskRef} and read the public surface + tests required.
2. Open the existing scaffold file(s) for this package.
3. Write the failing tests first (in the _test.go file). Run them and confirm they fail.
4. Implement the minimum code to make them pass. Verify with go test.
5. Refactor for clarity.
6. Run \`go test -race -count=1 ./internal/${pkgKey}/...\` (or appropriate path) and confirm it passes.
7. Run \`go vet ./...\` from repo root. Must be clean.
8. If your implementation requires changing a shared type signature in another package, STOP and report — do not bleed into other packages. The scaffold locked the contract.
9. When verifying libp2p API specifics, query context7 (library IDs: /libp2p/go-libp2p, /libp2p/go-libp2p-pubsub) — do not guess.

${extraGuidance}

Commit (one commit for the package):
\`\`\`
git add internal/${pkgKey}/ # adjust path if needed
git commit -m "feat(${pkgKey.replace(/\\//g, '-')}): <short summary>

<one or two paragraphs describing what this package does and why, in
plain prose. NO Claude attribution. NO co-authored-by. NO generated-with
footer.>"
\`\`\`

Return per the PACKAGE_SCHEMA. commitSha = the SHA of the commit you just made. testsPass = whether \`go test -race -count=1 ./internal/${pkgKey}/...\` exits 0.

CRITICAL: Do NOT write any AI-attribution string anywhere — no 'Claude', 'Anthropic', 'AI-generated', 'Co-Authored-By: Claude'. If you find yourself about to, stop.
`
}

// ============================================================================
// Phase 4: Implement L1 — leaf packages (no internal deps on other unbuilt pkgs)
// ============================================================================
phase('Implement L1')
log('Layer 1: identity, protocols, hooks/security, hooks/queue, dispatch (iface)…')

const l1 = await parallel([
  () => agent(pkgPrompt('identity',         'L1.A'), { schema: PACKAGE_SCHEMA, label: 'impl:identity',          phase: 'Implement L1' }),
  () => agent(pkgPrompt('protocols',        'L1.B'), { schema: PACKAGE_SCHEMA, label: 'impl:protocols',         phase: 'Implement L1' }),
  () => agent(pkgPrompt('hooks',            'L1.C',  'Scope this task to ONLY internal/hooks/security.go and security_test.go. Do not touch queue.go or gate.go.'), { schema: PACKAGE_SCHEMA, label: 'impl:hooks/security',   phase: 'Implement L1' }),
  () => agent(pkgPrompt('hooks',            'L1.D',  'Scope this task to ONLY internal/hooks/queue.go and queue_test.go. Do not touch security.go or gate.go.'),   { schema: PACKAGE_SCHEMA, label: 'impl:hooks/queue',      phase: 'Implement L1' }),
  () => agent(pkgPrompt('dispatch',         'L1.E',  'Scope this task to ONLY internal/dispatch/dispatch.go (the Surface interface and Dispatcher) and dispatch_test.go. Do not touch log_surface.go.'), { schema: PACKAGE_SCHEMA, label: 'impl:dispatch/iface', phase: 'Implement L1' }),
])

const l1Failures = l1.filter(r => r === null || !r.testsPass)
if (l1Failures.length > 0) {
  log(`Layer 1 had ${l1Failures.length} failure(s) — running a fix-up pass.`)
  await agent(`
${PREAMBLE}
Phase: Fix-up after L1.
Failing or null L1 results: ${JSON.stringify(l1.map(r => r === null ? 'null' : { pkg: r.pkg, ok: r.testsPass, notes: r.notes }))}.
Diagnose each, fix in-place, run \`go test -race -count=1 ./...\`, commit any fixes. No Claude attribution.
Return a brief summary.
`, { label: 'l1-fixup', phase: 'Implement L1' })
}

// ============================================================================
// Phase 5: Implement L2 — depends on L1
// ============================================================================
phase('Implement L2')
log('Layer 2: transport (needs identity), dispatch/log_surface, hooks/gate…')

const l2 = await parallel([
  () => agent(pkgPrompt('transport',        'L2.A'), { schema: PACKAGE_SCHEMA, label: 'impl:transport',         phase: 'Implement L2' }),
  () => agent(pkgPrompt('dispatch',         'L2.B',  'Scope this task to ONLY internal/dispatch/log_surface.go and log_surface_test.go. Implement the LogSurface that writes JSON lines.'), { schema: PACKAGE_SCHEMA, label: 'impl:dispatch/log', phase: 'Implement L2' }),
  () => agent(pkgPrompt('hooks',            'L2.C',  'Scope this task to ONLY internal/hooks/gate.go and gate_test.go. Use the queue and dispatcher interfaces already shipped in L1.'), { schema: PACKAGE_SCHEMA, label: 'impl:hooks/gate',  phase: 'Implement L2' }),
])

const l2Failures = l2.filter(r => r === null || !r.testsPass)
if (l2Failures.length > 0) {
  log(`Layer 2 had ${l2Failures.length} failure(s) — running a fix-up pass.`)
  await agent(`
${PREAMBLE}
Phase: Fix-up after L2.
Failing or null L2 results: ${JSON.stringify(l2.map(r => r === null ? 'null' : { pkg: r.pkg, ok: r.testsPass, notes: r.notes }))}.
Diagnose, fix, run \`go test -race -count=1 ./...\`, commit fixes. No Claude attribution.
`, { label: 'l2-fixup', phase: 'Implement L2' })
}

// ============================================================================
// Phase 6: Implement L3 — discovery, daemon/socket
// ============================================================================
phase('Implement L3')

const l3 = await parallel([
  () => agent(pkgPrompt('discovery',        'L3.A'), { schema: PACKAGE_SCHEMA, label: 'impl:discovery',        phase: 'Implement L3' }),
  () => agent(pkgPrompt('daemon',           'L3.B',  'Scope this task to ONLY internal/daemon/socket.go and socket_test.go. Do not touch daemon.go.'), { schema: PACKAGE_SCHEMA, label: 'impl:daemon/socket', phase: 'Implement L3' }),
])

const l3Failures = l3.filter(r => r === null || !r.testsPass)
if (l3Failures.length > 0) {
  log(`Layer 3 had ${l3Failures.length} failure(s) — running a fix-up pass.`)
  await agent(`
${PREAMBLE}
Phase: Fix-up after L3.
Failing or null L3 results: ${JSON.stringify(l3.map(r => r === null ? 'null' : { pkg: r.pkg, ok: r.testsPass, notes: r.notes }))}.
Diagnose, fix, commit. No Claude attribution.
`, { label: 'l3-fixup', phase: 'Implement L3' })
}

// ============================================================================
// Phase 7: Implement L4 — daemon (the integration coordinator)
// ============================================================================
phase('Implement L4')
log('Layer 4: daemon (Run loop wiring transport + gossipsub + dispatch + hooks)…')

await agent(pkgPrompt('daemon', 'L4', 'Scope: internal/daemon/daemon.go and daemon_test.go. Wire transport.New, pubsub.NewGossipSub + Join + Subscribe on TopicState, stream handlers for the four protocol IDs, mDNS via discovery.StartMDNS, SocketServer, and a goroutine that periodically publishes deltas of the StateStore. Test must include the two-peer convergence assertion described in the plan.'),
  { schema: PACKAGE_SCHEMA, label: 'impl:daemon', phase: 'Implement L4' })

// ============================================================================
// Phase 8: Implement L5 — cmd/agenthive (cobra CLI)
// ============================================================================
phase('Implement L5')
log('Layer 5: cmd/agenthive cobra CLI…')

await agent(pkgPrompt('cmd/agenthive', 'L5', 'Scope: cmd/agenthive/*.go. Implement all 6 subcommands as described in the plan. Use cobra.Command.SetIn/SetOut/SetErr so tests can run subcommands in-process. The hook subcommand must read PreToolUse JSON from stdin, dial the Unix socket, and print the hook output JSON. If the daemon is unreachable or the request times out, exit 0 with no output (Claude falls back to its built-in prompt — never fail-closed).'),
  { schema: PACKAGE_SCHEMA, label: 'impl:cmd', phase: 'Implement L5' })

// ============================================================================
// Phase 9: Verify — the local gauntlet
// ============================================================================
phase('Verify')
log('Running go build, go test -race, go vet, go fuzz (10s)…')

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
which golangci-lint >/dev/null 2>&1 && golangci-lint run --timeout=5m || echo "golangci-lint not installed; skipping"
\`\`\`

Each must exit 0 (lint may be skipped if not installed — record lintOk=true in that case).

If any fail, diagnose and fix in-place. Commit fixes with conventional-commit messages (no Claude attribution). You may make multiple fix commits. Then re-run the gauntlet.

Cap your fix attempts at 5 commits in this phase. If still failing after 5, return allGreen=false with a failure summary.

Return per VERIFY_SCHEMA.
`, { schema: VERIFY_SCHEMA, label: `verify:attempt-${verifyAttempt + 1}`, phase: 'Verify' })

  if (verify && verify.allGreen) break
  verifyAttempt++
  log(`Verify attempt ${verifyAttempt} failed: ${verify?.failureSummary ?? 'unknown'}`)
}

if (!verify || !verify.allGreen) {
  log('Local verify could not be made green after 3 attempts. Aborting before push.')
  return { aborted: 'local_verify_failed', verify }
}

// ============================================================================
// Phase 10: Push + open PR
// ============================================================================
phase('Push')

const push = await agent(`
${PREAMBLE}

Phase: Push and open PR.

1. cd ${REPO}
2. git status (must be clean — no uncommitted changes)
3. git log --oneline main..HEAD  (review the commits riding this branch)
4. git push -u origin ${BRANCH}
5. Open a PR with:

\`\`\`
gh pr create --title "feat: libp2p-based transport, daemon, hooks, dispatch" --body "$(cat <<'EOF'
## Summary

Adopt go-libp2p as the agenthive transport, identity, and discovery layer per docs/rfcs/adopt-libp2p.md. Ships the unbuilt subsystems (transport, identity, discovery, protocols, hooks, dispatch, daemon, cmd) on the new substrate.

## What landed

- internal/identity — Ed25519 keypair persistence
- internal/transport — libp2p Host (TCP + QUIC + Noise + DCUtR + AutoRelay + embedded Circuit Relay v2 + UPnP)
- internal/discovery — mDNS LAN peer discovery
- internal/protocols — stream protocol IDs, GossipSub topic, framed JSON messages
- internal/hooks — action gate, file queue (O_CREAT|O_EXCL first-response-wins), destructive-action classifier
- internal/dispatch — Dispatcher interface + log surface
- internal/daemon — Run loop wiring Host, pubsub, mDNS, dispatcher, hooks, Unix socket IPC
- cmd/agenthive — cobra CLI: init, id, peers add|list, start, hook, respond

Plus: HLC zero-time monotonic bug fix (clears the rapid .fail corpus).

## What is intentionally NOT in this PR

- Full surface adapters (tmux per-pane options, desktop notifications, Termux, ntfy, Slack, Discord) — future
- Bubbletea TUI — future
- Per-message CRDT signature verification on GossipSub — future

## RFCs riding along

- docs/rfcs/adopt-libp2p.md (the decision)
- docs/rfcs/debate-libp2p-advocate.md
- docs/rfcs/debate-quic-mtls-advocate.md
- docs/rfcs/debate-yggdrasil-advocate.md

## Test plan

- [x] go build ./...
- [x] go test -race -count=1 ./...
- [x] go vet ./...
- [x] go test -fuzz=Fuzz -fuzztime=10s ./internal/crdt
- [ ] CI green
EOF
)"
\`\`\`

NO Claude attribution anywhere — not in commits, not in PR body, not in PR title. No 'Generated with Claude Code' footer. The PR body above is the literal body — do not append anything.

6. Capture the PR URL from the gh output.
7. \`git rev-parse HEAD\` to capture the head SHA.

Return per PUSH_SCHEMA.
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
2. \`gh pr checks ${BRANCH} --watch --interval 15\` — this blocks until all checks complete. Capture the exit code.
3. After it returns: \`gh pr checks ${BRANCH} --json name,state,conclusion,detailsUrl\` — parse to identify any failing checks.
4. If all checks have conclusion=SUCCESS: return status=green, failingChecks=[], logSummary="all green".
5. Otherwise: for each failing check, fetch the failed-job logs:
   - Find the run ID from the check URL.
   - \`gh run view <run-id> --log-failed | tail -200\`
   - Summarize the relevant error lines (test failures, vet errors, build errors).
6. Return per CI_SCHEMA. logSummary should be ≤ 4000 chars of the most actionable error context.

Do NOT push fixes from this agent — only report.
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

Log summary from the failing job(s):

\`\`\`
${(lastStatus.logSummary ?? '').slice(0, 4000)}
\`\`\`

Diagnose the root cause. Reproduce locally:
\`\`\`
cd ${REPO}
git pull --rebase origin ${BRANCH}
go build ./...
go test -race -count=1 ./...
go vet ./...
\`\`\`

If the local run reproduces the failure, fix it. If it does NOT reproduce (the failure is CI-specific, like missing tools or env), examine .github/workflows/ci.yml and adjust either the workflow or the code to satisfy the constraint without weakening test discipline (do NOT add t.Skip() to skip a failing test just to make CI green; if a test is environment-sensitive, gate it with \`if os.Getenv("CI") != "" { t.Skip(...) }\` only when the test cannot pass in CI by its nature — like real mDNS or real Unix domain sockets that require capabilities).

Make focused fix commits (one per logical change). NO Claude attribution.

After fixes:
\`\`\`
git push origin ${BRANCH}
\`\`\`

Return a short text summary of the commits made and a one-line theory of root cause.
`, { label: `ci-fix:${attempt}`, phase: 'CI loop' })
}

if (lastStatus?.status !== 'green') {
  log(`CI still not green after ${MAX_CI_ATTEMPTS} attempts. Stopping before merge.`)
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

CI is green. Merge with squash + delete branch:

\`\`\`
cd ${REPO}
gh pr merge ${BRANCH} --squash --delete-branch
git checkout main
git pull origin main
git log --oneline -5
\`\`\`

Capture the new HEAD commit on main.

Return per MERGE_SCHEMA. merged=true if the merge succeeded.

NO Claude attribution anywhere. The squash commit message gh creates from the PR body should already be clean — if it includes any attribution, edit it.
`, { schema: MERGE_SCHEMA, label: 'merge', phase: 'Merge' })

log(`Done. Main HEAD: ${merge?.mainHead ?? 'unknown'}`)

return {
  status: 'shipped',
  branch: BRANCH,
  prUrl: push.prUrl,
  ciAttempts: attempt,
  mainHead: merge?.mainHead,
}
