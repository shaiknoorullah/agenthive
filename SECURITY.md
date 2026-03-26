# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in agenthive, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, email: **security@shaiknoorullah.dev** (or open a private security advisory on GitHub).

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Response Timeline

- **Acknowledgment**: within 48 hours
- **Assessment**: within 1 week
- **Fix release**: as soon as practical, typically within 2 weeks

## Scope

Security issues in the following areas are in scope:
- Transport encryption (SSH tunnels, Noise Protocol)
- Peer authentication and identity
- Action gate (PreToolUse hook) authorization bypass
- CRDT state injection or tampering
- Notification content injection
- File permission issues in `~/.config/agenthive/`

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |
| < latest | No      |
