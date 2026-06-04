# Installation

agenthive ships as a single static Go binary with no runtime dependencies beyond the host OS networking stack. Pick whichever install method fits your workflow.

## Pre-built binary (recommended)

Each release ships static binaries for `linux/{amd64,arm64}` and `darwin/{amd64,arm64}`. The tarballs also include `LICENSE`, `README.md`, `SECURITY.md`, and the `tmux/` plugin assets.

```bash
# Pick the right tarball for your platform
OS=linux ARCH=amd64 VERSION=0.1.0
curl -L "https://github.com/shaiknoorullah/agenthive/releases/download/v${VERSION}/agenthive_${VERSION}_${OS}_${ARCH}.tar.gz" \
  | tar -xzC /tmp

# Move binary onto PATH
sudo install -m755 /tmp/agenthive /usr/local/bin/

# Sanity check
agenthive --version
# agenthive 0.1.0 (commit <sha>, built <iso8601>)
```

### Verify checksums

Every release publishes `checksums.txt`. Verify before installing:

```bash
curl -L "https://github.com/shaiknoorullah/agenthive/releases/download/v${VERSION}/checksums.txt" -o checksums.txt
sha256sum -c --ignore-missing checksums.txt
```

## `go install`

If you have Go 1.22+ on PATH:

```bash
# Pinned to the latest release tag (recommended for stability)
go install github.com/shaiknoorullah/agenthive/cmd/agenthive@v0.1.0

# Or follow main
go install github.com/shaiknoorullah/agenthive/cmd/agenthive@latest
```

The binary lands in `$GOPATH/bin` (default `~/go/bin`). Make sure that's on your `PATH`.

`go install` builds locally so the `--version` output reports `dev (commit none, built unknown)`. If that bothers you, use the pre-built binary or run a tagged build with ldflags yourself.

## From source

```bash
git clone https://github.com/shaiknoorullah/agenthive.git
cd agenthive
go build -o agenthive ./cmd/agenthive
./agenthive --version
```

For a stamped build matching the official tarballs:

```bash
VERSION=$(git describe --tags --always)
COMMIT=$(git rev-parse HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build \
  -trimpath \
  -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
  -o agenthive ./cmd/agenthive
```

## Termux on Android

```bash
pkg install golang openssh
go install github.com/shaiknoorullah/agenthive/cmd/agenthive@latest
~/go/bin/agenthive --version
```

Termux's Go takes a bit to compile libp2p the first time. Subsequent runs use the build cache.

## tmux plugin

Install the optional tmux plugin via [TPM](https://github.com/tmux-plugins/tpm) — see [[tmux Plugin]] for the dedicated guide.

```tmux
# In ~/.tmux.conf
set -g @plugin 'shaiknoorullah/agenthive'
```

Then `prefix + I` to install.

## What's next

→ [[Quick Start]] — get two peers talking
