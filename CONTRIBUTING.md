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

### Workflow

1. Create a branch off `develop`: `git checkout -b <type>/description develop`
2. Write tests first (we use TDD)
3. Implement your changes
4. Run the full suite: `go test -race -count=1 ./...`
5. Commit with a clear message
6. Push and open a PR targeting `develop`
7. After review and CI passes, merge into `develop`

Branch naming: `feat/`, `fix/`, `docs/`, `test/`, `refactor/`, `chore/` prefixes.

**No direct commits to `develop` or `main`.** The only exception: hotfixes for production issues can PR directly to `main`.

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `test:` adding or updating tests
- `refactor:` code restructuring
- `chore:` maintenance

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
