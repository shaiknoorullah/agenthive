# CI/CD, Release Pipeline & Developer Tooling Design

> **Status:** Approved
> **Date:** 2026-03-27
> **Scope:** Commit hooks, CI pipelines, changelog generation, release automation, multi-platform distribution

## Goal

Establish a complete, Node-free CI/CD and release pipeline for agenthive that enforces conventional commits, automates changelog generation, builds multi-platform binaries, and publishes to package managers — all triggered by merging a Release PR.

## Toolchain

| Tool | Role | Install |
|------|------|---------|
| **lefthook** (Go) | Git hook manager | `brew install lefthook` or `go install` |
| **cocogitto / cog** (Rust) | Commit linting + interactive prompts | `brew install cocogitto` or `cargo install` |
| **git-cliff** (Rust) | Changelog generation + semver computation | `brew install git-cliff` or `cargo install` |
| **GoReleaser** (Go) | Cross-compilation, packaging, publishing | `brew install goreleaser` or `go install` |

Zero Node.js or Python dependencies. All tools are single static binaries.

## 1. Local Developer Tooling

### Git Hooks (lefthook.yml)

| Hook | Command | Purpose |
|------|---------|---------|
| `commit-msg` | `cog verify --file {1}` | Reject non-conventional commit messages |
| `pre-commit` | `go vet ./...` + `golangci-lint run` | Catch lint issues before commit |
| `pre-push` | `go test -race -count=1 ./...` | Prevent pushing broken code |

### Cocogitto Configuration (cog.toml)

**Allowed commit types:**
- `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `style`, `chore`, `ci`, `build`, `revert`

**Allowed scopes:**
- `crdt`, `daemon`, `transport`, `hooks`, `tui`, `dispatch`, `ci`, `release`

Developers can use `cog commit` for interactive guided prompts or write conventional commits manually.

### git-cliff Configuration (cliff.toml)

- Tera template grouping commits by type (Features, Bug Fixes, Documentation, etc.)
- Commit links back to GitHub (`https://github.com/shaiknoorullah/agenthive/commit/<sha>`)
- Filters out `chore(release)` commits from changelog
- `--bumped-version` computes next semver from conventional commits:
  - `fix:` = patch bump
  - `feat:` = minor bump
  - `BREAKING CHANGE` or `!` = major bump

### Setup for Contributors

```bash
brew install lefthook cocogitto git-cliff
lefthook install
```

## 2. CI Pipeline Architecture

### Workflow Overview

| Workflow | File | Trigger | Purpose |
|----------|------|---------|---------|
| CI | `ci.yml` | PR to `develop`, push to `develop` | Test, lint, fuzz, vulnerability check |
| Release Gate | `release-gate.yml` | PR to `main` | Full validation before release |
| Release | `release.yml` | Push to `main` (merge) | Build, tag, publish |
| Release PR | `release-pr.yml` | Push to `develop` | Auto-create/update Release PR |
| Security | `security.yml` | Weekly cron + manual dispatch | Scheduled security scanning |

### ci.yml (PRs and pushes to develop)

**Jobs (all parallel):**

| Job | What it does |
|-----|-------------|
| `test` | `go build` + `go test -race` + `go vet` + coverage upload (matrix: ubuntu-latest + macos-latest, Go 1.22) |
| `lint` | `golangci-lint` via `golangci/golangci-lint-action` |
| `fuzz` | `go test -fuzz=Fuzz -fuzztime=30s ./internal/crdt/...` |
| `govulncheck` | `govulncheck ./...` (installed via `go install`) |
| `dependency-review` | `actions/dependency-review-action` (PRs only, blocks known-vulnerable deps) |

Coverage uploaded to Codecov from ubuntu leg only.

### release-gate.yml (PRs to main)

Superset of ci.yml with stricter checks:

| Job | What it does |
|-----|-------------|
| `test` | Same as ci.yml (matrix: ubuntu + macos) |
| `lint` | `golangci-lint` |
| `fuzz` | Extended: `go test -fuzz=Fuzz -fuzztime=2m` |
| `govulncheck` | `govulncheck ./...` |
| `gosec` | `gosec` with SARIF upload to GitHub Code Scanning |
| `codeql` | GitHub CodeQL semantic analysis |
| `cross-compile` | Build verification for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 (output to /dev/null) |

### release-pr.yml (pushes to develop)

Triggered on every push to `develop`. Auto-creates or updates a Release PR targeting `main`.

**Behavior:**
1. Runs `git cliff --bumped-version` to compute next semver
2. Runs `git cliff --unreleased --tag <version>` to generate changelog preview
3. If no Release PR exists, creates one with title `release: v<version>` and changelog as body
4. If Release PR exists, updates the title and body with current version/changelog
5. Release-gate checks run automatically on the PR

The Release PR stays open and auto-updates. It always shows "here's what the next release looks like."

### release.yml (push to main / merge)

Triggered when the Release PR is merged into `main`. Uses a commit message filter to avoid re-triggering on its own changelog commit.

**Guard:** The workflow checks the HEAD commit message. If it starts with `chore(release):`, the workflow exits early (this is the changelog commit pushed by a previous release run). This prevents infinite loops.

**Steps (sequential):**
1. Check HEAD commit — skip if `chore(release):` (self-push guard)
2. `git cliff --bumped-version` computes the version
3. `git cliff --tag <version> --output CHANGELOG.md` finalizes the changelog
4. `git cliff --latest --tag <version> --strip header` generates release notes to `RELEASE_NOTES.md`
5. Commit CHANGELOG.md update: `chore(release): v<version>`
6. Create git tag: `v<version>`
7. Push tag and changelog commit
8. GoReleaser runs with `--release-notes RELEASE_NOTES.md`
9. GoReleaser builds all platform binaries, creates GitHub Release, publishes to Homebrew/AUR

### security.yml (weekly + manual)

| Job | What it does |
|-----|-------------|
| `govulncheck` | `govulncheck ./...` |
| `gosec` | `gosec` with SARIF upload |
| `scorecard` | OSSF Scorecard repo security posture assessment |

Schedule: Monday 08:00 UTC. Also available via `workflow_dispatch` for on-demand runs.

### Dependabot Configuration

All ecosystems target `develop`:

| Ecosystem | Interval | What it covers |
|-----------|----------|----------------|
| `gomod` | Weekly (Monday) | Go module dependencies |
| `github-actions` | Weekly (Monday) | Action version bumps |
| `docker` | Weekly (Monday) | Dockerfile base images (when added) |

Labels: `dependencies` + ecosystem-specific (`go`, `ci`, `docker`).

## 3. Release Artifacts & Distribution

### GoReleaser Configuration (.goreleaser.yaml)

**Build targets:**

| OS | Arch | CGO | Notes |
|----|------|-----|-------|
| linux | amd64 | disabled | Primary |
| linux | arm64 | disabled | ARM servers + Termux initial |
| darwin | amd64 | disabled | Intel Mac |
| darwin | arm64 | disabled | Apple Silicon |

`CGO_ENABLED=0` for all builds. Termux NDK + CGO build added later if DNS issues surface.

**Ldflags:** `-s -w -X main.version={{.Version}} -X main.commit={{.Commit}}`

### Distribution Channels

| Channel | Priority | Mechanism | User install command |
|---------|----------|-----------|---------------------|
| GitHub Releases | P0 | GoReleaser (automatic) | `curl -LO` + extract |
| Homebrew tap | P0 | GoReleaser `brews` section -> `shaiknoorullah/homebrew-agenthive` | `brew install shaiknoorullah/agenthive/agenthive` |
| .deb package | P1 | GoReleaser `nfpms` section | `dpkg -i agenthive_*.deb` |
| .rpm package | P1 | GoReleaser `nfpms` section | `rpm -i agenthive-*.rpm` |
| AUR | P1 | GoReleaser `aurs` section -> `aur.archlinux.org/agenthive-bin` | `yay -S agenthive-bin` |
| Termux .deb | P1 | nfpm termux.deb format | `dpkg -i` inside Termux |
| TPM auto-download | P0 | `scripts/install.sh` in tmux plugin | `prefix + I` (TPM install) |

### TPM Plugin Binary Distribution

The `.tmux` initialization script:
1. Checks for `agenthive` binary in `$PATH`
2. Falls back to `$PLUGIN_DIR/bin/agenthive`
3. If not found, runs `scripts/install.sh` which:
   - Detects OS (`uname -s`) and arch (`uname -m`)
   - Downloads correct binary from latest GitHub Release
   - Places it in `$PLUGIN_DIR/bin/`
4. On version mismatch, offers to upgrade

### Supply Chain Security

- **Cosign keyless signing** on checksums file (Sigstore/Fulcio OIDC)
- Users verify with `cosign verify-blob`
- All CI scanning tools installed via `go install` (not third-party Actions)
- No Trivy (supply chain compromised March 2026)

## 4. Security & Static Analysis

### Analysis Stack

| Tool | CI Stage | What it catches |
|------|----------|-----------------|
| `golangci-lint` | ci.yml, release-gate.yml | 50+ Go linters (style, bugs, complexity, dead code) |
| `govulncheck` | ci.yml, release-gate.yml, security.yml | Known vulnerabilities in dependencies (official Go vuln DB) |
| `gosec` + SARIF | release-gate.yml, security.yml | Hardcoded credentials, injection, weak crypto |
| `CodeQL` | release-gate.yml | Semantic/logic-level bugs, auth bypass, taint analysis |
| `OSSF Scorecard` | security.yml (weekly) | Repo security hygiene score |
| `dependency-review-action` | ci.yml (PRs only) | Block PRs introducing known-vulnerable dependencies |
| `cosign` | release.yml | Binary/checksum signing for supply chain verification |

### Branch Protection Rules

| Rule | `develop` | `main` |
|------|-----------|--------|
| Require PR | Yes | Yes |
| Required status checks | test (ubuntu), test (macos), lint, govulncheck | All develop checks + fuzz (2m), gosec, CodeQL, cross-compile (5 targets) |
| Require up-to-date before merge | Yes | Yes |
| Force push | Blocked | Blocked |
| Branch deletion | Blocked | Blocked |

## 5. Merge Strategy

- **Feature branches -> develop:** Squash or merge commit (contributor's choice)
- **develop -> main (Release PR):** Always **merge commit**. Individual commits must be preserved so git-cliff can generate grouped changelogs from conventional commit types.

## 6. Release Flow (End-to-End)

```
Developer workflow:
  git checkout -b feat/my-feature develop
  cog commit              # interactive prompt
  git push                # pre-push hook runs tests
  # open PR to develop

After merge to develop:
  release-pr.yml triggers
  -> computes next version (git cliff --bumped-version)
  -> creates/updates Release PR to main
  -> PR body shows changelog preview

When ready to release:
  Maintainer merges Release PR
  -> release.yml triggers
  -> git-cliff finalizes CHANGELOG.md
  -> creates git tag (v0.1.0)
  -> GoReleaser cross-compiles all targets
  -> uploads binaries, .deb, .rpm to GitHub Release
  -> pushes Homebrew formula to tap repo
  -> pushes PKGBUILD to AUR
  -> cosign signs checksums
```

## 7. Signed Commits (GPG)

All commits must be GPG-signed for verified badges on GitHub.

**Local setup:**
- Generate a GPG key (or use existing): `gpg --full-generate-key` (RSA 4096, email matching GitHub)
- Add public key to GitHub account (Settings > SSH and GPG keys)
- Configure git:
  ```
  git config --global user.signingkey <KEY_ID>
  git config --global commit.gpgsign true
  git config --global tag.gpgsign true
  ```

**CI commits (changelog, release tags):**
- The `release.yml` workflow creates commits (`chore(release):`) and tags
- Use a dedicated GPG key stored as `GPG_PRIVATE_KEY` and `GPG_PASSPHRASE` repository secrets
- Import the key in CI: `echo "$GPG_PRIVATE_KEY" | gpg --batch --import`
- Tags created by GoReleaser are also signed

**Enforcement:**
- Branch protection rules on `develop` and `main`: require signed commits (vigilant mode)
- Unsigned commits are rejected at push time

## 8. License Change (MIT -> BSL 1.1)

The project license changes from MIT to Business Source License 1.1.

**Parameters:**

| Parameter | Value |
|-----------|-------|
| Licensor | Shaik Noorullah |
| Licensed Work | agenthive (all versions from this change onward) |
| Additional Use Grant | Production use is permitted for any purpose **except** offering agenthive as a hosted or managed service that competes with the Licensor's commercial offerings. Self-hosted, single-organization use is unrestricted. |
| Change Date | 4 years from each release date |
| Change License | Apache License 2.0 |

**What this means:**
- Developers and companies can freely use, modify, and self-host agenthive
- Contributors can fork, submit PRs, and build on it
- Nobody can take the code and launch a competing hosted service
- After 4 years, each version automatically becomes Apache 2.0

**Files to update:**
- `LICENSE` — replace MIT with BSL 1.1 text
- `README.md` — update license badge and footer
- `CONTRIBUTING.md` — update license section
- `.goreleaser.yaml` — update license field in nfpm/brews sections

**Note:** Prior commits remain under MIT. The license change applies from the commit that introduces the new LICENSE file onward.

## 9. Configuration Files Summary

| File | Purpose |
|------|---------|
| `lefthook.yml` | Git hook definitions (commit-msg, pre-commit, pre-push) |
| `cog.toml` | Cocogitto config (allowed types, scopes) |
| `cliff.toml` | git-cliff config (changelog template, commit parsers, bump rules) |
| `.goreleaser.yaml` | GoReleaser config (builds, archives, nfpm, brews, aurs, signing) |
| `.github/workflows/ci.yml` | CI pipeline for develop |
| `.github/workflows/release-gate.yml` | Full validation for PRs to main |
| `.github/workflows/release.yml` | Release pipeline (tag + build + publish) |
| `.github/workflows/release-pr.yml` | Auto Release PR management |
| `.github/workflows/security.yml` | Weekly security scanning |
| `.github/dependabot.yml` | Dependency update automation |
| `codecov.yml` | Coverage thresholds |
| `.golangci.yml` | Linter configuration |
| `scripts/install.sh` | TPM binary auto-download script |
