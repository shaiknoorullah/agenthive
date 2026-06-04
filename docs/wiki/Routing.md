# Routing

A routing rule says: when a notification matches **this selector**, dispatch to **these targets**. Rules live in the CRDT, sync across the mesh in one round trip, and can be edited from any device.

## Anatomy of a rule

```
ID              SELECTOR                       TARGETS
critical-phone  priority:critical              phone,laptop
codex-only      source:codex-cli               desktop
default         *                              log
```

- **ID** — string, your choice. Used to delete the rule with `routes del`.
- **Selector** — comma-separated `key:value` clauses (AND-joined), or `*` for wildcard.
- **Targets** — comma-separated peer names, or the literal string `ALL` (every peer except self).

## Selector grammar

| Clause | Meaning |
|---|---|
| `agent:<name>` | Match notifications whose `agent` field equals `<name>` |
| `project:<name>` | Match `project` exactly |
| `session:<id>` | Match `session_id` exactly |
| `window:<idx>` | Match `window` index |
| `pane:<id>` | Match `pane` id |
| `source:<name>` | Match `source` (e.g., `claude-code`, `codex-cli`) |
| `priority:<level>` | Match `priority` (`info` / `warning` / `critical`) |
| `*` | Match anything |
| `default` | Alias for `*` (semantic: "the fallback") |

Multiple clauses are conjunction:

```
agenthive routes add critical-claude 'priority:critical,source:claude-code' 'phone'
```

This matches only critical-level notifications from Claude Code.

## Target syntax

A target is a peer name. Peer names default to the local hostname; you can override per-peer in the CRDT `peers` map. Targets are matched against the `PeerInfo.Name` field, not the PeerID.

Special target:

| Target | Meaning |
|---|---|
| `ALL` | Every peer in `peers` except self |

Multiple targets are union — the notification goes to every named peer, deduplicated.

If a target name doesn't match any peer in the CRDT, it's silently ignored. (No error, no warning. The route rule is "send to a peer named X if it exists.")

## Adding a rule

```bash
agenthive routes add my-rule '<selector>' '<targets>'
```

The change propagates via GossipSub to all peers in under a round trip.

## Listing rules

```bash
agenthive routes list
```

Or in the TUI: `agenthive tui` → press `r`.

## Deleting a rule

```bash
agenthive routes del my-rule
```

Tombstone propagates the same way.

## Rule conflicts and precedence

Multiple rules can match the same notification. The matcher takes the **union of targets** from all matching rules.

If you want exclusivity (only the most specific rule fires), file an issue — that's not in v0.1.0.

The `default` / `*` rule fires for **every** notification on top of any other matching rule. If you don't want a fallback, don't add one.

## Examples

### Phone gets everything critical

```bash
agenthive routes add phone-crit 'priority:critical' 'phone'
```

### Laptop sees Claude, desktop sees Codex

```bash
agenthive routes add claude-laptop 'source:claude-code' 'laptop'
agenthive routes add codex-desktop 'source:codex-cli' 'desktop'
```

### Per-project dashboards

```bash
agenthive routes add api-dash    'project:api-server'    'laptop,desktop'
agenthive routes add docs-dash   'project:docs-site'     'laptop'
```

### Quiet hours via deletion

There's no DND mode in v0.1.0. The closest approximation:

```bash
# Before sleep
agenthive routes del phone-crit
# After waking
agenthive routes add phone-crit 'priority:critical' 'phone'
```

A real DND mode is on the v0.2.0 roadmap.

### Per-session focus

When a long-running refactor is going on in `session:refactor-2026`, you might want **only** that session to reach you, with everything else silent:

```bash
agenthive routes add focus-only 'session:refactor-2026' 'phone,laptop'
# Don't add a default rule. Other sessions don't match anything → no dispatch.
```

## Selectors against what?

Every notification carries the following fields (those that apply):

```json
{
  "agent":      "claude",
  "project":    "api-server",
  "session_id": "abc",
  "window":     1,
  "pane":       "%2",
  "source":     "claude-code",
  "priority":   "warning"
}
```

The matcher walks each rule's selector and checks every `key:value` clause against the notification field of the same name. **Set selector fields must match exactly. Unset selector fields are wildcards.** Field names are case-sensitive; values are case-sensitive.

## See also

- [[CLI Reference]] — `routes` subcommands in detail
- [[Architecture]] — where the matcher sits in the dispatch path
- [[CRDT State Sync]] — how rule changes propagate across the mesh
