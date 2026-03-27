# CI/CD, Release Pipeline & Developer Tooling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the complete CI/CD pipeline, release automation, commit hooks, changelog generation, multi-platform distribution, GPG signing, and license change for agenthive.

**Architecture:** lefthook for git hooks, cocogitto for commit linting + interactive prompts, git-cliff for changelog + semver, GoReleaser for cross-compilation + publishing. 5 GitHub Actions workflows (ci, release-gate, release, release-pr, security). Auto Release PR flow: commits on develop auto-update a Release PR to main; merging it triggers tag + build + publish.

**Tech Stack:** lefthook, cocogitto (cog), git-cliff, GoReleaser, GitHub Actions, Codecov, gosec, govulncheck, CodeQL, OSSF Scorecard, cosign, GPG

**Spec:** `docs/superpowers/specs/2026-03-27-cicd-release-pipeline-design.md`

**Constraints:**
- Never commit directly to `develop` or `main` — use feature branches + PRs
- Never add Co-Authored-By lines to commits
- Use SSH key at `~/.ssh/noorullah_github` for git push: `GIT_SSH_COMMAND='ssh -i ~/.ssh/noorullah_github -o IdentitiesOnly=yes'`

---

## File Structure

### New Files
- `lefthook.yml` — git hook definitions
- `cog.toml` — cocogitto config (commit types, scopes)
- `cliff.toml` — git-cliff config (changelog template, commit parsers)
- `.goreleaser.yaml` — GoReleaser config (builds, archives, nfpm, brews, signing)
- `.golangci.yml` — golangci-lint config
- `codecov.yml` — coverage thresholds
- `scripts/install.sh` — TPM binary auto-download script
- `.github/workflows/release-gate.yml` — full validation for PRs to main
- `.github/workflows/release.yml` — release pipeline (tag + build + publish)
- `.github/workflows/release-pr.yml` — auto Release PR management
- `.github/workflows/security.yml` — weekly security scanning

### Modified Files
- `.github/workflows/ci.yml` — retarget to develop, add govulncheck + dependency-review + coverage
- `.github/dependabot.yml` — target develop, add docker ecosystem, add labels
- `LICENSE` — replace MIT with BSL 1.1
- `README.md` — update license badge
- `CONTRIBUTING.md` — update license section

---

### Task 1: Create Feature Branch and Install Tools

**Files:**
- None (setup only)

- [ ] **Step 1: Create feature branch from develop**

```bash
cd /home/devsupreme/agenthive
git checkout develop
git pull origin develop
git checkout -b ci/pipeline-and-release-tooling develop
```

- [ ] **Step 2: Verify tools are installed**

```bash
lefthook --version
cog --version
git cliff --version
goreleaser --version
```

Expected: version output for each. If any missing, install via:
```bash
brew install lefthook cocogitto git-cliff goreleaser
```

---

### Task 2: License Change (MIT -> BSL 1.1)

**Files:**
- Modify: `LICENSE`
- Modify: `README.md:10`
- Modify: `CONTRIBUTING.md:128-130`

- [ ] **Step 1: Replace LICENSE file with BSL 1.1**

Replace the entire contents of `LICENSE` with the Business Source License 1.1 text. Use the canonical BSL 1.1 template from https://mariadb.com/bsl11/ with these parameters:

- **Licensor:** Shaik Noorullah
- **Licensed Work:** agenthive
- **Additional Use Grant:** Production use is permitted for any purpose except offering the Licensed Work as a hosted or managed service that competes with the Licensor's commercial offerings. Self-hosted, single-organization use is unrestricted.
- **Change Date:** Four years from the date of each release of the Licensed Work
- **Change License:** Apache License, Version 2.0

Fetch the template:
```bash
curl -sL 'https://mariadb.com/bsl11/' | head -80
```

Then write the LICENSE file with the parameters filled in.

- [ ] **Step 2: Update README license badge**

In `README.md`, replace:
```
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
```
With:
```
[![License: BSL 1.1](https://img.shields.io/badge/license-BSL%201.1-orange.svg)](LICENSE)
```

- [ ] **Step 3: Update CONTRIBUTING.md license section**

In `CONTRIBUTING.md`, replace:
```
By contributing, you agree that your contributions will be licensed under the MIT License.
```
With:
```
By contributing, you agree that your contributions will be licensed under the Business Source License 1.1 (BSL 1.1). After 4 years from each release date, that version automatically converts to the Apache License 2.0. See [LICENSE](LICENSE) for full terms.
```

- [ ] **Step 4: Commit license change**

```bash
git add LICENSE README.md CONTRIBUTING.md
git commit -m "chore: change license from MIT to BSL 1.1

Production use is permitted except for competing hosted services.
Each release converts to Apache 2.0 after 4 years."
```

---

### Task 3: Lefthook Configuration

**Files:**
- Create: `lefthook.yml`

- [ ] **Step 1: Create lefthook.yml**

```yaml
# https://github.com/evilmartians/lefthook

commit-msg:
  commands:
    conventional-commit:
      run: cog verify --file {1}

pre-commit:
  parallel: true
  commands:
    go-vet:
      glob: "*.go"
      run: go vet ./...
    golangci-lint:
      glob: "*.go"
      run: golangci-lint run

pre-push:
  commands:
    tests:
      run: go test -race -count=1 ./...
```

- [ ] **Step 2: Install hooks in local repo**

```bash
lefthook install
```

Expected: `lefthook installed` or similar success message.

- [ ] **Step 3: Verify commit-msg hook works**

Test with a bad commit message:
```bash
echo "test" > /tmp/bad-commit-msg
cog verify --file /tmp/bad-commit-msg; echo "exit: $?"
```

Expected: exit code 1, error about invalid conventional commit.

Test with a good commit message:
```bash
echo "feat: test message" > /tmp/good-commit-msg
cog verify --file /tmp/good-commit-msg; echo "exit: $?"
```

Expected: exit code 0.

- [ ] **Step 4: Commit lefthook config**

```bash
git add lefthook.yml
git commit -m "ci: add lefthook git hooks for commit linting and pre-push tests"
```

---

### Task 4: Cocogitto Configuration

**Files:**
- Create: `cog.toml`

- [ ] **Step 1: Create cog.toml**

```toml
# Cocogitto configuration
# https://docs.cocogitto.io/

[changelog]
path = "CHANGELOG.md"
authors = []

[commit_types]
feat = { changelog_title = "Features" }
fix = { changelog_title = "Bug Fixes" }
docs = { changelog_title = "Documentation" }
test = { changelog_title = "Tests" }
refactor = { changelog_title = "Refactoring" }
perf = { changelog_title = "Performance" }
style = { changelog_title = "Style" }
chore = { changelog_title = "Miscellaneous" }
ci = { changelog_title = "CI/CD" }
build = { changelog_title = "Build" }
revert = { changelog_title = "Reverts" }

[allowed_scopes]
list = [
    "crdt",
    "daemon",
    "transport",
    "hooks",
    "tui",
    "dispatch",
    "ci",
    "release",
]
```

- [ ] **Step 2: Verify cog check passes on recent commits**

```bash
cog check --from-latest-tag 2>&1 || cog check 2>&1 | tail -5
```

Note: existing commits may not all pass since they predate cog. This is expected. Future commits will be validated by the hook.

- [ ] **Step 3: Commit cocogitto config**

```bash
git add cog.toml
git commit -m "ci: add cocogitto config for conventional commit types and scopes"
```

---

### Task 5: git-cliff Configuration

**Files:**
- Create: `cliff.toml`

- [ ] **Step 1: Create cliff.toml**

```toml
# git-cliff configuration
# https://git-cliff.org/docs/configuration

[changelog]
header = """
# Changelog\n
All notable changes to this project will be documented in this file.\n
"""
body = """
{%- macro remote_url() -%}
  https://github.com/shaiknoorullah/agenthive
{%- endmacro -%}

{% if version -%}
    ## [{{ version | trim_start_matches(pat="v") }}] - {{ timestamp | date(format="%Y-%m-%d") }}
{% else -%}
    ## [unreleased]
{% endif -%}

{% for group, commits in commits | group_by(attribute="group") %}
    ### {{ group | striptags | trim | upper_first }}
    {% for commit in commits %}
        - {% if commit.scope %}*({{ commit.scope }})* {% endif -%}
            {% if commit.breaking %}[**breaking**] {% endif -%}
            {{ commit.message | upper_first }} \
            ([{{ commit.id | truncate(length=7, end="") }}]({{ self::remote_url() }}/commit/{{ commit.id }}))\
    {% endfor %}
{% endfor %}
"""
footer = """
{% for release in releases -%}
    {% if release.version -%}
        {% if release.previous.version -%}
            [{{ release.version | trim_start_matches(pat="v") }}]: \
                {{ self::remote_url() }}/compare/{{ release.previous.version }}...{{ release.version }}
        {% endif -%}
    {% else -%}
        [unreleased]: {{ self::remote_url() }}/compare/{{ release.previous.version }}...HEAD
    {% endif -%}
{% endfor %}
"""
trim = true

[git]
conventional_commits = true
filter_unconventional = true
split_commits = false
commit_parsers = [
    { message = "^feat", group = "Features" },
    { message = "^fix", group = "Bug Fixes" },
    { message = "^docs", group = "Documentation" },
    { message = "^perf", group = "Performance" },
    { message = "^refactor", group = "Refactoring" },
    { message = "^style", group = "Style" },
    { message = "^test", group = "Testing" },
    { message = "^ci", group = "CI/CD" },
    { message = "^build", group = "Build" },
    { message = "^revert", group = "Reverts" },
    { message = "^chore\\(release\\)", skip = true },
    { message = "^chore", group = "Miscellaneous" },
]
protect_breaking_commits = false
filter_commits = false
tag_pattern = "v[0-9].*"
sort_commits = "oldest"

[bump]
features_always_bump_minor = true
breaking_always_bump_major = true
```

- [ ] **Step 2: Test changelog generation**

```bash
git cliff --unreleased
```

Expected: markdown output grouping recent commits by type (Features, Bug Fixes, etc.)

- [ ] **Step 3: Test version computation**

```bash
git cliff --bumped-version
```

Expected: a semver like `v0.1.0` or `v0.2.0` (depends on commit types since last tag, or `v0.1.0` if no tags exist).

- [ ] **Step 4: Commit git-cliff config**

```bash
git add cliff.toml
git commit -m "ci: add git-cliff config for changelog generation and version bumping"
```

---

### Task 6: golangci-lint Configuration

**Files:**
- Create: `.golangci.yml`

- [ ] **Step 1: Create .golangci.yml**

```yaml
run:
  timeout: 5m

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gocritic
    - gofmt
    - goimports
    - misspell
    - unconvert
    - unparam
    - bodyclose
    - noctx
    - prealloc

linters-settings:
  gocritic:
    enabled-tags:
      - diagnostic
      - performance
    disabled-tags:
      - style
      - experimental
      - opinionated
  misspell:
    locale: US

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck
        - unparam
```

- [ ] **Step 2: Run golangci-lint to verify config**

```bash
golangci-lint run ./...
```

Expected: either clean output or lint warnings (no config errors).

- [ ] **Step 3: Commit golangci-lint config**

```bash
git add .golangci.yml
git commit -m "ci: add golangci-lint configuration"
```

---

### Task 7: Codecov Configuration

**Files:**
- Create: `codecov.yml`

- [ ] **Step 1: Create codecov.yml**

```yaml
coverage:
  status:
    project:
      default:
        target: auto
        threshold: 2%
    patch:
      default:
        target: 80%

comment:
  layout: "reach,diff,flags,files"
  behavior: default
  require_changes: true

ignore:
  - "docs/**"
  - "scripts/**"
  - "**/*_test.go"
```

- [ ] **Step 2: Commit codecov config**

```bash
git add codecov.yml
git commit -m "ci: add codecov coverage thresholds"
```

---

### Task 8: CI Workflow (ci.yml)

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Replace ci.yml with develop-targeted version**

Replace the entire contents of `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [develop]
  pull_request:
    branches: [develop]

permissions:
  contents: read

jobs:
  test:
    name: Test (Go ${{ matrix.go }}, ${{ matrix.os }})
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
        go: ['1.22']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
      - run: go build ./...
      - run: go test -race -count=1 -coverprofile=coverage.txt -covermode=atomic ./...
      - run: go vet ./...
      - name: Upload coverage
        if: matrix.os == 'ubuntu-latest'
        uses: codecov/codecov-action@v5
        with:
          files: coverage.txt
          flags: unittests
          fail_ci_if_error: false

  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  fuzz:
    name: Fuzz
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test -fuzz=Fuzz -fuzztime=30s ./internal/crdt/...

  govulncheck:
    name: Vulnerability Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest
      - run: govulncheck ./...

  dependency-review:
    name: Dependency Review
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
      - uses: actions/dependency-review-action@v4
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML OK"
```

Expected: `YAML OK`

- [ ] **Step 3: Commit ci.yml**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: retarget CI to develop, add govulncheck and dependency-review"
```

---

### Task 9: Release Gate Workflow

**Files:**
- Create: `.github/workflows/release-gate.yml`

- [ ] **Step 1: Create release-gate.yml**

```yaml
name: Release Gate

on:
  pull_request:
    branches: [main]

permissions:
  contents: read
  security-events: write

jobs:
  test:
    name: Test (Go ${{ matrix.go }}, ${{ matrix.os }})
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
        go: ['1.22']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
      - run: go build ./...
      - run: go test -race -count=1 -coverprofile=coverage.txt -covermode=atomic ./...
      - run: go vet ./...

  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  fuzz:
    name: Fuzz (extended)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test -fuzz=Fuzz -fuzztime=2m ./internal/crdt/...

  govulncheck:
    name: Vulnerability Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest
      - run: govulncheck ./...

  gosec:
    name: Security Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Install gosec
        run: go install github.com/securego/gosec/v2/cmd/gosec@latest
      - name: Run gosec
        run: gosec -fmt sarif -out gosec.sarif ./...
      - name: Upload SARIF
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: gosec.sarif

  codeql:
    name: CodeQL
    runs-on: ubuntu-latest
    permissions:
      security-events: write
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: github/codeql-action/init@v3
        with:
          languages: go
      - uses: github/codeql-action/autobuild@v3
      - uses: github/codeql-action/analyze@v3

  cross-compile:
    name: Cross-Compile (${{ matrix.goos }}/${{ matrix.goarch }})
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        include:
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
          - goos: darwin
            goarch: amd64
          - goos: darwin
            goarch: arm64
          - goos: windows
            goarch: amd64
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          CGO_ENABLED: '0'
        run: go build -o /dev/null ./...
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release-gate.yml'))" && echo "YAML OK"
```

Expected: `YAML OK`

- [ ] **Step 3: Commit release-gate workflow**

```bash
git add .github/workflows/release-gate.yml
git commit -m "ci: add release-gate workflow with extended fuzz, gosec, CodeQL, and cross-compile"
```

---

### Task 10: Security Workflow

**Files:**
- Create: `.github/workflows/security.yml`

- [ ] **Step 1: Create security.yml**

```yaml
name: Security

on:
  schedule:
    - cron: '0 8 * * 1'
  workflow_dispatch:

permissions:
  contents: read
  security-events: write

jobs:
  govulncheck:
    name: Vulnerability Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest
      - run: govulncheck ./...

  gosec:
    name: Security Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Install gosec
        run: go install github.com/securego/gosec/v2/cmd/gosec@latest
      - name: Run gosec
        run: gosec -fmt sarif -out gosec.sarif ./...
      - name: Upload SARIF
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: gosec.sarif

  scorecard:
    name: OSSF Scorecard
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@v4
        with:
          persist-credentials: false
      - uses: ossf/scorecard-action@v2
        with:
          results_file: scorecard.sarif
          results_format: sarif
          publish_results: true
      - name: Upload SARIF
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: scorecard.sarif
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/security.yml'))" && echo "YAML OK"
```

Expected: `YAML OK`

- [ ] **Step 3: Commit security workflow**

```bash
git add .github/workflows/security.yml
git commit -m "ci: add weekly security workflow with govulncheck, gosec, and OSSF Scorecard"
```

---

### Task 11: Release PR Workflow

**Files:**
- Create: `.github/workflows/release-pr.yml`

- [ ] **Step 1: Create release-pr.yml**

```yaml
name: Release PR

on:
  push:
    branches: [develop]

permissions:
  contents: write
  pull-requests: write

jobs:
  release-pr:
    name: Update Release PR
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Install git-cliff
        uses: taiki-e/install-action@git-cliff

      - name: Compute next version
        id: version
        run: |
          NEXT=$(git cliff --bumped-version 2>/dev/null || echo "v0.1.0")
          echo "version=$NEXT" >> "$GITHUB_OUTPUT"
          echo "Computed next version: $NEXT"

      - name: Generate changelog preview
        run: |
          VERSION=${{ steps.version.outputs.version }}
          git cliff --unreleased --tag "$VERSION" --strip header > RELEASE_BODY.md
          echo "" >> RELEASE_BODY.md
          echo "---" >> RELEASE_BODY.md
          echo "" >> RELEASE_BODY.md
          echo "_This PR was auto-generated. Merge it to create release \`$VERSION\`._" >> RELEASE_BODY.md

      - name: Create or update Release PR
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          VERSION=${{ steps.version.outputs.version }}
          TITLE="release: $VERSION"
          BODY=$(cat RELEASE_BODY.md)

          # Check if a release PR already exists
          EXISTING_PR=$(gh pr list --base main --head develop --json number --jq '.[0].number // empty')

          if [ -n "$EXISTING_PR" ]; then
            echo "Updating existing PR #$EXISTING_PR"
            gh pr edit "$EXISTING_PR" --title "$TITLE" --body "$BODY"
          else
            echo "Creating new Release PR"
            gh pr create --base main --head develop --title "$TITLE" --body "$BODY"
          fi
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release-pr.yml'))" && echo "YAML OK"
```

Expected: `YAML OK`

- [ ] **Step 3: Commit release-pr workflow**

```bash
git add .github/workflows/release-pr.yml
git commit -m "ci: add release-pr workflow for auto-managed Release PRs"
```

---

### Task 12: Release Workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create release.yml**

```yaml
name: Release

on:
  push:
    branches: [main]

permissions:
  contents: write
  id-token: write

jobs:
  release:
    name: Release
    runs-on: ubuntu-latest
    # Skip if this push is from a release commit (prevent infinite loop)
    if: "!startsWith(github.event.head_commit.message, 'chore(release):')"
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          token: ${{ secrets.RELEASE_TOKEN }}

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install git-cliff
        uses: taiki-e/install-action@git-cliff

      - name: Import GPG key
        env:
          GPG_PRIVATE_KEY: ${{ secrets.GPG_PRIVATE_KEY }}
          GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}
        run: |
          echo "$GPG_PRIVATE_KEY" | gpg --batch --import
          GPG_KEY_ID=$(gpg --list-secret-keys --keyid-format long | grep sec | head -1 | awk '{print $2}' | cut -d'/' -f2)
          git config user.signingkey "$GPG_KEY_ID"
          git config commit.gpgsign true
          git config tag.gpgsign true

      - name: Configure git
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"

      - name: Compute version
        id: version
        run: |
          VERSION=$(git cliff --bumped-version 2>/dev/null || echo "v0.1.0")
          echo "version=$VERSION" >> "$GITHUB_OUTPUT"
          echo "Release version: $VERSION"

      - name: Generate changelog
        run: |
          VERSION=${{ steps.version.outputs.version }}
          git cliff --tag "$VERSION" --output CHANGELOG.md
          git cliff --latest --tag "$VERSION" --strip header > RELEASE_NOTES.md

      - name: Commit changelog and create tag
        env:
          GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}
        run: |
          VERSION=${{ steps.version.outputs.version }}
          git add CHANGELOG.md
          git commit -S -m "chore(release): $VERSION"
          git tag -s "$VERSION" -m "Release $VERSION"
          git push origin main --tags

      - name: Install cosign
        uses: sigstore/cosign-installer@v3

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean --release-notes RELEASE_NOTES.md
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

**Note:** `secrets.RELEASE_TOKEN` is a PAT (not the default `GITHUB_TOKEN`) because the release workflow pushes a commit back to `main`. The default `GITHUB_TOKEN` cannot trigger subsequent workflows, and a PAT with `repo` scope is needed to push to a protected branch from CI.

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))" && echo "YAML OK"
```

Expected: `YAML OK`

- [ ] **Step 3: Commit release workflow**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow with git-cliff, GPG signing, cosign, and GoReleaser"
```

---

### Task 13: GoReleaser Configuration

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Create .goreleaser.yaml**

```yaml
version: 2

builds:
  - env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w -X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: 'checksums.txt'

signs:
  - cmd: cosign
    signature: "${artifact}.sig"
    certificate: "${artifact}.pem"
    args:
      - "sign-blob"
      - "--output-signature=${signature}"
      - "--output-certificate=${certificate}"
      - "${artifact}"
      - "--yes"
    artifacts: checksum

changelog:
  disable: true  # We use git-cliff for changelog generation

brews:
  - name: agenthive
    repository:
      owner: shaiknoorullah
      name: homebrew-agenthive
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    homepage: "https://github.com/shaiknoorullah/agenthive"
    description: "Encrypted P2P mesh for AI agent notification and control"
    license: "BSL-1.1"
    dependencies:
      - name: tmux
    install: |
      bin.install "agenthive"
    test: |
      system "#{bin}/agenthive", "--version"

nfpms:
  - id: agenthive
    package_name: agenthive
    formats:
      - deb
      - rpm
    vendor: "Shaik Noorullah"
    maintainer: "Shaik Noorullah"
    description: "Encrypted P2P mesh for AI agent notification and control"
    license: "BSL-1.1"
    homepage: "https://github.com/shaiknoorullah/agenthive"
    dependencies:
      - tmux
    contents:
      - src: ./LICENSE
        dst: /usr/share/doc/agenthive/LICENSE
        type: doc

release:
  github:
    owner: shaiknoorullah
    name: agenthive
  prerelease: auto
  draft: false
```

- [ ] **Step 2: Validate goreleaser config**

```bash
goreleaser check
```

Expected: output containing `config is valid` or similar success.

- [ ] **Step 3: Commit goreleaser config**

```bash
git add .goreleaser.yaml
git commit -m "ci: add GoReleaser config for cross-platform builds, Homebrew, and .deb/.rpm"
```

**Note:** AUR publishing (`aurs` section) and Termux .deb builds are P1 items. They require an AUR account + SSH key and Android NDK respectively. Add them to `.goreleaser.yaml` in a follow-up PR when those accounts are set up.

---

### Task 14: Update Dependabot Configuration

**Files:**
- Modify: `.github/dependabot.yml`

- [ ] **Step 1: Replace dependabot.yml**

Replace the entire contents of `.github/dependabot.yml`:

```yaml
version: 2
updates:
  - package-ecosystem: gomod
    directory: "/"
    schedule:
      interval: weekly
      day: monday
    target-branch: develop
    reviewers:
      - "shaiknoorullah"
    labels:
      - "dependencies"
      - "go"
    commit-message:
      prefix: "deps"
    open-pull-requests-limit: 10

  - package-ecosystem: github-actions
    directory: "/"
    schedule:
      interval: weekly
      day: monday
    target-branch: develop
    reviewers:
      - "shaiknoorullah"
    labels:
      - "dependencies"
      - "ci"
    commit-message:
      prefix: "ci"

  - package-ecosystem: docker
    directory: "/"
    schedule:
      interval: weekly
      day: monday
    target-branch: develop
    reviewers:
      - "shaiknoorullah"
    labels:
      - "dependencies"
      - "docker"
    commit-message:
      prefix: "deps"
```

- [ ] **Step 2: Commit dependabot update**

```bash
git add .github/dependabot.yml
git commit -m "ci: update dependabot to target develop, add docker ecosystem and labels"
```

---

### Task 15: TPM Install Script

**Files:**
- Create: `scripts/install.sh`

- [ ] **Step 1: Create scripts directory and install.sh**

```bash
#!/usr/bin/env bash
# Auto-download agenthive binary from GitHub Releases.
# Used by TPM plugin and manual installs.

set -euo pipefail

REPO="shaiknoorullah/agenthive"
INSTALL_DIR="${AGENTHIVE_INSTALL_DIR:-${TMUX_PLUGIN_DIR:-$HOME/.local/bin}}"

detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux*)  os="linux" ;;
        Darwin*) os="darwin" ;;
        *)       echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)             echo "Unsupported arch: $(uname -m)" >&2; exit 1 ;;
    esac

    echo "${os}_${arch}"
}

get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' \
        | head -1 \
        | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        echo "Failed to fetch latest version" >&2
        exit 1
    fi

    echo "$version"
}

main() {
    local platform version archive_name url tmp_dir

    platform=$(detect_platform)
    version="${1:-$(get_latest_version)}"
    archive_name="agenthive_${version#v}_${platform}.tar.gz"
    url="https://github.com/${REPO}/releases/download/${version}/${archive_name}"

    echo "Installing agenthive ${version} for ${platform}..."

    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    curl -fsSL "$url" -o "${tmp_dir}/${archive_name}"
    tar -xzf "${tmp_dir}/${archive_name}" -C "$tmp_dir"

    mkdir -p "$INSTALL_DIR"
    mv "${tmp_dir}/agenthive" "${INSTALL_DIR}/agenthive"
    chmod +x "${INSTALL_DIR}/agenthive"

    echo "Installed agenthive ${version} to ${INSTALL_DIR}/agenthive"
}

main "$@"
```

- [ ] **Step 2: Make script executable**

```bash
chmod +x scripts/install.sh
```

- [ ] **Step 3: Verify script syntax**

```bash
bash -n scripts/install.sh && echo "Syntax OK"
```

Expected: `Syntax OK`

- [ ] **Step 4: Commit install script**

```bash
git add scripts/install.sh
git commit -m "feat: add TPM binary auto-download install script"
```

---

### Task 16: GPG Signing Setup Guide

**Files:**
- Modify: `CONTRIBUTING.md`

- [ ] **Step 1: Add GPG signing section to CONTRIBUTING.md**

Add after the "Code Style" section and before "Reporting Issues" in `CONTRIBUTING.md`:

```markdown
## Signed Commits

All commits must be GPG-signed. GitHub shows a "Verified" badge on signed commits.

### Setup

1. Generate a GPG key (RSA 4096, email must match your GitHub account):
   ```bash
   gpg --full-generate-key
   ```

2. Get your key ID:
   ```bash
   gpg --list-secret-keys --keyid-format long
   ```
   The key ID is after `rsa4096/` (e.g., `rsa4096/ABC123DEF456` -> key ID is `ABC123DEF456`).

3. Add the public key to your GitHub account:
   ```bash
   gpg --armor --export YOUR_KEY_ID
   ```
   Copy the output and paste it at https://github.com/settings/gpg/new

4. Configure git to sign commits:
   ```bash
   git config --global user.signingkey YOUR_KEY_ID
   git config --global commit.gpgsign true
   git config --global tag.gpgsign true
   ```
```

- [ ] **Step 2: Commit signing docs**

```bash
git add CONTRIBUTING.md
git commit -m "docs: add GPG signing setup guide to CONTRIBUTING"
```

---

### Task 17: Push Branch and Create PR

**Files:**
- None (git operations only)

- [ ] **Step 1: Verify all tests pass**

```bash
cd /home/devsupreme/agenthive
go test -race -count=1 ./...
```

Expected: `ok` for all packages.

- [ ] **Step 2: Verify all YAML configs are valid**

```bash
python3 -c "
import yaml, glob
for f in glob.glob('.github/workflows/*.yml'):
    yaml.safe_load(open(f))
    print(f'OK: {f}')
for f in ['codecov.yml', '.golangci.yml']:
    yaml.safe_load(open(f))
    print(f'OK: {f}')
print('All YAML valid')
"
```

Expected: `All YAML valid`

- [ ] **Step 3: Review commit log**

```bash
git log --oneline ci/pipeline-and-release-tooling --not develop
```

Expected: ~15 commits covering license, lefthook, cog, cliff, golangci-lint, codecov, ci.yml, release-gate, security, release-pr, release, goreleaser, dependabot, install script, and GPG docs.

- [ ] **Step 4: Push branch**

```bash
GIT_SSH_COMMAND='ssh -i ~/.ssh/noorullah_github -o IdentitiesOnly=yes' git push -u origin ci/pipeline-and-release-tooling
```

- [ ] **Step 5: Create PR targeting develop**

```bash
gh pr create --base develop --title "ci: add complete CI/CD pipeline, release automation, and developer tooling" --body "$(cat <<'EOF'
## Summary

- Change license from MIT to BSL 1.1 (self-hosted use OK, competing hosted service restricted, converts to Apache 2.0 after 4 years)
- Add lefthook git hooks (commit-msg linting via cocogitto, pre-commit lint, pre-push tests)
- Add cocogitto config for conventional commit types and scopes
- Add git-cliff config for changelog generation and semver computation
- Add golangci-lint and codecov configuration
- Replace ci.yml: retarget to develop, add govulncheck + dependency-review + coverage
- Add release-gate.yml: extended fuzz, gosec, CodeQL, cross-compile verification
- Add release.yml: git-cliff changelog + GPG tag + GoReleaser + cosign signing
- Add release-pr.yml: auto-create/update Release PR when develop changes
- Add security.yml: weekly govulncheck + gosec + OSSF Scorecard
- Add GoReleaser config: linux/darwin x amd64/arm64, Homebrew tap, .deb/.rpm
- Update dependabot to target develop with labels
- Add TPM binary auto-download install script
- Add GPG signing guide to CONTRIBUTING

## Spec

`docs/superpowers/specs/2026-03-27-cicd-release-pipeline-design.md`

## Post-merge setup required

1. Create `shaiknoorullah/homebrew-agenthive` repo on GitHub
2. Create GitHub PAT with `repo` scope, store as `HOMEBREW_TAP_TOKEN` secret
3. Create release PAT with `repo` scope, store as `RELEASE_TOKEN` secret
4. Generate CI GPG key, store as `GPG_PRIVATE_KEY` and `GPG_PASSPHRASE` secrets
5. Set up branch protection rules for `develop` and `main`
6. Enable Codecov integration

## Test plan

- [ ] All existing tests pass (`go test -race ./...`)
- [ ] All YAML configs validate (no syntax errors)
- [ ] `lefthook install` succeeds
- [ ] `cog verify` rejects bad commit messages
- [ ] `git cliff --unreleased` generates changelog
- [ ] `goreleaser check` validates config
- [ ] `scripts/install.sh` passes bash syntax check
EOF
)"
```

---

### Task 18: Post-Merge Setup (Manual Steps)

These are manual steps the maintainer must complete after the PR is merged to `develop`. They cannot be automated in the plan.

- [ ] **Step 1: Create Homebrew tap repository**

Go to https://github.com/new and create `homebrew-agenthive` (public, empty, no README).

- [ ] **Step 2: Create GitHub secrets**

In https://github.com/shaiknoorullah/agenthive/settings/secrets/actions, add:

| Secret | Value | Purpose |
|--------|-------|---------|
| `HOMEBREW_TAP_TOKEN` | PAT with `repo` scope | GoReleaser pushes Homebrew formula |
| `RELEASE_TOKEN` | PAT with `repo` scope | Release workflow pushes changelog commit to main |
| `GPG_PRIVATE_KEY` | CI GPG private key (armored) | Sign release commits and tags |
| `GPG_PASSPHRASE` | GPG key passphrase | Unlock GPG key in CI |

Generate the CI GPG key:
```bash
gpg --batch --gen-key <<EOF2
%no-protection
Key-Type: RSA
Key-Length: 4096
Name-Real: github-actions[bot]
Name-Email: github-actions[bot]@users.noreply.github.com
Expire-Date: 0
EOF2
```

Export for secrets:
```bash
gpg --armor --export-secret-keys "github-actions[bot]@users.noreply.github.com"
```

Add the public key to https://github.com/settings/gpg/new so GitHub shows "Verified" badges on CI commits.

- [ ] **Step 3: Set up branch protection**

For `develop`:
- Require PR reviews: 0 (solo dev)
- Required status checks: `Test (Go 1.22, ubuntu-latest)`, `Test (Go 1.22, macos-latest)`, `Lint`, `Vulnerability Check`
- Require up-to-date before merge: Yes
- Block force pushes and branch deletion

For `main`:
- Require PR reviews: 1
- Required status checks: all of develop + `Fuzz (extended)`, `Security Lint`, `CodeQL`, all `Cross-Compile` jobs
- Require up-to-date before merge: Yes
- Require signed commits: Yes (vigilant mode)
- Block force pushes and branch deletion

- [ ] **Step 4: Enable Codecov**

Go to https://app.codecov.io and connect the `shaiknoorullah/agenthive` repository.
