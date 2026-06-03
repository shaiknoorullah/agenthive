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
- Wire encryption — libp2p Noise XX over TCP and QUIC
- Peer authentication and identity (Ed25519 keypair in `~/.config/agenthive/identity.key`; PeerID derived from pubkey)
- Circuit Relay v2 abuse (reservation exhaustion, traffic redirection)
- DCUtR hole-punch coordination attacks
- GossipSub message validation and CRDT peer allow-list bypass
- Action gate (PreToolUse hook) authorization bypass
- CRDT state injection or tampering
- Notification content injection
- File permission issues in `~/.config/agenthive/` (expected: directory 0700, identity.key 0600)
- Unix socket (`agenthive.sock`) IPC tampering

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |
| < latest | No      |
