# Research: Bidirectional Action Buttons for Agent Notifications

**Date:** 2026-03-26
**Status:** Research Complete
**Scope:** Multi-surface action routing from AI coding agents to user devices and back

---

## Table of Contents

1. [Claude Code Hook Payload Analysis](#1-claude-code-hook-payload-analysis)
2. [Response Injection Mechanisms](#2-response-injection-mechanisms)
3. [Multi-Surface Action Button Implementation](#3-multi-surface-action-button-implementation)
4. [Security Model](#4-security-model)
5. [Architecture Proposal](#5-architecture-proposal)

---

## 1. Claude Code Hook Payload Analysis

Claude Code exposes a rich hook system with 24 documented lifecycle events. The events most relevant to bidirectional action buttons are **PreToolUse**, **PermissionRequest**, **Notification**, and **Stop**.

### 1.1 Hook Events Relevant to Action Buttons

#### PreToolUse (the permission gate)

**When it fires:** After Claude creates tool parameters, before tool execution.

**Matchers:** Tool names -- `Bash`, `Edit`, `Write`, `Read`, `Glob`, `Grep`, `Agent`, `WebFetch`, `WebSearch`, and MCP tools (`mcp__server__tool`).

**JSON input (stdin):**
```json
{
  "session_id": "abc123",
  "transcript_path": "/path/to/transcript.jsonl",
  "cwd": "/current/working/directory",
  "permission_mode": "default",
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": {
    "command": "rm -rf /tmp/build",
    "description": "Clean build directory",
    "timeout": 30000
  },
  "tool_use_id": "unique-id-123"
}
```

**Tool-specific input schemas:**
| Tool | Key fields in `tool_input` |
|------|---------------------------|
| `Bash` | `command`, `description`, `timeout`, `run_in_background` |
| `Write` | `file_path`, `content` |
| `Edit` | `file_path`, `old_string`, `new_string`, `replace_all` |
| `Read` | `file_path`, `offset`, `limit` |
| `Glob` | `pattern`, `path` |
| `Grep` | `pattern`, `path`, `glob`, `output_mode` |
| `Agent` | `prompt`, `description`, `subagent_type`, `model` |

**Exit code behavior:**
- Exit 0: Allow tool call
- Exit 2: Block tool call (stderr message fed back to Claude)

**Structured JSON output for decisions:**
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "permissionDecisionReason": "User approved via phone notification",
    "updatedInput": {},
    "additionalContext": "User approved at 14:32:05 from mobile"
  }
}
```

The `permissionDecision` field accepts three values:
- `"allow"` -- bypasses permission prompt, tool executes immediately
- `"deny"` -- blocks this specific tool call, Claude receives the reason
- `"ask"` -- falls through to normal permission prompt

**Critical insight:** PreToolUse can return `"allow"` or `"deny"` programmatically. This means a hook script can wait for an external signal (e.g., a user clicking Allow on their phone), then return the decision. The hook does not need to inject keystrokes -- it controls the decision directly via its exit code and stdout JSON.

#### PermissionRequest (the permission dialog itself)

**When it fires:** When the permission dialog is about to be shown to the user.

**JSON input includes:**
```json
{
  "tool_name": "Bash",
  "tool_input": { "command": "npm install" },
  "permission_suggestions": ["allow_once", "allow_always", "deny"]
}
```

**Decision output:**
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PermissionRequest",
    "decision": {
      "behavior": "allow",
      "updatedInput": {},
      "updatedPermissions": [
        {
          "type": "addRules",
          "rules": ["Bash(npm *)"],
          "behavior": "allow",
          "destination": "session"
        }
      ],
      "message": "Approved via remote action button"
    }
  }
}
```

**Critical insight:** PermissionRequest can not only allow/deny but also update permission rules (addRules, replaceRules, removeRules, setMode) and persist them to session, local, project, or user settings. This means "Allow Always" can be implemented from a remote button click.

#### Notification

**When it fires:** When Claude Code sends user-facing notifications.

**Matchers:** `permission_prompt`, `idle_prompt`, `auth_success`, `elicitation_dialog`

**JSON input:**
```json
{
  "session_id": "abc123",
  "cwd": "/path/to/project",
  "hook_event_name": "Notification",
  "message": "Claude is waiting for permission to run: npm test",
  "title": "Permission Required",
  "notification_type": "permission_prompt"
}
```

**Key detail:** The `notification_type` field tells us exactly why Claude is notifying the user. The `permission_prompt` type means Claude is waiting for permission -- this is the trigger for showing action buttons. The `idle_prompt` type means Claude has been waiting and the user has not responded.

**Limitation:** Notification hooks cannot block. They are observe-only. However, they are the ideal trigger for dispatching action button messages to external surfaces.

#### Stop

**When it fires:** When Claude finishes responding.

**JSON input:**
```json
{
  "session_id": "abc123",
  "cwd": "/path/to/project",
  "hook_event_name": "Stop",
  "stop_hook_active": false,
  "last_assistant_message": "I've completed the refactoring. All tests pass."
}
```

**Key detail:** The `last_assistant_message` field contains Claude's final response text. This is the content to show in "Agent finished" notifications. The hook can also block stopping (exit 2 with `"decision": "block"` and a `reason`), which would force Claude to continue -- useful for automated continuation workflows.

### 1.2 Hook Payload Summary Table

| Hook Event | Can block? | Has message content? | Has tool details? | Action button use case |
|------------|-----------|---------------------|-------------------|----------------------|
| PreToolUse | Yes (allow/deny/ask) | No | Yes (tool_name, tool_input) | Primary: approve/deny tool execution |
| PermissionRequest | Yes (allow/deny + rule updates) | No | Yes (tool_name, tool_input, suggestions) | Alternative: approve/deny with "always allow" |
| Notification | No | Yes (message, title, type) | No | Trigger: dispatch action buttons to surfaces |
| Stop | Yes (can force continuation) | Yes (last_assistant_message) | No | Dismiss/acknowledge/continue |

### 1.3 Two-Hook Strategy

The architecture should use two hooks in concert:

1. **Notification hook** (`notification_type: "permission_prompt"`): Detects that Claude is asking for permission. Dispatches the action button request to all surfaces with the tool details. Does not block.

2. **PreToolUse hook** (or **PermissionRequest hook**): When the user clicks a button on any surface, the response is stored (e.g., in a file or via IPC). The next PreToolUse invocation for the same tool reads this stored decision and returns it. Alternatively, the PreToolUse hook can poll/wait for a decision with a timeout.

**Problem with this approach:** PreToolUse fires *before* the permission dialog, so it runs before the Notification hook would fire for `permission_prompt`. The timing is:

```
Claude decides to use tool
  -> PreToolUse hook fires (can allow/deny/ask)
  -> If PreToolUse returns "ask" or does not decide:
     -> Permission dialog shown
     -> Notification hook fires (permission_prompt)
     -> User sees dialog in terminal
```

**Better strategy:** Use PreToolUse as the single decision point. The hook:
1. Writes the pending action request to the action queue (what tool, what input)
2. Dispatches notifications to all surfaces
3. Waits (with timeout) for a response from any surface
4. Returns the decision (allow/deny) or falls through to "ask" on timeout

This keeps everything synchronous within a single hook invocation, avoiding the two-hook timing problem.

---

## 2. Response Injection Mechanisms

### 2.1 tmux send-keys (Keystroke Injection)

**How it works:**
```bash
tmux send-keys -t %42 "y" Enter
```
Sends the literal character "y" followed by the Enter key to pane %42.

**Reliability assessment:**

| Aspect | Status | Details |
|--------|--------|---------|
| Basic keystroke delivery | Reliable | Characters arrive in the correct order |
| Enter key interpretation | Unreliable | Claude Code uses Ink (React terminal UI). Programmatic Enter is often treated as newline, not submit. The autocomplete dropdown intercepts Enter. |
| Workaround | Exists | Send text, sleep 0.3s, send Escape (dismiss autocomplete), sleep 0.1s, send Enter. Adds ~0.4s latency. |
| Bracketed paste | Exists | Using `tmux set-buffer` + `paste-buffer -p` wraps text in ESC[200~/ESC[201~ sequences, avoiding Ink interpretation issues. |
| Hex byte injection | More reliable | `tmux send-keys -t %42 $'\x0d'` sends raw carriage return, bypassing keyword interpretation. |

**Community-validated workaround (from GitHub issue #15553):**
```bash
# Used by operators running 10+ agent systems
tmux send-keys -t "$pane" "inbox3"
sleep 0.3
tmux send-keys -t "$pane" Escape
sleep 0.1
tmux send-keys -t "$pane" Enter
```

**Edge cases:**
- If the pane is in copy mode, send-keys goes to copy mode, not the running process
- If Claude Code is not at a permission prompt (e.g., it is thinking), the keystroke is queued and may be interpreted incorrectly when the prompt appears
- Race condition: if Claude finishes and presents a new prompt between the text send and the Enter send
- Multi-byte characters and special characters may require `-l` flag (literal mode)

**Verdict:** Keystroke injection via tmux send-keys is a *fallback* mechanism, not the primary one. PreToolUse hook decisions are strictly superior for permission approvals because they operate at the API level, not the UI level.

### 2.2 PreToolUse Hook Decisions (Programmatic Approval)

**This is the recommended primary mechanism.**

The PreToolUse hook can return `"allow"` or `"deny"` without any keystroke injection:

```bash
#!/usr/bin/env bash
# PreToolUse hook that waits for external decision

JSON_DATA=$(cat)
TOOL_NAME=$(echo "$JSON_DATA" | jq -r '.tool_name')
TOOL_INPUT=$(echo "$JSON_DATA" | jq -r '.tool_input')
SESSION_ID=$(echo "$JSON_DATA" | jq -r '.session_id')
TOOL_USE_ID=$(echo "$JSON_DATA" | jq -r '.tool_use_id')

# Write pending action to queue
ACTION_DIR="$HOME/.tmux-notifications/actions"
mkdir -p "$ACTION_DIR"
ACTION_FILE="$ACTION_DIR/${SESSION_ID}_${TOOL_USE_ID}"

cat > "$ACTION_FILE.pending" <<EOF
{
  "session_id": "$SESSION_ID",
  "tool_use_id": "$TOOL_USE_ID",
  "tool_name": "$TOOL_NAME",
  "tool_input": $TOOL_INPUT,
  "timestamp": $(date +%s),
  "pane_id": "$TMUX_PANE"
}
EOF

# Dispatch to notification surfaces (non-blocking)
dispatch_to_surfaces "$ACTION_FILE.pending" &

# Wait for response (max 300 seconds)
TIMEOUT=300
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    if [ -f "$ACTION_FILE.response" ]; then
        DECISION=$(jq -r '.decision' "$ACTION_FILE.response")
        REASON=$(jq -r '.reason // "Remote approval"' "$ACTION_FILE.response")
        rm -f "$ACTION_FILE.pending" "$ACTION_FILE.response"

        # Return decision to Claude Code
        cat <<ENDJSON
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "$DECISION",
    "permissionDecisionReason": "$REASON"
  }
}
ENDJSON
        exit 0
    fi
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done

# Timeout: fall through to normal permission prompt
rm -f "$ACTION_FILE.pending"
exit 0
```

**Advantages over keystroke injection:**
- No timing issues, no autocomplete interference
- Works regardless of terminal UI state
- Can pass structured reasons and context back to Claude
- Can update permission rules ("Allow Always") in a single operation
- Works identically whether Claude Code is in tmux, VS Code, or a plain terminal

**Limitation:** The hook blocks Claude Code while waiting. During the wait, the permission prompt is not shown in the terminal (because PreToolUse fires before the prompt). The user at the terminal sees Claude "thinking" until the hook returns. To mitigate, the hook should have a reasonable timeout (30-300 seconds) after which it returns "ask" to fall through to the normal prompt.

### 2.3 PermissionRequest Hook Decisions

Similar to PreToolUse but fires later (when the dialog is about to show). Can additionally modify permission rules:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PermissionRequest",
    "decision": {
      "behavior": "allow",
      "updatedPermissions": [
        {
          "type": "addRules",
          "rules": ["Bash(npm *)"],
          "behavior": "allow",
          "destination": "session"
        }
      ]
    }
  }
}
```

### 2.4 Claude Code SDK (Programmatic Control)

Claude Code can be run programmatically via the Agent SDK:

```bash
# One-shot execution with auto-approved tools
claude -p "Run the test suite" --allowedTools "Bash,Read,Edit" --output-format json

# Continue a specific session
claude -p "Fix the failing test" --resume "$SESSION_ID" --allowedTools "Bash,Edit"
```

**Relevance to action buttons:** The SDK is useful for *starting* agent tasks but not for *interacting with* a running interactive session. For an already-running Claude Code session in tmux, the hook system is the correct interface. The SDK feature request for Unix socket IPC (GitHub issue #15553) is not yet implemented.

### 2.5 Mechanism Comparison

| Mechanism | Reliability | Latency | Structured | Works outside tmux | Recommended |
|-----------|------------|---------|------------|-------------------|-------------|
| PreToolUse hook decision | Excellent | <1s | Yes (JSON) | Yes | Primary |
| PermissionRequest hook | Excellent | <1s | Yes (JSON + rule updates) | Yes | For "Always Allow" |
| tmux send-keys (with Escape workaround) | Good (with delays) | ~0.5s | No (keystroke only) | No | Fallback only |
| tmux send-keys (raw hex) | Good | <0.1s | No | No | Fallback only |
| Claude Code SDK --resume | N/A (different use case) | >1s | Yes | Yes | Not applicable |
| Unix socket IPC | Not yet available | - | - | - | Future |

---

## 3. Multi-Surface Action Button Implementation

### 3.1 tmux Native

#### display-menu

**Syntax:**
```bash
tmux display-menu -T "Agent Permission" -x C -y C \
  "Allow"       a "run-shell '~/.tmux-notifications/scripts/action-respond.sh allow %42 tool123'" \
  "Deny"        d "run-shell '~/.tmux-notifications/scripts/action-respond.sh deny %42 tool123'" \
  "Allow Always" A "run-shell '~/.tmux-notifications/scripts/action-respond.sh allow-always %42 tool123'" \
  ""            "" "" \
  "View Details" v "display-popup -w 80% -h 60% -E 'cat ~/.tmux-notifications/actions/pending_tool123.json | jq . | less'"
```

Menu items are triplets: `<display-text> <shortcut-key> <tmux-command>`. The command can be any tmux command, including `run-shell` which can invoke scripts that write response files, send-keys to other panes, or make HTTP calls.

**Positioning:** `-x C -y C` centers the menu. Can also use `-x P -y P` for pane-relative or `-x M -y M` for mouse position.

**Limitations:**
- Maximum display width is terminal width
- No async behavior -- menu blocks the tmux client until dismissed
- Cannot show rich content (just text labels)

#### display-popup with interactive script

```bash
tmux display-popup -w 60% -h 40% -E \
  '~/.tmux-notifications/scripts/action-popup.sh %42 tool123'
```

The popup runs a script that can render rich TUI content:

```bash
#!/usr/bin/env bash
# action-popup.sh - Interactive approval popup

PANE_ID="$1"
TOOL_ID="$2"
ACTION_FILE="$HOME/.tmux-notifications/actions/pending_${TOOL_ID}.json"

# Display tool details
echo "=== Agent Permission Request ==="
echo ""
jq -r '"Tool: \(.tool_name)\nCommand: \(.tool_input.command // .tool_input.file_path // "N/A")\nProject: \(.cwd | split("/") | last)"' "$ACTION_FILE"
echo ""
echo "---"
echo "[a] Allow    [d] Deny    [A] Allow Always    [v] View Full Details    [q] Dismiss"
echo ""

read -n 1 -s choice
case "$choice" in
    a) echo "allow" > "${ACTION_FILE%.pending}.response" ;;
    d) echo "deny" > "${ACTION_FILE%.pending}.response" ;;
    A) echo "allow-always" > "${ACTION_FILE%.pending}.response" ;;
    v) jq . "$ACTION_FILE" | less; exec "$0" "$@" ;;  # Re-show after viewing
    *) exit 0 ;;
esac
```

**Advantages over display-menu:**
- Rich content display (tool details, command preview, file diffs)
- Can show scrollable content
- More flexible input handling
- Can run any script (Python, Node.js, etc.)

#### confirm-before

```bash
tmux confirm-before -p "Allow Bash: rm -rf /tmp/build? (y/n)" \
  "run-shell '~/.tmux-notifications/scripts/action-respond.sh allow %42 tool123'"
```

**Limitations:** Only yes/no, no custom buttons. Useful for simple Allow/Deny but not for "Allow Always" or "View Details".

#### Cross-pane interaction

All tmux commands can target other panes. A menu/popup in one pane can send-keys to another:

```bash
# Menu item that sends "y" to pane %42
tmux display-menu \
  "Approve" a "send-keys -t %42 y Enter"
```

### 3.2 Desktop Notifications

#### Linux: notify-send and notify-send.py

**Standard notify-send (limited):**
```bash
notify-send "Claude: Permission Request" \
  "Bash: npm test\nProject: my-app" \
  --urgency=critical \
  --icon=dialog-question
```
Standard `notify-send` cannot add action buttons. It is fire-and-forget.

**notify-send.py (action buttons with stdout response):**
```bash
#!/usr/bin/env bash
# Blocks until user clicks a button. Print action ID to stdout.
RESPONSE=$(notify-send.py \
  "Claude: Permission Request" \
  "Tool: Bash\nCommand: npm test" \
  --action allow:Allow deny:Deny always:"Allow Always" \
  --urgency critical)

case "$RESPONSE" in
    allow)  echo '{"decision":"allow"}' > "$RESPONSE_FILE" ;;
    deny)   echo '{"decision":"deny"}' > "$RESPONSE_FILE" ;;
    always) echo '{"decision":"allow","always":true}' > "$RESPONSE_FILE" ;;
    close)  ;; # User dismissed notification
esac
```

**Key behavior:** `notify-send.py` blocks until the user interacts. Run it in the background with `&` and capture the PID for cleanup. The action identifier (e.g., "allow") is printed to stdout when the user clicks the button. "close" is printed if dismissed.

**notify-send.sh alternative:**
```bash
notify-send.sh "Permission Request" "Bash: npm test" \
  --action "allow:Allow:~/.tmux-notifications/scripts/action-respond.sh allow %42 tool123" \
  --action "deny:Deny:~/.tmux-notifications/scripts/action-respond.sh deny %42 tool123"
```
Actions use `LABEL:COMMAND` format -- the command runs directly when the button is clicked.

**Notification server requirements:** Action buttons require a notification daemon that supports the `org.freedesktop.Notifications` actions capability. Most modern desktops support this: GNOME (mutter), KDE (Plasma), XFCE (xfce4-notifyd), dunst, mako (Sway). Notable exception: some minimal WM setups with no notification daemon.

#### macOS: osascript

```bash
RESPONSE=$(osascript -e '
  button returned of (display dialog "Claude wants to run: npm test" ¬
    with title "Agent Permission Request" ¬
    buttons {"Deny", "Allow", "Allow Always"} ¬
    default button "Allow" ¬
    with icon caution ¬
    giving up after 300)')

case "$RESPONSE" in
    "Allow")        echo '{"decision":"allow"}' > "$RESPONSE_FILE" ;;
    "Deny")         echo '{"decision":"deny"}' > "$RESPONSE_FILE" ;;
    "Allow Always") echo '{"decision":"allow","always":true}' > "$RESPONSE_FILE" ;;
esac
```

**Key behavior:**
- `osascript` blocks until the user clicks a button
- Maximum 3 buttons (AppleScript limitation)
- `giving up after 300` adds a 300-second auto-dismiss timeout
- Returns the button text via `button returned of`
- Can be run in the background with `&`
- The dialog appears as a system modal -- it takes focus

**macOS notification center (non-blocking alternative):**
```bash
osascript -e 'display notification "Bash: npm test" with title "Claude Permission" subtitle "Project: my-app" sound name "Purr"'
```
This shows a notification banner but has no action buttons and no response mechanism. Useful for informational alerts only.

### 3.3 Slack Interactive Messages

#### Architecture

Slack interactive messages with buttons use Block Kit and require either a public HTTP endpoint or Socket Mode (WebSocket) to receive button click callbacks.

**Socket Mode is the recommended approach** because it does not require a public URL:

```
User clicks button in Slack
  -> Slack sends interaction payload over WebSocket
  -> Local bot process receives the payload
  -> Bot writes response file or calls action router
  -> PreToolUse hook picks up the decision
```

#### Implementation with Bolt for Python + Socket Mode

```python
import os
import json
from slack_bolt import App
from slack_bolt.adapter.socket_mode import SocketModeHandler

app = App(token=os.environ["SLACK_BOT_TOKEN"])

@app.action("approve_tool")
def handle_approve(ack, body, respond):
    ack()
    action_id = body["actions"][0]["value"]  # e.g., "session123_tool456"
    response_file = f"{os.environ['HOME']}/.tmux-notifications/actions/{action_id}.response"

    with open(response_file, "w") as f:
        json.dump({
            "decision": "allow",
            "reason": f"Approved by {body['user']['username']} via Slack",
            "surface": "slack",
            "user": body["user"]["id"],
            "timestamp": body["actions"][0]["action_ts"]
        }, f)

    respond(
        replace_original=True,
        text=f"Approved by <@{body['user']['id']}>"
    )

@app.action("deny_tool")
def handle_deny(ack, body, respond):
    ack()
    action_id = body["actions"][0]["value"]
    response_file = f"{os.environ['HOME']}/.tmux-notifications/actions/{action_id}.response"

    with open(response_file, "w") as f:
        json.dump({
            "decision": "deny",
            "reason": f"Denied by {body['user']['username']} via Slack",
            "surface": "slack",
            "user": body["user"]["id"]
        }, f)

    respond(replace_original=True, text=f"Denied by <@{body['user']['id']}>")

# Start Socket Mode
handler = SocketModeHandler(app, os.environ["SLACK_APP_TOKEN"])
handler.start()
```

#### Sending the action button message

```python
from slack_sdk import WebClient

client = WebClient(token=os.environ["SLACK_BOT_TOKEN"])

def send_action_request(channel, action_data):
    tool_name = action_data["tool_name"]
    tool_input = action_data["tool_input"]
    action_id = f"{action_data['session_id']}_{action_data['tool_use_id']}"

    # Format tool details
    if tool_name == "Bash":
        detail = f"```{tool_input.get('command', 'N/A')}```"
    elif tool_name in ("Write", "Edit"):
        detail = f"`{tool_input.get('file_path', 'N/A')}`"
    else:
        detail = f"```{json.dumps(tool_input, indent=2)[:500]}```"

    client.chat_postMessage(
        channel=channel,
        text=f"Permission request: {tool_name}",
        blocks=[
            {
                "type": "header",
                "text": {"type": "plain_text", "text": f"Agent Permission: {tool_name}"}
            },
            {
                "type": "section",
                "text": {"type": "mrkdwn", "text": detail}
            },
            {
                "type": "context",
                "elements": [
                    {"type": "mrkdwn", "text": f"Project: *{action_data.get('project', 'unknown')}* | Session: `{action_data['session_id'][:8]}`"}
                ]
            },
            {
                "type": "actions",
                "elements": [
                    {
                        "type": "button",
                        "text": {"type": "plain_text", "text": "Allow"},
                        "style": "primary",
                        "action_id": "approve_tool",
                        "value": action_id
                    },
                    {
                        "type": "button",
                        "text": {"type": "plain_text", "text": "Deny"},
                        "style": "danger",
                        "action_id": "deny_tool",
                        "value": action_id
                    },
                    {
                        "type": "button",
                        "text": {"type": "plain_text", "text": "View Details"},
                        "action_id": "view_details",
                        "value": action_id
                    }
                ]
            }
        ]
    )
```

**Slack timing constraints:**
- Interactive message responses must acknowledge within 3 seconds (`ack()`)
- The original message can be updated for up to 30 minutes after posting
- Socket Mode WebSocket connections refresh periodically (handled by Bolt automatically)

### 3.4 Discord

#### Architecture

Discord bots receive button interactions via the Gateway (WebSocket). Unlike Slack, Discord does not have a "Socket Mode" toggle -- the Gateway WebSocket is the standard mechanism.

**Sending a message with buttons:**
```python
import discord

class ActionView(discord.ui.View):
    def __init__(self, action_id):
        super().__init__(timeout=300)  # 5-minute timeout
        self.action_id = action_id

    @discord.ui.button(label="Allow", style=discord.ButtonStyle.success)
    async def allow(self, interaction, button):
        response_file = f"{os.environ['HOME']}/.tmux-notifications/actions/{self.action_id}.response"
        with open(response_file, "w") as f:
            json.dump({"decision": "allow", "user": str(interaction.user), "surface": "discord"}, f)
        await interaction.response.edit_message(content=f"Approved by {interaction.user}", view=None)

    @discord.ui.button(label="Deny", style=discord.ButtonStyle.danger)
    async def deny(self, interaction, button):
        response_file = f"{os.environ['HOME']}/.tmux-notifications/actions/{self.action_id}.response"
        with open(response_file, "w") as f:
            json.dump({"decision": "deny", "user": str(interaction.user), "surface": "discord"}, f)
        await interaction.response.edit_message(content=f"Denied by {interaction.user}", view=None)
```

**Discord timing constraints:**
- Must acknowledge interactions within 3 seconds
- Buttons remain active for up to 15 minutes (configurable via `timeout`)
- Requires a bot application registered with Discord

**Key difference from Slack:** Discord requires a persistent WebSocket Gateway connection. There is no webhook-only mode for receiving button interactions. Webhooks can *send* messages with buttons but cannot *receive* the click events.

### 3.5 Phone/Mobile via ntfy

ntfy is the most practical approach for mobile action buttons because it requires no app development, supports self-hosting, and has native Android/iOS apps.

#### Sending a notification with action buttons

```bash
#!/usr/bin/env bash
# dispatch-ntfy.sh - Send action request to phone via ntfy

NTFY_TOPIC="${NTFY_TOPIC:-agent-actions}"
NTFY_SERVER="${NTFY_SERVER:-https://ntfy.sh}"
CALLBACK_URL="${ACTION_CALLBACK_URL:-https://your-server.example.com/action}"

ACTION_ID="$1"
TOOL_NAME="$2"
TOOL_DETAIL="$3"
PROJECT="$4"

curl -s "$NTFY_SERVER/$NTFY_TOPIC" \
  -H "Title: Agent: $TOOL_NAME ($PROJECT)" \
  -H "Priority: high" \
  -H "Tags: robot,warning" \
  -H "Actions: \
http, Allow, ${CALLBACK_URL}/allow/${ACTION_ID}, method=POST, headers.Authorization=Bearer ${ACTION_TOKEN}; \
http, Deny, ${CALLBACK_URL}/deny/${ACTION_ID}, method=POST, headers.Authorization=Bearer ${ACTION_TOKEN}; \
view, Details, ${CALLBACK_URL}/details/${ACTION_ID}" \
  -d "$TOOL_DETAIL"
```

**How ntfy action buttons work:**
- Up to 3 action buttons per notification
- Four action types: `view` (open URL), `http` (send HTTP request), `broadcast` (Android intent), `copy` (clipboard)
- The `http` action type sends an HTTP request to an arbitrary URL when tapped -- this is the callback mechanism
- The HTTP request includes headers, method, and body specified in the action definition
- The `clear=true` option auto-dismisses the notification after the action is taken

**ntfy + local callback server:**

The `http` action requires a URL reachable from the phone. Options:
1. **Self-hosted ntfy + local network:** Phone and server on same network (home/office WiFi)
2. **Tailscale/WireGuard/ZeroTier:** VPN mesh gives the phone a route to the dev machine
3. **SSH tunnel:** `ssh -R 8080:localhost:8080 public-server` exposes a local callback endpoint
4. **Cloudflare Tunnel / ngrok:** Exposes local port with a public URL (privacy trade-off)

### 3.6 Phone/Mobile via PWA with Web Push

#### Architecture

A lightweight PWA served from the dev machine (or a small VPS) can show push notifications with action buttons.

**Service Worker notification with actions:**
```javascript
// service-worker.js
self.addEventListener('push', event => {
  const data = event.data.json();

  event.waitUntil(
    self.registration.showNotification(data.title, {
      body: data.body,
      icon: '/icon-192.png',
      badge: '/badge-72.png',
      tag: data.actionId,  // Deduplicate by action ID
      requireInteraction: true,  // Don't auto-dismiss
      actions: [
        { action: 'allow', title: 'Allow' },
        { action: 'deny', title: 'Deny' }
      ],
      data: {
        actionId: data.actionId,
        callbackUrl: data.callbackUrl
      }
    })
  );
});

self.addEventListener('notificationclick', event => {
  event.notification.close();
  const { actionId, callbackUrl } = event.notification.data;
  const decision = event.action || 'allow';  // Default click = allow

  event.waitUntil(
    fetch(`${callbackUrl}/${decision}/${actionId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ decision, timestamp: Date.now() })
    })
  );
});
```

**Limitations:**
- iOS Safari supports web push (since iOS 16.4) but action buttons are limited or absent
- Android Chrome supports up to 2 action buttons reliably
- Requires HTTPS for service workers (can use localhost or self-signed certs for local development)
- Push subscription requires a VAPID key pair

### 3.7 Surface Capability Comparison

| Surface | Action buttons | Response latency | Always available | Setup complexity | Auth built-in |
|---------|---------------|-----------------|------------------|-----------------|---------------|
| tmux display-menu | 5+ items | <0.1s | Only when in tmux | None | tmux session |
| tmux display-popup | Unlimited (TUI) | <0.1s | Only when in tmux | None | tmux session |
| Linux notify-send.py | 3 buttons | <0.5s | When at desktop | Low (pip install) | No |
| macOS osascript | 3 buttons | <0.5s | When at desktop | None | No |
| Slack (Socket Mode) | 5+ buttons | 1-3s | Always (if bot running) | Medium (Slack app) | Slack auth |
| Discord | 5+ buttons | 1-3s | Always (if bot running) | Medium (Discord bot) | Discord auth |
| ntfy (phone) | 3 buttons | 1-5s | Always (push) | Low (ntfy app) | Token header |
| PWA push | 2 buttons | 1-5s | Always (push) | High (HTTPS + VAPID) | Subscription |

---

## 4. Security Model

### 4.1 Threat Model

| Threat | Risk | Surface affected |
|--------|------|-----------------|
| Unauthorized approval (stranger clicks Allow) | High | Slack, Discord, ntfy, PWA |
| Replay attack (re-sending an old approval) | Medium | All network surfaces |
| Man-in-the-middle (intercepted approval) | Medium | ntfy (if not self-hosted), PWA |
| Brute-force action IDs | Low | All (if IDs are guessable) |
| Local privilege escalation (another user on same machine) | Low | tmux, file-based response |

### 4.2 Authentication per Surface

**tmux native:**
- Inherently authenticated by tmux session access
- Only users with access to the tmux socket can interact
- No additional auth needed

**Desktop notifications (Linux/macOS):**
- Inherently authenticated by desktop session
- Only the logged-in user sees and can interact with notifications
- No additional auth needed

**Slack:**
- Authenticate via Slack workspace membership
- Restrict to specific channels visible only to authorized users
- Validate `user_id` in interaction payload against an allowlist
- Example:
  ```python
  AUTHORIZED_USERS = {"U12345", "U67890"}

  @app.action("approve_tool")
  def handle_approve(ack, body, respond):
      if body["user"]["id"] not in AUTHORIZED_USERS:
          ack()
          respond(text="Unauthorized", replace_original=False)
          return
      ack()
      # ... process approval
  ```

**Discord:**
- Similar to Slack: validate `interaction.user.id` against allowlist
- Restrict bot to specific servers and channels

**ntfy:**
- Use access tokens in action callback URLs: `headers.Authorization=Bearer <token>`
- Self-host ntfy server for full control
- Use topic-level ACLs to restrict who can subscribe
- The callback URL should validate the Authorization header

**PWA:**
- Use unique push subscription per user
- The notification callback includes a session-specific HMAC token
- Validate the token server-side before processing the action

### 4.3 Action ID Security

Action IDs must be unguessable. Use cryptographically random identifiers:

```bash
ACTION_ID=$(openssl rand -hex 16)
# Result: e.g., "a3f8c2d1e5b7094f6a2c8d3e1f5b7a09"
```

Do not use sequential IDs, session IDs alone, or predictable patterns.

### 4.4 Action Expiry

Every pending action should have a TTL:

```json
{
  "action_id": "a3f8c2d1e5b7094f",
  "expires_at": 1711468800,
  "tool_name": "Bash",
  "tool_input": {"command": "npm test"}
}
```

The action router should reject responses to expired actions. Suggested TTL: 5 minutes for normal actions, 30 seconds for destructive actions (`rm`, `git push --force`).

### 4.5 Confirmation for Destructive Actions

Actions classified as destructive should require a two-step confirmation:

1. User clicks "Allow" on phone
2. A second confirmation appears: "This will run `rm -rf /tmp/build`. Confirm?"
3. Only after second confirmation does the action proceed

**Classification heuristic:**
```bash
is_destructive() {
    local cmd="$1"
    case "$cmd" in
        *rm\ -rf*|*git\ push\ --force*|*drop\ table*|*DELETE\ FROM*|*shutdown*) return 0 ;;
        *) return 1 ;;
    esac
}
```

### 4.6 Audit Trail

Every action response should be logged:

```
[2026-03-26 14:32:05] ACTION session=abc123 tool=Bash cmd="npm test" decision=allow surface=slack user=kai response_time=4.2s
[2026-03-26 14:33:12] ACTION session=abc123 tool=Write file=src/auth.py decision=deny surface=phone user=kai response_time=8.7s
[2026-03-26 14:35:01] ACTION session=abc123 tool=Bash cmd="rm -rf dist" decision=allow surface=tmux_menu user=local response_time=0.3s
```

Store in `~/.tmux-notifications/action-audit.log` with rotation.

### 4.7 Rate Limiting

Prevent notification flooding:
- Maximum 1 action request per surface per 5 seconds
- Maximum 10 pending actions across all sessions
- If an action is already pending for the same tool+input, deduplicate

---

## 5. Architecture Proposal

### 5.1 System Overview

```
+--------------------------------------------------+
|  Claude Code (in tmux pane %42)                   |
|                                                   |
|  "I need to run: npm test"                        |
|       |                                           |
|       v                                           |
|  PreToolUse Hook                                  |
|  (scripts/action-gate.sh)                         |
|       |                                           |
|       +---> Write pending action to action queue  |
|       |     (~/.tmux-notifications/actions/)      |
|       |                                           |
|       +---> Dispatch to all surfaces (async)      |
|       |          |         |        |       |     |
|       |          v         v        v       v     |
|       |       tmux      desktop   Slack   ntfy   |
|       |       popup     notif     msg     push   |
|       |                                           |
|       +---> Poll for response file (blocking)     |
|       |     (max 300s timeout)                    |
|       |                                           |
|       v                                           |
|  Response received from ANY surface               |
|       |                                           |
|       v                                           |
|  Return decision to Claude Code                   |
|  (JSON on stdout, exit 0)                         |
+--------------------------------------------------+
```

### 5.2 Component Architecture

```
+-------------------+
|   Action Gate     |    PreToolUse hook script
|   (Bash script)   |    Writes pending, dispatches, polls
+-------------------+
         |
         v
+-------------------+
|   Action Queue    |    File-based: ~/.tmux-notifications/actions/
|   (Filesystem)    |    {action_id}.pending  -> action request
|                   |    {action_id}.response -> user decision
|                   |    {action_id}.meta     -> routing metadata
+-------------------+
         |
    +----+----+----+----+
    |    |    |    |    |
    v    v    v    v    v
+------+ +------+ +------+ +------+ +------+
| tmux | |desktop| | Slack| | ntfy | | PWA  |
|popup | |notif  | | bot  | | push | | push |
+------+ +------+ +------+ +------+ +------+
    |         |        |        |        |
    v         v        v        v        v
+-------------------+
|  Action Router    |    Receives responses from all surfaces
|  (Daemon or       |    Writes {action_id}.response
|   per-surface     |    First response wins (atomic file creation)
|   callbacks)      |
+-------------------+
```

### 5.3 Action Queue File Format

**Pending action file** (`{action_id}.pending`):
```json
{
  "action_id": "a3f8c2d1e5b7094f",
  "session_id": "claude-session-xyz",
  "tool_use_id": "tool-use-123",
  "tool_name": "Bash",
  "tool_input": {
    "command": "npm test",
    "description": "Run the test suite"
  },
  "project": "my-app",
  "pane_id": "%42",
  "cwd": "/home/user/projects/my-app",
  "timestamp": 1711468500,
  "expires_at": 1711468800,
  "dispatched_to": ["tmux", "desktop", "slack", "ntfy"]
}
```

**Response file** (`{action_id}.response`):
```json
{
  "decision": "allow",
  "reason": "Approved by kai via Slack",
  "surface": "slack",
  "user": "U12345",
  "timestamp": 1711468512,
  "always": false
}
```

### 5.4 Action Gate (PreToolUse Hook)

```bash
#!/usr/bin/env bash
# scripts/action-gate.sh
# PreToolUse hook: dispatches action requests, waits for responses

set -euo pipefail

# Read hook input
JSON_DATA=$(cat)
TOOL_NAME=$(echo "$JSON_DATA" | jq -r '.tool_name')
SESSION_ID=$(echo "$JSON_DATA" | jq -r '.session_id')
TOOL_USE_ID=$(echo "$JSON_DATA" | jq -r '.tool_use_id')
CWD=$(echo "$JSON_DATA" | jq -r '.cwd')

# --- Configuration ---
ACTION_DIR="$HOME/.tmux-notifications/actions"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIMEOUT=${ACTION_GATE_TIMEOUT:-300}
SURFACES=${ACTION_GATE_SURFACES:-"tmux,desktop"}  # Comma-separated

mkdir -p "$ACTION_DIR"

# Generate cryptographically random action ID
ACTION_ID=$(openssl rand -hex 16)
ACTION_FILE="$ACTION_DIR/$ACTION_ID"

# Determine project name
PROJECT=$(basename "$CWD")

# --- Check if this tool is pre-approved ---
# (Skip the gate for tools the user has already approved)
APPROVED_FILE="$ACTION_DIR/.approved_rules"
if [ -f "$APPROVED_FILE" ] && grep -q "^${TOOL_NAME}:" "$APPROVED_FILE" 2>/dev/null; then
    exit 0  # Allow without gating
fi

# --- Write pending action ---
TOOL_INPUT=$(echo "$JSON_DATA" | jq -c '.tool_input')
NOW=$(date +%s)
EXPIRES=$((NOW + TIMEOUT))

cat > "$ACTION_FILE.pending" <<ENDJSON
{
  "action_id": "$ACTION_ID",
  "session_id": "$SESSION_ID",
  "tool_use_id": "$TOOL_USE_ID",
  "tool_name": "$TOOL_NAME",
  "tool_input": $TOOL_INPUT,
  "project": "$PROJECT",
  "pane_id": "${TMUX_PANE:-unknown}",
  "cwd": "$CWD",
  "timestamp": $NOW,
  "expires_at": $EXPIRES
}
ENDJSON

# --- Dispatch to surfaces (non-blocking) ---
IFS=',' read -ra SURFACE_LIST <<< "$SURFACES"
for surface in "${SURFACE_LIST[@]}"; do
    case "$surface" in
        tmux)
            # Show tmux popup/menu in background
            if [ -n "${TMUX_PANE:-}" ]; then
                tmux display-menu -T " Agent: $TOOL_NAME " -x C -y C \
                    "Allow"        a "run-shell '$SCRIPT_DIR/action-respond.sh allow $ACTION_ID'" \
                    "Deny"         d "run-shell '$SCRIPT_DIR/action-respond.sh deny $ACTION_ID'" \
                    "Allow Always" A "run-shell '$SCRIPT_DIR/action-respond.sh allow-always $ACTION_ID $TOOL_NAME'" \
                    2>/dev/null &
            fi
            ;;
        desktop)
            "$SCRIPT_DIR/dispatch-desktop.sh" "$ACTION_ID" "$TOOL_NAME" "$PROJECT" &
            ;;
        slack)
            "$SCRIPT_DIR/dispatch-slack.sh" "$ACTION_ID" &
            ;;
        ntfy)
            "$SCRIPT_DIR/dispatch-ntfy.sh" "$ACTION_ID" "$TOOL_NAME" "$PROJECT" &
            ;;
    esac
done

# --- Poll for response ---
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    if [ -f "$ACTION_FILE.response" ]; then
        DECISION=$(jq -r '.decision' "$ACTION_FILE.response")
        REASON=$(jq -r '.reason // "Remote approval"' "$ACTION_FILE.response")
        ALWAYS=$(jq -r '.always // false' "$ACTION_FILE.response")

        # Log the action
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] ACTION session=$SESSION_ID tool=$TOOL_NAME decision=$DECISION surface=$(jq -r '.surface' "$ACTION_FILE.response") response_time=${ELAPSED}s" \
            >> "$HOME/.tmux-notifications/action-audit.log"

        # Handle "Always Allow"
        if [ "$ALWAYS" = "true" ]; then
            echo "${TOOL_NAME}:allow" >> "$ACTION_DIR/.approved_rules"
        fi

        # Clean up
        rm -f "$ACTION_FILE.pending" "$ACTION_FILE.response"

        # Return decision to Claude Code
        cat <<ENDJSON
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "$DECISION",
    "permissionDecisionReason": "$REASON"
  }
}
ENDJSON
        exit 0
    fi
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done

# --- Timeout: clean up and fall through to normal prompt ---
rm -f "$ACTION_FILE.pending"
# Return "ask" to show the normal permission dialog
cat <<ENDJSON
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "ask"
  }
}
ENDJSON
exit 0
```

### 5.5 Action Response Handler

```bash
#!/usr/bin/env bash
# scripts/action-respond.sh
# Called by any surface to record a decision

DECISION="$1"     # allow, deny, allow-always
ACTION_ID="$2"
TOOL_NAME="${3:-}" # Only needed for allow-always

ACTION_DIR="$HOME/.tmux-notifications/actions"
RESPONSE_FILE="$ACTION_DIR/$ACTION_ID.response"
PENDING_FILE="$ACTION_DIR/$ACTION_ID.pending"

# Verify action exists and has not expired
if [ ! -f "$PENDING_FILE" ]; then
    echo "Action not found or already handled" >&2
    exit 1
fi

EXPIRES_AT=$(jq -r '.expires_at' "$PENDING_FILE")
NOW=$(date +%s)
if [ "$NOW" -gt "$EXPIRES_AT" ]; then
    rm -f "$PENDING_FILE"
    echo "Action expired" >&2
    exit 1
fi

# First-response-wins: use atomic file creation
# The ( set -o noclobber; echo > file ) pattern fails if file exists
if ( set -o noclobber; echo '{}' > "$RESPONSE_FILE" ) 2>/dev/null; then
    # We won the race -- write the actual response
    ALWAYS=false
    ACTUAL_DECISION="$DECISION"
    if [ "$DECISION" = "allow-always" ]; then
        ACTUAL_DECISION="allow"
        ALWAYS=true
    fi

    cat > "$RESPONSE_FILE" <<ENDJSON
{
  "decision": "$ACTUAL_DECISION",
  "reason": "Approved via action button",
  "surface": "${ACTION_SURFACE:-tmux}",
  "user": "${ACTION_USER:-local}",
  "timestamp": $NOW,
  "always": $ALWAYS
}
ENDJSON
else
    # Another surface already responded
    echo "Action already handled by another surface" >&2
    exit 0
fi
```

### 5.6 Race Condition Handling: First Response Wins

When the same action request is dispatched to multiple surfaces, the first response must win and all others must be rejected. The architecture uses atomic file creation with `noclobber`:

```bash
# Bash noclobber: fails if file already exists
if ( set -o noclobber; echo '{}' > "$RESPONSE_FILE" ) 2>/dev/null; then
    # This surface won -- write the real response
    cat > "$RESPONSE_FILE" <<EOF
{"decision": "allow", ...}
EOF
else
    # Another surface already responded -- no-op
    exit 0
fi
```

**Why this works:**
- `set -o noclobber` makes `>` fail with `EEXIST` if the file exists
- On POSIX filesystems, `O_CREAT | O_EXCL` (which noclobber maps to) is guaranteed atomic
- Only one process can "win" the creation
- The winning process then overwrites the placeholder `{}` with the real response
- The polling loop in the action gate reads the response once it contains a valid `decision` field

**After resolution:**
- Update the Slack message: "Approved by @kai" (replace original)
- Dismiss the tmux popup (if still open)
- Cancel the desktop notification
- Update the ntfy notification

Updating Slack/Discord messages after resolution:

```python
# In the Slack bot, after writing response:
respond(replace_original=True, text=f"Approved by <@{user_id}> at {timestamp}")
```

For tmux, the popup auto-dismisses when the user interacts. For desktop notifications, closing is platform-specific and not always possible.

### 5.7 Timeout Handling

When no surface responds within the timeout:

1. The PreToolUse hook returns `"permissionDecision": "ask"`
2. Claude Code shows its normal terminal permission prompt
3. The user at the terminal can approve/deny normally
4. Pending action files are cleaned up

This ensures the system degrades gracefully -- if all notification surfaces are unreachable, the agent still works via the normal terminal workflow.

### 5.8 Transport Failure Handling

**If the transport to the remote server is down when the user responds:**

For file-based responses (tmux, desktop notifications running on the same machine), this is not an issue -- the file write is local.

For remote surfaces (Slack, Discord, ntfy, PWA):

1. **Slack/Discord bot on the same machine as the agent:** The bot writes the response file locally. No remote transport needed.

2. **Slack/Discord bot on a separate server:** The callback endpoint must route to the correct machine. Options:
   - **SSH tunnel:** Callback URL points to an SSH-forwarded port
   - **Tailscale/VPN:** Callback URL uses the VPN address
   - **Message queue:** Use a shared Redis/NATS/file-sync system that both machines can access
   - **Retry with exponential backoff:** If the callback fails, queue it and retry

3. **ntfy action callbacks:** The HTTP action targets a URL on the dev machine. If the machine is unreachable:
   - The ntfy app shows an error ("Could not connect")
   - The user sees the failure and can retry
   - The action eventually expires and the terminal prompt takes over

**Architecture for multi-machine deployments:**

```
Phone (ntfy)                   Dev Machine (tmux + Claude)
     |                                |
     | HTTP action callback           |
     +------> [Public proxy / VPN] -->+
              (Tailscale, SSH -R,     |
               Cloudflare Tunnel)     v
                                Action Router
                                     |
                                     v
                              Write .response file
                                     |
                                     v
                              PreToolUse hook reads it
```

### 5.9 Dispatcher Scripts

**Desktop notification dispatcher (Linux):**
```bash
#!/usr/bin/env bash
# scripts/dispatch-desktop.sh

ACTION_ID="$1"
TOOL_NAME="$2"
PROJECT="$3"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

ACTION_DIR="$HOME/.tmux-notifications/actions"
PENDING="$ACTION_DIR/$ACTION_ID.pending"
TOOL_DETAIL=$(jq -r '.tool_input.command // .tool_input.file_path // "N/A"' "$PENDING")

export ACTION_SURFACE="desktop"
export ACTION_USER="local"

if command -v notify-send.py &>/dev/null; then
    # notify-send.py: blocking with action buttons
    RESPONSE=$(notify-send.py \
        "Agent: $TOOL_NAME ($PROJECT)" \
        "$TOOL_DETAIL" \
        --action allow:Allow deny:Deny always:"Allow Always" \
        --urgency critical \
        --expire-time 300000)

    case "$RESPONSE" in
        allow)  "$SCRIPT_DIR/action-respond.sh" allow "$ACTION_ID" ;;
        deny)   "$SCRIPT_DIR/action-respond.sh" deny "$ACTION_ID" ;;
        always) "$SCRIPT_DIR/action-respond.sh" allow-always "$ACTION_ID" "$TOOL_NAME" ;;
        close)  ;; # Dismissed, no action
    esac

elif [[ "$(uname)" == "Darwin" ]]; then
    # macOS: osascript dialog
    RESPONSE=$(osascript -e "
        button returned of (display dialog \"$TOOL_NAME: $TOOL_DETAIL\" \
            with title \"Agent Permission ($PROJECT)\" \
            buttons {\"Deny\", \"Allow\", \"Allow Always\"} \
            default button \"Allow\" \
            with icon caution \
            giving up after 300)" 2>/dev/null || echo "timeout")

    case "$RESPONSE" in
        "Allow")        "$SCRIPT_DIR/action-respond.sh" allow "$ACTION_ID" ;;
        "Deny")         "$SCRIPT_DIR/action-respond.sh" deny "$ACTION_ID" ;;
        "Allow Always") "$SCRIPT_DIR/action-respond.sh" allow-always "$ACTION_ID" "$TOOL_NAME" ;;
    esac

else
    # Fallback: plain notify-send (no action buttons)
    notify-send "Agent: $TOOL_NAME ($PROJECT)" "$TOOL_DETAIL" --urgency=critical 2>/dev/null
fi
```

**ntfy dispatcher:**
```bash
#!/usr/bin/env bash
# scripts/dispatch-ntfy.sh

ACTION_ID="$1"
TOOL_NAME="$2"
PROJECT="$3"

ACTION_DIR="$HOME/.tmux-notifications/actions"
PENDING="$ACTION_DIR/$ACTION_ID.pending"

NTFY_TOPIC="${NTFY_TOPIC:-agent-actions}"
NTFY_SERVER="${NTFY_SERVER:-https://ntfy.sh}"
CALLBACK_URL="${ACTION_CALLBACK_URL}"
ACTION_TOKEN="${ACTION_TOKEN}"

[ -z "$CALLBACK_URL" ] && exit 0  # ntfy not configured

TOOL_DETAIL=$(jq -r '.tool_input.command // .tool_input.file_path // "N/A"' "$PENDING")

curl -s "$NTFY_SERVER/$NTFY_TOPIC" \
    -H "Title: Agent: $TOOL_NAME ($PROJECT)" \
    -H "Priority: high" \
    -H "Tags: robot,warning" \
    -H "Click: ${CALLBACK_URL}/details/${ACTION_ID}" \
    -H "Actions: http, Allow, ${CALLBACK_URL}/allow/${ACTION_ID}, method=POST, headers.Authorization=Bearer ${ACTION_TOKEN}; http, Deny, ${CALLBACK_URL}/deny/${ACTION_ID}, method=POST, headers.Authorization=Bearer ${ACTION_TOKEN}" \
    -d "$TOOL_DETAIL"
```

### 5.10 Callback HTTP Server

For ntfy and PWA action buttons, a lightweight HTTP server must run on or be reachable from the dev machine:

```bash
#!/usr/bin/env bash
# scripts/action-server.sh
# Minimal HTTP callback server using socat or ncat
# For production, use a proper server (Python, Go, etc.)

ACTION_DIR="$HOME/.tmux-notifications/actions"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT="${ACTION_SERVER_PORT:-18423}"
TOKEN="${ACTION_TOKEN}"

handle_request() {
    read -r METHOD PATH _
    # Read headers
    while read -r header; do
        header=$(echo "$header" | tr -d '\r')
        [ -z "$header" ] && break
        case "$header" in
            Authorization:*) AUTH="${header#Authorization: }" ;;
        esac
    done

    # Validate token
    if [ "$AUTH" != "Bearer $TOKEN" ]; then
        echo -e "HTTP/1.1 403 Forbidden\r\nContent-Length: 12\r\n\r\nUnauthorized"
        return
    fi

    # Parse path: /allow/{action_id} or /deny/{action_id}
    DECISION=$(echo "$PATH" | cut -d'/' -f2)
    ACTION_ID=$(echo "$PATH" | cut -d'/' -f3)

    export ACTION_SURFACE="remote"
    export ACTION_USER="remote"

    case "$DECISION" in
        allow|deny)
            "$SCRIPT_DIR/action-respond.sh" "$DECISION" "$ACTION_ID"
            echo -e "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
            ;;
        details)
            CONTENT=$(cat "$ACTION_DIR/$ACTION_ID.pending" 2>/dev/null || echo '{"error":"not found"}')
            LEN=${#CONTENT}
            echo -e "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: $LEN\r\n\r\n$CONTENT"
            ;;
        *)
            echo -e "HTTP/1.1 404 Not Found\r\nContent-Length: 9\r\n\r\nNot found"
            ;;
    esac
}

echo "Action callback server listening on port $PORT"
while true; do
    echo "" | ncat -l -p "$PORT" -c "$(declare -f handle_request); handle_request"
done
```

For production use, a minimal Python or Go server is more appropriate:

```python
#!/usr/bin/env python3
# scripts/action-server.py
"""Minimal action callback server for ntfy/PWA button responses."""

import json
import os
import subprocess
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler

ACTION_DIR = os.path.expanduser("~/.tmux-notifications/actions")
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
TOKEN = os.environ.get("ACTION_TOKEN", "")
PORT = int(os.environ.get("ACTION_SERVER_PORT", "18423"))

class ActionHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        # Validate auth
        auth = self.headers.get("Authorization", "")
        if TOKEN and auth != f"Bearer {TOKEN}":
            self.send_error(403, "Unauthorized")
            return

        parts = self.path.strip("/").split("/")
        if len(parts) != 2:
            self.send_error(404)
            return

        decision, action_id = parts

        if decision not in ("allow", "deny", "allow-always"):
            self.send_error(400, "Invalid decision")
            return

        # Validate action_id format (hex only)
        if not all(c in "0123456789abcdef" for c in action_id):
            self.send_error(400, "Invalid action ID")
            return

        env = os.environ.copy()
        env["ACTION_SURFACE"] = "remote"
        env["ACTION_USER"] = "remote"

        result = subprocess.run(
            [os.path.join(SCRIPT_DIR, "action-respond.sh"), decision, action_id],
            env=env, capture_output=True, text=True
        )

        if result.returncode == 0:
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"OK")
        else:
            self.send_error(409, result.stderr.strip())

    def do_GET(self):
        parts = self.path.strip("/").split("/")
        if len(parts) == 2 and parts[0] == "details":
            action_id = parts[1]
            pending = os.path.join(ACTION_DIR, f"{action_id}.pending")
            if os.path.exists(pending):
                with open(pending) as f:
                    data = f.read()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(data.encode())
            else:
                self.send_error(404, "Action not found")
        else:
            self.send_error(404)

    def log_message(self, format, *args):
        pass  # Suppress default logging

if __name__ == "__main__":
    server = HTTPServer(("0.0.0.0", PORT), ActionHandler)
    print(f"Action callback server on port {PORT}", file=sys.stderr)
    server.serve_forever()
```

### 5.11 Hook Configuration

The hooks are registered in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash|Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "~/.tmux/plugins/tmux-agent-notifications/scripts/action-gate.sh",
            "timeout": 310000
          }
        ]
      }
    ],
    "Notification": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "~/.tmux/plugins/tmux-agent-notifications/scripts/claude-hook.sh Notification"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "~/.tmux/plugins/tmux-agent-notifications/scripts/claude-hook.sh Stop"
          }
        ]
      }
    ]
  }
}
```

**Important:** The PreToolUse hook timeout (310000ms = 310 seconds) must exceed the action gate's internal timeout (300 seconds) to prevent Claude Code from killing the hook before it finishes polling.

### 5.12 tmux Plugin Configuration

```tmux
# Enable action buttons (off by default)
set -g @claude-notif-action-buttons "on"

# Surfaces to dispatch to (comma-separated)
set -g @claude-notif-action-surfaces "tmux,desktop"

# Timeout in seconds (default 300)
set -g @claude-notif-action-timeout "300"

# For ntfy mobile notifications
set -g @claude-notif-ntfy-topic "my-agent-actions"
set -g @claude-notif-ntfy-server "https://ntfy.sh"

# For Slack
set -g @claude-notif-slack-channel "#agent-approvals"

# Action callback server port (for ntfy/PWA remote callbacks)
set -g @claude-notif-action-server-port "18423"
```

### 5.13 Sequence Diagram: Full Action Flow

```
User          Phone(ntfy)    Slack Bot     tmux popup    Desktop      Action Gate    Claude Code
 |               |              |              |            |              |              |
 |               |              |              |            |              |<-- PreToolUse |
 |               |              |              |            |              |    (Bash:     |
 |               |              |              |            |              |     npm test) |
 |               |              |              |            |              |              |
 |               |              |              |            |              |-- Write       |
 |               |              |              |            |              |   .pending    |
 |               |              |              |            |              |              |
 |               |<-- push -----|              |<-- menu ---|<-- notify ---|-- Dispatch    |
 |               |              |<-- msg ------|            |              |   to surfaces |
 |               |              |              |            |              |              |
 |               |              |              |            |              |-- Poll for    |
 |               |              |              |            |              |   .response   |
 |               |              |              |            |              |   (blocking)  |
 |               |              |              |            |              |              |
 |-- tap Allow ->|              |              |            |              |              |
 |               |-- HTTP POST->|              |            |              |              |
 |               |  (callback)  |              |            |              |              |
 |               |              |              |            |              |              |
 |               |        [callback server]    |            |              |              |
 |               |              |              |            |              |              |
 |               |              |              |            |   Write      |              |
 |               |              |              |            | .response <--|              |
 |               |              |              |            |  (noclobber) |              |
 |               |              |              |            |              |              |
 |               |              |              |            |              |-- Read        |
 |               |              |              |            |              |   .response   |
 |               |              |              |            |              |              |
 |               |              |              |            |              |-- Return      |
 |               |              |              |            |              |   "allow" --> |
 |               |              |              |            |              |              |
 |               |              |              |            |              |    Tool runs  |
 |               |              |              |            |              |    npm test   |
```

### 5.14 Implementation Phases

| Phase | Components | Effort | Value |
|-------|-----------|--------|-------|
| **Phase 1: Local tmux** | action-gate.sh, action-respond.sh, tmux display-menu integration | 1 week | Core functionality for terminal users |
| **Phase 2: Desktop** | dispatch-desktop.sh (notify-send.py + osascript) | 2-3 days | Works when developer is not in tmux |
| **Phase 3: ntfy mobile** | dispatch-ntfy.sh, action-server.py, callback routing | 1 week | Phone approval from anywhere |
| **Phase 4: Slack** | Slack bot (Bolt + Socket Mode), dispatch-slack.sh | 1 week | Team visibility and approval |
| **Phase 5: Discord** | Discord bot, dispatch-discord.sh | 3-4 days | Alternative team surface |
| **Phase 6: PWA** | Service worker, push subscription, web UI | 2 weeks | Cross-platform mobile without app |

### 5.15 Open Questions

1. **Should the PreToolUse hook block for the full timeout, or should it return "ask" quickly and let the Notification hook handle remote dispatch?** Blocking gives a clean single-decision-point architecture but makes Claude Code appear frozen during the wait. Returning "ask" lets the terminal prompt appear immediately but complicates the response routing (now the response must inject keystrokes instead of returning a hook decision).

2. **Should the action callback server be a persistent daemon or started on-demand by the hook?** A daemon avoids startup latency but adds another process to manage. On-demand startup is simpler but adds 0.5-1s latency to the first callback.

3. **How should the plugin handle multiple Claude Code sessions requesting approval simultaneously?** Each session's PreToolUse hook runs independently and writes its own pending action. The action ID ensures no cross-session confusion. But the user's phone might show 5 notifications in a row, which is noisy. Grouping/batching notifications across sessions is a future optimization.

4. **Is the file-based action queue the right choice, or should we use a Unix socket or named pipe?** Files are simple, debuggable, and work with the noclobber atomic creation pattern. But they require polling (sleep 1 loop). A Unix socket or named pipe would allow the action gate to block on a read instead of polling, reducing latency and CPU usage. The trade-off is complexity and the need to manage the socket lifecycle.

5. **For the PermissionRequest hook (which can update permission rules), should "Allow Always" be implemented there instead of in PreToolUse?** Using PermissionRequest would let us update Claude Code's actual permission rules (session or project-level), making "Allow Always" persistent without our own `.approved_rules` file. The downside is that PermissionRequest fires later in the flow (after the permission dialog is about to show), so the timing with external dispatch is more complex.

---

## Research Sources

### Claude Code Hooks
- [Claude Code Hooks Reference (official)](https://code.claude.com/docs/en/hooks)
- [Agent SDK Hooks Documentation](https://platform.claude.com/docs/en/agent-sdk/hooks)
- [Claude Code Hooks: All Events (Pixelmojo)](https://www.pixelmojo.io/blogs/claude-code-hooks-production-quality-ci-cd-patterns)
- [Claude Code Hook Control Flow (Steve Kinney)](https://stevekinney.com/courses/ai-development/claude-code-hook-control-flow)
- [Claude Code Hook Examples (Steve Kinney)](https://stevekinney.com/courses/ai-development/claude-code-hook-examples)
- [Claude Code Hooks Complete Guide (SmartScope)](https://smartscope.blog/en/generative-ai/claude/claude-code-hooks-guide/)
- [Claude Code Hooks Guide (Serenities AI)](https://serenitiesai.com/articles/claude-code-hooks-guide-2026)
- [Configure Permissions (Anthropic)](https://platform.claude.com/docs/en/agent-sdk/permissions)

### Claude Code Programmatic Control
- [Run Claude Code Programmatically (official)](https://code.claude.com/docs/en/headless)
- [Programmatic Input Submission Feature Request (GitHub #15553)](https://github.com/anthropics/claude-code/issues/15553)
- [Claude Code Hooks Mastery (GitHub)](https://github.com/disler/claude-code-hooks-mastery)

### tmux
- [tmux Manual Page](https://www.man7.org/linux/man-pages/man1/tmux.1.html)
- [tmux display-popup Guide (tmuxai)](https://tmuxai.dev/tmux-popup/)
- [tmux send-keys Guide (tmuxai)](https://tmuxai.dev/tmux-send-keys/)
- [tmux Menus (Kolkhis Notes)](https://notes.kolkhis.dev/linux/tmux/menus/)
- [tmux Session Switching with Menus (qmacro)](https://qmacro.org/blog/posts/2021/08/12/session-switching-with-the-tmux-menu/)
- [tmux send-keys Issue #1425](https://github.com/tmux/tmux/issues/1425)
- [tmux-menus Plugin](https://github.com/jaclu/tmux-menus)

### Desktop Notifications
- [notify-send.py (GitHub)](https://github.com/phuhl/notify-send.py)
- [notify-send.sh (GitHub)](https://github.com/vlevit/notify-send.sh)
- [Mac Automation Scripting Guide: Dialogs (Apple)](https://developer.apple.com/library/archive/documentation/LanguagesUtilities/Conceptual/MacAutomationScriptingGuide/DisplayDialogsandAlerts.html)
- [AppleScript Display Dialog (Fandom Wiki)](https://applescript.fandom.com/wiki/Display_Dialog)
- [User Interaction from Bash Scripts (Scripting OS X)](https://scriptingosx.com/2018/08/user-interaction-from-bash-scripts/)
- [notify-send Man Page (Debian)](https://manpages.debian.org/testing/libnotify-bin/notify-send.1.en.html)

### Slack
- [Slack Socket Mode (official)](https://docs.slack.dev/apis/events-api/using-socket-mode/)
- [Slack Interactive Messages (official)](https://api.slack.com/messaging/interactivity)
- [Slack Block Kit (official)](https://docs.slack.dev/block-kit/)
- [Bolt for Python (GitHub)](https://github.com/slackapi/bolt-python)
- [Getting Started with Bolt for Python](https://slack.dev/bolt-python/tutorial/getting-started/)

### Discord
- [Discord Components V2: Buttons in Webhooks](https://discord-webhook.com/en/blog/discord-components-v2-guide/)
- [Discord Webhook Resource (official)](https://docs.discord.com/developers/resources/webhook)
- [Interactive Components (Embed Generator)](https://message.style/docs/guides/interactive-components/)

### Mobile / Push Notifications
- [ntfy.sh Official Site](https://ntfy.sh/)
- [ntfy Publishing Documentation](https://docs.ntfy.sh/publish/)
- [ntfy Action Buttons Issue (#134)](https://github.com/binwiederhier/ntfy/issues/134)
- [Web Push Notification Behavior (web.dev)](https://web.dev/articles/push-notifications-notification-behaviour)
- [Notification Actions in Chrome](https://developer.chrome.com/blog/notification-actions)
- [Service Worker notificationclick (MDN)](https://developer.mozilla.org/en-US/docs/Web/API/ServiceWorkerGlobalScope/notificationclick_event)
- [PWA Push Notifications on iOS (2026)](https://webscraft.org/blog/pwa-pushspovischennya-na-ios-u-2026-scho-realno-pratsyuye?lang=en)
- [Push Notifications in PWAs (MagicBell)](https://www.magicbell.com/blog/using-push-notifications-in-pwas)
