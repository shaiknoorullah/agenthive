---
name: Bug Report
about: Report a bug to help us improve
title: "[bug] "
labels: bug
assignees: ''
---

## Describe the bug

A clear description of what the bug is.

## To Reproduce

Steps to reproduce the behavior:
1. Run `agenthive ...`
2. Do '...'
3. See error

## Expected behavior

What you expected to happen.

## Environment

- **OS**: [e.g., Ubuntu 24.04, macOS 15, Termux on Android 14]
- **agenthive version**: [e.g., commit SHA or `agenthive --version`]
- **Go version** (if building from source): [e.g., 1.22]
- **tmux version** (only if exercising the tmux surface): [e.g., 3.4]

## Logs

Relevant agenthive output (default log surface is JSON lines under `~/.config/agenthive/`):

```
paste logs here
```

For libp2p connection issues, also include `GOLOG_LOG_LEVEL=debug` output from a short reproduction.

## Additional context

Any other context about the problem.
