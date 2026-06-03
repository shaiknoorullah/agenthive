# Contributing to agenthive

Thanks for your interest in contributing to agenthive.

## Getting Started

1. Fork the repo and create your branch from `main`
2. Set up locally:

```bash
git clone https://github.com/YOUR_USERNAME/agenthive.git
cd agenthive
go mod download
go build ./...
go test -race ./...
```

## Development Workflow

1. Create a feature branch: `git checkout -b feat/my-feature`
2. Write tests first (we use TDD)
3. Implement your changes
4. Run the full suite: `go test -race -count=1 ./...`
5. Commit with a clear message
6. Push and open a pull request

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `test:` adding or updating tests
- `refactor:` code restructuring
- `chore:` maintenance

## Architecture Decisions

Before proposing architectural changes, read the design documents in `docs/rfcs/`. Major decisions were made through adversarial debate analysis — each option gets an advocate paper, a judge synthesizes, the verdict is filed alongside. If you disagree with a decision, open an issue with your reasoning.

| Document | Covers |
|----------|--------|
| `docs/rfcs/adopt-libp2p.md` | Transport, identity, discovery, NAT (current) |
| `docs/rfcs/debate-libp2p-advocate.md` | Adversary paper that won the transport debate |
| `docs/rfcs/debate-quic-mtls-advocate.md` | Adversary paper (QUIC + mTLS option) |
| `docs/rfcs/debate-yggdrasil-advocate.md` | Adversary paper (IPv6 overlay mesh option) |
| `docs/rfcs/debate-transport-judgment.md` | Original SSH+gossip judgment (superseded by `adopt-libp2p.md`) |
| `docs/rfcs/debate-judgment.md` | Local notification architecture (Phase 1: native tmux options) |
| `docs/rfcs/action-buttons-research.md` | Bidirectional action approval system |
| `docs/rfcs/feature-research.md` | Feature roadmap |
| `docs/rfcs/code-analysis.md` | Lessons from the shell-based predecessor |

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

- **Bugs**: use the bug report template. Include OS, Go version, agenthive version, and steps to reproduce. tmux version only if you're exercising the (planned) tmux surface.
- **Features**: use the feature request template. Check `docs/rfcs/feature-research.md` first; that's where the roadmap lives.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
