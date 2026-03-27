# Contributing to agenthive

Thanks for your interest in contributing to agenthive.

## Getting Started

1. Fork the repo and create your branch from `develop`
2. Set up locally:

```bash
git clone https://github.com/YOUR_USERNAME/agenthive.git
cd agenthive
git checkout develop
go mod download
go build ./...
go test -race ./...
```

## Branch Strategy

We use a two-branch model:

- **`main`** -- stable releases only. Never commit directly to main.
- **`develop`** -- integration branch. Never commit directly to develop.

Every change -- features, fixes, docs, tests, refactors, chores -- goes through a branch and PR. No exceptions.

When `develop` is stable and tested, it gets merged into `main` via PR as a release.

```
main      ----o--------------------------o---- (stable releases)
               \                        /
develop    -----o---o---o---o---o---o---o----- (integration)
                 \     / \     / \     /
branches          o---o   o---o   o---o
```

### Merge Strategy

- **Feature branches -> develop:** Squash or merge commit (contributor's choice)
- **develop -> main (Release PR):** Always **merge commit** (no squash, no rebase). Individual commits must be preserved so git-cliff can generate grouped changelogs from conventional commit types.

### Release Flow

Releases are automated via a Release PR:

1. As commits land on `develop`, CI auto-creates/updates a Release PR targeting `main`
2. The PR shows: computed next version, changelog preview, commit list
3. Release-gate checks run automatically on the PR (full test suite + cross-compile + security)
4. When ready to ship, merge the Release PR
5. Merge to `main` triggers the release pipeline: changelog finalized, tag created, binaries built, GitHub Release published

### Workflow

1. Create a branch off `develop`: `git checkout -b <type>/description develop`
2. Write tests first (we use TDD)
3. Implement your changes
4. Run the full suite: `go test -race -count=1 ./...`
5. Commit with a clear message
6. Push and open a PR targeting `develop`
7. After review and CI passes, merge into `develop`

Branch naming: `feat/`, `fix/`, `docs/`, `test/`, `refactor/`, `chore/`, `build/`, `ci/`, `perf/`, `style/`, `revert/` prefixes.

**No direct commits to `develop` or `main`.** The only exception: hotfixes for production issues can PR directly to `main`.

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/). Commit messages are validated by a `commit-msg` hook via lefthook + cocogitto.

Use `cog commit` for interactive guided prompts, or write manually:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Types:**

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `test:` adding or updating tests
- `refactor:` code restructuring (no behavior change)
- `perf:` performance improvement
- `style:` formatting, whitespace (no code change)
- `chore:` maintenance, dependencies, tooling
- `ci:` CI/CD configuration
- `build:` build system, compilation, packaging
- `revert:` reverting a previous commit

**Scopes** (optional): `crdt`, `daemon`, `transport`, `hooks`, `tui`, `dispatch`, `ci`, `release`

**Breaking changes:** Add `!` after the type/scope (e.g., `feat(transport)!: change envelope format`) or include `BREAKING CHANGE:` in the footer.

## Architecture Decisions

Before proposing architectural changes, read the design documents in `docs/rfcs/`. Major decisions were made through adversarial debate analysis. If you disagree with a decision, open an issue with your reasoning.

| Document | Covers |
|----------|--------|
| `docs/rfcs/debate-judgment.md` | Local notification architecture |
| `docs/rfcs/debate-transport-judgment.md` | Transport layer and mesh topology |
| `docs/rfcs/action-buttons-research.md` | Bidirectional action system |
| `docs/rfcs/feature-research.md` | Feature roadmap |

## Testing Requirements

- New code must have tests
- `go test -race ./...` must pass
- CRDT changes require property tests (commutativity, associativity, idempotency)
- We use `testify` for assertions, `rapid` for property-based tests, and Go's native fuzzing

## Code Style

- Standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused
- Prefer clear names over comments

## Reporting Issues

- **Bugs**: use the bug report template. Include OS, tmux version, steps to reproduce.
- **Features**: use the feature request template. Check `docs/rfcs/feature-research.md` first.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
