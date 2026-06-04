# Design Decisions

Every load-bearing architectural decision in agenthive has a written record. The repo uses **adversarial-debate RFCs** for hard choices: each option gets an advocate paper, a judge synthesizes, the verdict is filed alongside.

This page is an index. Click through to the source documents for the actual reasoning.

## Current decisions

| Decision | Document | Status |
|---|---|---|
| Transport / identity / discovery / NAT | [adopt-libp2p.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/adopt-libp2p.md) | accepted |
| Bidirectional action approval | [action-buttons-research.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/action-buttons-research.md) | accepted |
| Local notification storage (tmux phase) | [debate-judgment.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-judgment.md) | accepted (native tmux options) |
| Feature roadmap | [feature-research.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/feature-research.md) | living document |

## Adversarial debate — transport (closed, libp2p wins)

The transport choice ran through two debate rounds with five advocate papers and two judgments.

| Document | Position |
|---|---|
| [debate-libp2p-advocate.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-libp2p-advocate.md) | go-libp2p — the winner |
| [debate-quic-mtls-advocate.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-quic-mtls-advocate.md) | QUIC + mTLS via quic-go |
| [debate-yggdrasil-advocate.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-yggdrasil-advocate.md) | IPv6 mesh overlay |
| [debate-ssh-tunnel-advocate.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-ssh-tunnel-advocate.md) | SSH reverse tunnels |
| [debate-p2p-webrtc-advocate.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-p2p-webrtc-advocate.md) | P2P WebRTC data channels |
| [debate-file-based-advocate.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-file-based-advocate.md) | File-based IPC for local notifications |
| [debate-native-tmux-advocate.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-native-tmux-advocate.md) | Native tmux options for local notifications |
| [debate-transport-judgment.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-transport-judgment.md) | Original verdict (SSH + custom mesh + CRDT) — **superseded by adopt-libp2p.md** |

The original transport judgment picked SSH + custom mesh + CRDT. After libp2p was put on the table (it wasn't considered in the first round), it won the second round and superseded the SSH verdict. The two adversary papers (QUIC+mTLS, Yggdrasil) lost cleanly to libp2p but are worth reading to understand the trade space.

## Lessons learned

| Document | Covers |
|---|---|
| [code-analysis.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/code-analysis.md) | 31 bugs in the shell-based predecessor; informs design rules ("atomic writes, structured logging, tested components, explicit state machines") |

## How the debate process works

1. Identify a load-bearing decision with multiple plausible answers.
2. Spawn one advocate paper per option. Each advocate argues their case adversarially — strongest argument, honest weakness, concrete tech, point-by-point rebuttals.
3. A judgment paper scores against pre-declared axes (NAT traversal, mobile, infra footprint, security, etc.) and picks a winner.
4. The verdict is filed with the advocate papers. Future contributors can read both the winning reasoning and the losing alternatives — including what would have changed the verdict.
5. If conditions change (a new technology, a new constraint), the debate can be reopened. Supersession is the rule, not deletion.

## Why this matters

agenthive's design space is heavily contested — peer-to-peer notification systems can be built dozens of ways. Without the debate records, future contributors (and future-you) would relitigate the same arguments. With them, you can quickly find out why each option was rejected and whether your new idea actually changes the calculus.

## Reading order for a new contributor

1. [adopt-libp2p.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/adopt-libp2p.md) — what we actually do
2. [action-buttons-research.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/action-buttons-research.md) — the central feature
3. [debate-libp2p-advocate.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/debate-libp2p-advocate.md) — why libp2p
4. [code-analysis.md](https://github.com/shaiknoorullah/agenthive/blob/main/docs/rfcs/code-analysis.md) — what not to repeat

The rest is depth-on-demand.
