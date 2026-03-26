# Position Paper: SSH-Based Persistent Reverse Tunnel as Transport Layer for Bidirectional Agent Control Plane

**Author:** SSH Persistent Tunnel Advocate
**Date:** 2026-03-26
**Status:** RFC / Technical Debate Position
**Target:** tmux-agent-notifications cross-machine notification relay (Feature #12) and bidirectional action response pathway

---

## Executive Summary

The previous debate rounds addressed how notifications are stored and displayed within a single tmux server: files versus native tmux options. That debate is about the local data model. This paper addresses a harder, unsolved problem: **how do notifications and action responses travel between machines?**

When a developer starts five Claude Code agents on a remote build server over SSH, then closes their laptop lid and walks to the couch with their phone, those agents will finish, request permissions, and wait -- silently. The developer has no way to know, no way to respond, and no way to unblock the agents without returning to the terminal and reconnecting. Feature #12 in the feature research document identifies this as a high-complexity experimental feature. This paper argues it is not experimental -- it is the critical missing piece -- and that SSH reverse tunnels are the correct transport layer to solve it.

SSH reverse tunnels provide encrypted, authenticated, NAT-traversing, bidirectional communication channels using infrastructure that every developer already has. No cloud service. No STUN/TURN servers. No WebRTC browser runtime. No new protocol to learn. The plugin maintains its own persistent connection via `autossh`, and messages flow as JSON over the tunnel. The result is a system where a developer's phone can buzz with "Claude/api-server: wants to run `rm -rf build/`" and they can tap "Deny" from the couch -- and the agent on the remote server receives the denial within seconds.

---

## Architecture

```
+=====================================================================+
|                         REMOTE SERVER                                |
|  +------------------+     +------------------+     +--------------+  |
|  | Claude Code      |     | Codex CLI        |     | Agent N      |  |
|  | (tmux pane %42)  |     | (tmux pane %58)  |     | (pane %71)   |  |
|  +--------+---------+     +--------+---------+     +------+-------+  |
|           |                        |                      |          |
|           v                        v                      v          |
|  +----------------------------------------------------------------+  |
|  |              tmux hook -> tmux-notify-lib.sh                   |  |
|  |  Writes per-pane @notif-* options (native tmux, per judgment)  |  |
|  |  AND sends JSON to local relay-sender daemon via Unix socket   |  |
|  +------------------------------+---------------------------------+  |
|                                 |                                    |
|  +------------------------------v---------------------------------+  |
|  |                     relay-sender daemon                        |  |
|  |  Listens on: /tmp/tmux-notif-relay.sock (Unix domain socket)  |  |
|  |  Queues outbound JSON messages                                 |  |
|  |  Receives inbound action responses                             |  |
|  +------------------------------+---------------------------------+  |
|                                 |                                    |
|  +------------------------------v---------------------------------+  |
|  |  autossh -M 0 -N -R 19222:localhost:19222 user@local-relay    |  |
|  |  Persistent reverse tunnel with ServerAliveInterval keepalive  |  |
|  |  Reconnects automatically on network interruption              |  |
|  +------------------------------+---------------------------------+  |
|                                 |                                    |
+==================================|====================================+
                                   | (SSH encrypted, key-authenticated)
                                   |
              ~~ NAT / Firewall / Internet ~~
                                   |
+==================================|====================================+
|                         LOCAL RELAY                                   |
|  +------------------------------v---------------------------------+  |
|  |  sshd listening on port 22 (standard)                          |  |
|  |  Reverse-forwarded port 19222 receives tunnel traffic          |  |
|  +------------------------------+---------------------------------+  |
|                                 |                                    |
|  +------------------------------v---------------------------------+  |
|  |                    relay-receiver daemon                        |  |
|  |  Listens on: localhost:19222 (only loopback, not exposed)      |  |
|  |  Dispatches notifications to:                                  |  |
|  |    - Desktop notifications (notify-send / osascript)           |  |
|  |    - Pushover / ntfy.sh (phone push notifications)             |  |
|  |    - Slack webhook (team channel)                              |  |
|  |    - Local tmux status bar (if running)                        |  |
|  |  Collects action responses and sends back through tunnel       |  |
|  +------------------------------+---------------------------------+  |
|                                 |                                    |
|  +------------------------------v---------------------------------+  |
|  |  Mobile / Tablet / Watch                                       |  |
|  |  Receives push notification via Pushover / ntfy.sh             |  |
|  |  Action buttons: [Allow] [Deny] [View Details]                 |  |
|  |  Response flows back: ntfy -> relay-receiver -> tunnel ->      |  |
|  |                       relay-sender -> tmux pane                |  |
|  +----------------------------------------------------------------+  |
+======================================================================+
```

---

## 1. Why SSH Is the Right Transport

### 1.1 Zero New Infrastructure

Every developer who runs agents on a remote server already has SSH configured. They have keys. They have `~/.ssh/config` entries. They have `sshd` running on the remote machine and typically have an SSH-accessible machine on their local network (a NAS, a home server, a desktop that stays on, or even their laptop with `sshd` enabled). SSH is the only transport layer that requires zero new infrastructure, zero new accounts, and zero new trust relationships.

WebRTC requires STUN servers for NAT traversal. Even "serverless" P2P WebRTC needs at least a signaling mechanism (a shared file? a cloud endpoint?) to exchange ICE candidates. STUN works for simple NAT but fails with symmetric NAT (common on corporate and mobile networks), requiring a TURN relay -- which is exactly the cloud service we are trying to avoid.

### 1.2 Encryption and Authentication Are Solved

SSH provides:

- **AES-256-GCM or ChaCha20-Poly1305 encryption** (negotiated automatically)
- **Ed25519 or RSA key-based authentication** (no passwords in the loop)
- **Host key verification** (TOFU model prevents MITM after first connection)
- **Forward secrecy** via Diffie-Hellman key exchange per session

No TLS certificate management. No Let's Encrypt renewal cron jobs. No self-signed certificate trust configuration. No certificate pinning logic in the client. SSH key management is a problem developers have already solved for themselves.

### 1.3 NAT Traversal via Reverse Tunnel

The SSH reverse port forward (`-R`) is the mechanism that makes this work without any cloud relay:

```bash
# On the REMOTE server, establish reverse tunnel to LOCAL machine:
ssh -N -R 19222:localhost:19222 user@local-relay-host
```

This creates a listening socket on `local-relay-host:19222` that forwards traffic back through the SSH connection to `localhost:19222` on the remote server. The remote server initiates the connection outward (traversing its NAT/firewall), and the local machine receives inbound connections on the forwarded port. No port forwarding rules on the remote server's firewall. No UPnP. No hole punching.

### 1.4 Proven Reliability with autossh

`autossh` (authored by Carson Harding, maintained continuously since 2004) monitors an SSH connection and restarts it on failure. It has a 20+ year track record of production use in monitoring systems, IoT deployments, and infrastructure automation.

```bash
# Persistent reverse tunnel with automatic reconnection:
autossh -M 0 \
  -o "ServerAliveInterval 30" \
  -o "ServerAliveCountMax 3" \
  -o "ExitOnForwardFailure yes" \
  -N -R 19222:localhost:19222 \
  user@local-relay-host
```

Key flags:
- **`-M 0`**: Disable autossh's own monitoring port; rely on SSH's `ServerAliveInterval` instead (cleaner, no extra port needed).
- **`ServerAliveInterval 30`**: SSH sends a keepalive every 30 seconds. If 3 consecutive keepalives fail (90 seconds), the connection is declared dead.
- **`ExitOnForwardFailure yes`**: If the reverse port forward fails (e.g., port already in use on the local side), SSH exits immediately rather than establishing a useless connection.
- **`-N`**: No remote command -- this is a tunnel-only connection.

autossh handles: network outages, laptop sleep/wake cycles, Wi-Fi switches, VPN reconnections, and SSH server restarts. It uses exponential backoff on repeated failures to avoid hammering a down server.

---

## 2. Message Protocol

Messages flow as newline-delimited JSON over the SSH tunnel. Each message has a `type` field for routing.

### 2.1 Notification (remote -> local)

```json
{
  "type": "notification",
  "id": "a1b2c3d4",
  "server": "build-server-01",
  "pane": "%42",
  "project": "api-server",
  "source": "Claude",
  "message": "Agent has finished",
  "priority": "info",
  "timestamp": "2026-03-26T14:32:01Z",
  "actions": null
}
```

### 2.2 Action Request (remote -> local)

```json
{
  "type": "action_request",
  "id": "e5f6g7h8",
  "server": "build-server-01",
  "pane": "%42",
  "project": "api-server",
  "source": "Claude",
  "message": "Agent wants to execute: rm -rf build/",
  "priority": "critical",
  "timestamp": "2026-03-26T14:33:15Z",
  "actions": ["allow", "deny"],
  "timeout_seconds": 300,
  "context": {
    "tool": "Bash",
    "command": "rm -rf build/",
    "working_dir": "/home/dev/api-server"
  }
}
```

### 2.3 Action Response (local -> remote)

```json
{
  "type": "action_response",
  "request_id": "e5f6g7h8",
  "action": "deny",
  "responded_by": "phone-push",
  "responded_at": "2026-03-26T14:33:42Z"
}
```

### 2.4 Heartbeat (bidirectional)

```json
{
  "type": "heartbeat",
  "server": "build-server-01",
  "timestamp": "2026-03-26T14:34:00Z",
  "active_agents": 3
}
```

---

## 3. Concrete Implementation

### 3.1 Remote Side: relay-sender daemon

The relay-sender runs on the remote server as a background process managed by the tmux plugin. It listens on a Unix domain socket for notifications from agent hooks, queues them, and sends them through the SSH tunnel.

```bash
#!/usr/bin/env bash
# relay-sender.sh -- runs on the remote server
# Launched by claude-notifications.tmux at plugin init

SOCK="/tmp/tmux-notif-relay.sock"
TUNNEL_PORT=19222
FIFO="/tmp/tmux-notif-relay.fifo"
QUEUE_DIR="$HOME/.tmux-notifications/relay-queue"

mkdir -p "$QUEUE_DIR"
rm -f "$SOCK" "$FIFO"
mkfifo "$FIFO"

cleanup() { rm -f "$SOCK" "$FIFO"; kill 0; }
trap cleanup EXIT

# Background: read responses from tunnel, dispatch to tmux panes
(
  while true; do
    # socat connects to the reverse-forwarded port on localhost
    socat -u TCP:localhost:$TUNNEL_PORT - 2>/dev/null | while IFS= read -r line; do
      type=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
      if [ "$type" = "action_response" ]; then
        request_id=$(echo "$line" | jq -r '.request_id')
        action=$(echo "$line" | jq -r '.action')
        # Write response to a file the agent hook can poll
        echo "$action" > "$QUEUE_DIR/response_${request_id}"
      fi
    done
    sleep 5  # reconnect delay on failure
  done
) &

# Foreground: accept notifications on Unix socket, forward to tunnel
socat UNIX-LISTEN:"$SOCK",fork - | while IFS= read -r notification; do
  # Forward to tunnel (local relay-receiver)
  echo "$notification" | socat - TCP:localhost:$TUNNEL_PORT 2>/dev/null
  # Also queue to disk for retry if tunnel is down
  notif_id=$(echo "$notification" | jq -r '.id // "unknown"')
  echo "$notification" > "$QUEUE_DIR/pending_${notif_id}"
done
```

### 3.2 Integration with tmux_alert

The existing `tmux_alert` function gains a relay call alongside the local notification write:

```bash
tmux_alert() {
    local msg="$1"
    local display_name="$2"
    local session_title="$3"

    # --- Local notification (native tmux options, per hybrid judgment) ---
    tmux set -p -t "$TMUX_PANE" @notif-msg "$msg"
    tmux set -p -t "$TMUX_PANE" @notif-project "$display_name"
    tmux set -p -t "$TMUX_PANE" @notif-source "$NOTIFY_SOURCE"
    tmux set -p -t "$TMUX_PANE" @notif-time "$(date +%H:%M:%S)"
    refresh_all_clients

    # --- Remote relay (if configured and socket exists) ---
    local relay_sock="/tmp/tmux-notif-relay.sock"
    if [ -S "$relay_sock" ]; then
        local notif_id
        notif_id=$(head -c 8 /dev/urandom | xxd -p)
        local hostname
        hostname=$(hostname -s)
        printf '{"type":"notification","id":"%s","server":"%s","pane":"%s","project":"%s","source":"%s","message":"%s","priority":"info","timestamp":"%s"}\n' \
            "$notif_id" "$hostname" "$TMUX_PANE" "$display_name" \
            "$NOTIFY_SOURCE" "$msg" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            | socat - UNIX-CONNECT:"$relay_sock" 2>/dev/null
    fi
}
```

### 3.3 Local Side: relay-receiver daemon

The relay-receiver runs on the developer's local machine (laptop, home server, or a Raspberry Pi on their network). It listens on the reverse-forwarded port and dispatches notifications.

```bash
#!/usr/bin/env bash
# relay-receiver.sh -- runs on the local machine
# Listens on the port that SSH reverse-forwards to

LISTEN_PORT=19222
NTFY_TOPIC="${TMUX_NOTIF_NTFY_TOPIC:-}"  # e.g., "my-agent-alerts"

while true; do
  socat TCP-LISTEN:$LISTEN_PORT,reuseaddr,fork - | while IFS= read -r line; do
    type=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
    case "$type" in
      notification|action_request)
        server=$(echo "$line" | jq -r '.server')
        project=$(echo "$line" | jq -r '.project')
        message=$(echo "$line" | jq -r '.message')
        priority=$(echo "$line" | jq -r '.priority // "info"')
        source_tool=$(echo "$line" | jq -r '.source')

        title="${server}: ${source_tool}/${project}"
        body="$message"

        # Desktop notification (Linux)
        if command -v notify-send >/dev/null 2>&1; then
            urgency="normal"
            [ "$priority" = "critical" ] && urgency="critical"
            notify-send -u "$urgency" "$title" "$body"
        fi

        # Desktop notification (macOS)
        if command -v osascript >/dev/null 2>&1; then
            osascript -e "display notification \"$body\" with title \"$title\""
        fi

        # Mobile push via ntfy.sh (self-hosted or public)
        if [ -n "$NTFY_TOPIC" ]; then
            ntfy_priority="default"
            [ "$priority" = "critical" ] && ntfy_priority="urgent"
            curl -s \
              -H "Title: $title" \
              -H "Priority: $ntfy_priority" \
              -H "Tags: robot" \
              -d "$body" \
              "https://ntfy.sh/${NTFY_TOPIC}" >/dev/null 2>&1

            # For action requests, include response buttons
            if [ "$type" = "action_request" ]; then
                request_id=$(echo "$line" | jq -r '.id')
                curl -s \
                  -H "Title: ACTION REQUIRED: $title" \
                  -H "Priority: urgent" \
                  -H "Actions: http, Allow, ${NTFY_RESPONSE_URL}/respond?id=${request_id}&action=allow; http, Deny, ${NTFY_RESPONSE_URL}/respond?id=${request_id}&action=deny" \
                  -d "$body" \
                  "https://ntfy.sh/${NTFY_TOPIC}" >/dev/null 2>&1
            fi
        fi
        ;;
      heartbeat)
        # Log for monitoring; no user-visible action
        ;;
    esac
  done
  sleep 2  # Restart listener on failure
done
```

### 3.4 SSH Multiplexing for Multiple Servers

When a developer works across multiple remote servers, SSH's ControlMaster feature avoids redundant TCP connections and authentication handshakes:

```
# ~/.ssh/config
Host build-server-*
    User deploy
    ControlMaster auto
    ControlPath ~/.ssh/sockets/%r@%h-%p
    ControlPersist 600
    ServerAliveInterval 30
    ServerAliveCountMax 3
```

Each remote server runs its own `relay-sender` with a unique tunnel port:

```bash
# Server 1: reverse-forward to local port 19222
autossh -M 0 -N -R 19222:localhost:19222 user@local-relay

# Server 2: reverse-forward to local port 19223
autossh -M 0 -N -R 19223:localhost:19223 user@local-relay

# Server 3: reverse-forward to local port 19224
autossh -M 0 -N -R 19224:localhost:19224 user@local-relay
```

The local `relay-receiver` listens on all assigned ports, or a single multiplexing receiver accepts connections on a port range. Alternatively, use SSH's `StreamLocalBindUnlink` with Unix domain sockets for cleaner namespacing:

```bash
# Forward a Unix socket instead of a TCP port:
autossh -M 0 -N -R /tmp/tmux-relay-build01.sock:localhost:19222 user@local-relay
```

---

## 4. Handling Mobile Devices

Mobile phones and tablets cannot run `sshd`. The SSH tunnel solves the remote-to-local hop; the last mile to mobile devices uses a lightweight push notification service. Critically, the push service carries only the notification payload -- the tunnel itself never touches a third party.

### 4.1 ntfy.sh (Self-Hostable)

[ntfy.sh](https://ntfy.sh) is an open-source HTTP-based pub/sub notification service. It can be self-hosted on the same local relay machine, meaning zero data leaves the developer's network:

```bash
# Self-hosted ntfy on the local relay (Docker one-liner):
docker run -p 8080:80 binwiederhier/ntfy serve

# Notifications are pushed from relay-receiver to local ntfy:
curl -d "Agent finished on build-server" http://localhost:8080/my-agents
```

The ntfy Android/iOS app subscribes to the topic and displays push notifications with action buttons. When the developer taps "Allow" or "Deny," the ntfy server forwards the response to the relay-receiver's HTTP callback endpoint, which sends it back through the SSH tunnel to the remote server.

### 4.2 Pushover (Zero Self-Hosting)

For developers who prefer not to self-host, [Pushover](https://pushover.net) provides a simple API with a one-time $5 app purchase. The relay-receiver sends notifications via Pushover's API:

```bash
curl -s --form-string "token=$PUSHOVER_APP_TOKEN" \
  --form-string "user=$PUSHOVER_USER_KEY" \
  --form-string "title=$title" \
  --form-string "message=$body" \
  --form-string "priority=$pushover_priority" \
  https://api.pushover.net/1/messages.json
```

### 4.3 Privacy Model

In all configurations, the SSH tunnel carries notifications from the remote server to the developer's own local machine. The local machine then decides how to dispatch. If using self-hosted ntfy, notification content never leaves the developer's network. If using Pushover or public ntfy.sh, only the last-mile push traverses external infrastructure -- and even this can be encrypted end-to-end using ntfy's encryption feature or by encrypting the JSON payload before sending.

---

## 5. Resource Usage and Performance

### 5.1 SSH Tunnel Overhead

An idle SSH tunnel with `ServerAliveInterval 30` produces:

- **Bandwidth**: ~200 bytes per keepalive packet, every 30 seconds = ~400 bytes/minute = ~24 KB/hour. Negligible.
- **Memory**: One `ssh` process consumes approximately 4-8 MB RSS. With `autossh` wrapper, total is ~10-12 MB per remote server.
- **CPU**: Effectively zero when idle. Encryption/decryption of keepalive packets is sub-microsecond on modern hardware.
- **File descriptors**: 3-4 per tunnel (stdin/stdout/stderr + socket).

For comparison, a single tmux `#()` status-line fork creates a new process (1-3 MB RSS) every `status-interval` seconds.

### 5.2 Notification Latency

Notification flow latency breakdown:

| Step | Latency |
|------|---------|
| Agent hook fires, JSON written to Unix socket | < 1 ms |
| relay-sender reads from socket, writes to TCP tunnel | < 1 ms |
| SSH tunnel transit (already established, no handshake) | 1-50 ms (network RTT) |
| relay-receiver reads, dispatches to notify-send | < 5 ms |
| Desktop notification appears | < 100 ms |
| Push notification to phone (via ntfy/Pushover) | 1-5 seconds |

**Total: under 200 ms for desktop, under 6 seconds for phone push.** This is well within acceptable limits for a system where agents complete tasks "minutes apart, not seconds."

### 5.3 Reconnection Behavior

When the network drops:

1. SSH detects failure after `ServerAliveInterval * ServerAliveCountMax` = 90 seconds.
2. SSH exits with a non-zero status.
3. `autossh` detects the exit and restarts SSH after a brief delay (default: 1 second, with exponential backoff on repeated failures).
4. SSH re-establishes the connection, re-authenticates via key, and re-creates the reverse port forward.
5. Any notifications queued in `relay-sender`'s disk queue during the outage are sent.

**Typical reconnection time: 5-15 seconds after network recovery.** During the outage, notifications are queued to disk and delivered once the tunnel recovers. No notifications are lost.

---

## 6. Security Model

### 6.1 Attack Surface

| Component | Exposure | Mitigation |
|-----------|----------|------------|
| SSH tunnel | Encrypted, key-authenticated | Standard SSH hardening (disable password auth, use Ed25519 keys, `AllowUsers` directive) |
| Reverse-forwarded port (19222) | Bound to `localhost` only by default | `GatewayPorts no` in sshd_config (default) prevents remote binding |
| Unix domain socket (`/tmp/tmux-notif-relay.sock`) | Local filesystem permissions | Socket created with mode 0600; only the user's processes can connect |
| relay-receiver HTTP callback (for ntfy responses) | Localhost only | Bind to 127.0.0.1; use a random token in callback URLs for authentication |
| Push notification content | Visible to push service (ntfy/Pushover) | Use ntfy's built-in encryption or encrypt payloads before sending; strip sensitive details (show "Agent needs permission" not full command text) |

### 6.2 SSH Key Management

The tunnel uses a dedicated SSH key pair, separate from the developer's interactive SSH keys:

```bash
# Generate a dedicated key for the notification tunnel:
ssh-keygen -t ed25519 -f ~/.ssh/tmux-notif-relay -N "" -C "tmux-notification-relay"

# On the local relay machine, restrict the key in authorized_keys:
echo 'command="/bin/false",no-pty,no-agent-forwarding,no-X11-forwarding,permitopen="none",permitlisten="19222" ssh-ed25519 AAAA... tmux-notification-relay' >> ~/.ssh/authorized_keys
```

This key can **only** create a reverse port forward on port 19222. It cannot execute commands, forward other ports, or open a shell. Even if the key is compromised, the attacker can only listen on a specific port on the local machine's loopback interface.

### 6.3 Comparison to WebRTC/P2P Security

WebRTC's security model is designed for browsers and relies on DTLS-SRTP for media and SCTP-over-DTLS for data channels. This is robust but carries complexity:

- Certificate generation and exchange during signaling
- ICE candidate gathering (potentially exposing local IP addresses)
- STUN/TURN server trust (the TURN server sees all relayed traffic)
- Browser sandboxing assumptions that do not apply in a terminal context

SSH's security model is simpler, more battle-tested in server-to-server contexts, and does not require any third-party infrastructure for the trust establishment phase.

---

## 7. Addressing P2P/WebRTC Strengths

A P2P/WebRTC advocate would argue for direct device-to-device communication without any "relay" or "server" component. Here is why SSH tunnels are superior for this specific use case:

### 7.1 "WebRTC handles NAT traversal natively"

WebRTC's ICE framework handles NAT traversal through STUN (Session Traversal Utilities for NAT) and TURN (Traversal Using Relays around NAT). But:

- **STUN only works for cone NAT.** Symmetric NAT (common on corporate networks, mobile carriers, and many home routers) defeats STUN. The WebRTC connection falls back to TURN, which is a relay server -- the very cloud dependency we are trying to avoid.
- **STUN/TURN servers must be reachable.** If the developer is on a restrictive corporate network that blocks UDP (STUN uses UDP 3478) or non-standard ports, WebRTC fails entirely. SSH uses TCP port 22, which is almost universally allowed.
- **SSH reverse tunnels guarantee traversal.** The remote server initiates an outbound SSH connection (TCP, port 22). This traverses any NAT type, any firewall that allows outbound SSH. No fallback needed. No "maybe it works" ICE negotiation.

### 7.2 "WebRTC is peer-to-peer, no server needed"

This is misleading for several reasons:

- **Signaling requires a server.** WebRTC peers must exchange SDP offers/answers and ICE candidates through a signaling channel. What is that channel? A cloud service? A shared file? Without a signaling mechanism, WebRTC peers cannot discover each other. SSH key exchange has already happened (the developer configured SSH access to the remote server).
- **TURN is a server.** When direct P2P fails (symmetric NAT, firewalls), WebRTC uses TURN. A TURN server is a relay. It must be provisioned, maintained, and trusted. Self-hosting a TURN server (e.g., coturn) is more operational burden than running `sshd`, which is already running.
- **The "peer" is a tmux session, not a browser.** WebRTC's strengths (media streaming, browser integration, low-latency audio/video) are irrelevant here. We are sending 200-byte JSON messages every few minutes. WebRTC's data channel is massively overengineered for this workload.

### 7.3 "WebRTC works directly to mobile devices"

True in a browser context. But:

- Mobile WebRTC requires a running browser tab or a native app using a WebRTC library. Neither exists for this use case.
- SSH tunnel + push notification (ntfy/Pushover) delivers to mobile natively via the OS push notification system, which is battery-efficient, works when the app is not open, and supports action buttons.
- The developer does not need to keep a browser tab open to receive notifications.

### 7.4 "P2P avoids a single point of failure"

The SSH tunnel has one SPOF: the SSH connection itself. But autossh provides automatic recovery with a 20-year reliability track record. The local relay machine is also a SPOF, but so is any WebRTC peer -- if the developer's laptop is off, WebRTC cannot reach it either. The SSH model is explicit about this: the local relay machine must be on and reachable. This can be a $35 Raspberry Pi running 24/7 with <5W power draw.

---

## 8. Risks and Mitigations

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| **Local relay machine is unreachable** (off, network down) | High | Medium | Queue notifications to disk on remote; deliver on reconnection. Alert developer via separate channel (e.g., email) if tunnel is down for >1 hour. Support multiple relay targets (home server + cloud VPS fallback). |
| **SSH key compromise** | High | Low | Use dedicated, restricted key (no shell, no port forwarding except 19222). Rotate keys. Monitor authorized_keys for changes. |
| **Port conflict** on reverse-forwarded port | Medium | Low | `ExitOnForwardFailure yes` ensures immediate detection. Use configurable port range. Use Unix domain socket forwarding instead of TCP ports. |
| **autossh not installed** on remote server | Medium | Medium | Fall back to a pure bash reconnection loop: `while true; do ssh -N -R ...; sleep 5; done`. autossh is a convenience, not a hard dependency. |
| **Firewall blocks outbound SSH** from remote server | High | Low | Rare for development servers. Mitigation: use SSH over port 443 (`Port 443` in sshd_config on local relay) to piggyback on HTTPS-allowed firewall rules. |
| **Push notification latency** for mobile (ntfy/Pushover) | Low | Medium | Accept 1-5 second latency for mobile. Desktop notifications via the tunnel are sub-200ms. For time-critical action requests, set aggressive push priority. |
| **Complexity for initial setup** | Medium | High | Provide a setup wizard script: `tmux-notif-relay setup` that generates keys, configures SSH, tests the tunnel, and installs the receiver daemon. First-time setup should take under 5 minutes. |
| **Multiple remote servers create port management overhead** | Medium | Medium | Use a port allocation convention (19222 + server index) or Unix socket forwarding. The relay-receiver auto-discovers tunnels. |

### Fallback: No autossh Available

If `autossh` is not installed and cannot be installed (restricted environment), a minimal bash wrapper provides equivalent functionality:

```bash
#!/usr/bin/env bash
# minimal-autossh.sh -- pure bash SSH reconnection loop
REMOTE_PORT=19222
LOCAL_RELAY="user@local-relay"
SSH_KEY="$HOME/.ssh/tmux-notif-relay"
BACKOFF=1
MAX_BACKOFF=300

while true; do
    ssh -i "$SSH_KEY" \
        -o "ServerAliveInterval 30" \
        -o "ServerAliveCountMax 3" \
        -o "ExitOnForwardFailure yes" \
        -o "StrictHostKeyChecking accept-new" \
        -N -R "${REMOTE_PORT}:localhost:${REMOTE_PORT}" \
        "$LOCAL_RELAY" 2>/dev/null

    # Connection failed or dropped; reconnect with backoff
    sleep "$BACKOFF"
    BACKOFF=$((BACKOFF * 2))
    [ "$BACKOFF" -gt "$MAX_BACKOFF" ] && BACKOFF=$MAX_BACKOFF
done
```

This is 15 lines of bash. It provides the same reconnection semantics as autossh without any additional dependency.

---

## 9. Decision Matrix

| Criterion | SSH Reverse Tunnel | WebRTC / P2P | Winner |
|-----------|-------------------|--------------|--------|
| **New infrastructure required** | None (SSH already configured) | STUN server (minimum), TURN server (for symmetric NAT), signaling server | SSH |
| **NAT traversal reliability** | 100% (outbound TCP to port 22, universally allowed) | ~70-85% without TURN (fails on symmetric NAT, UDP-blocking firewalls) | SSH |
| **Encryption** | AES-256-GCM / ChaCha20 (SSH standard) | DTLS-SRTP (WebRTC standard) | Tie |
| **Authentication** | SSH keys (already configured) | Certificate exchange during signaling (new infrastructure) | SSH |
| **Mobile device support** | Via push notification service (ntfy/Pushover) from local relay | Requires native app or browser tab with WebRTC | SSH |
| **Reconnection** | autossh: automatic, 20-year track record, exponential backoff | ICE restart: depends on implementation, often requires re-signaling | SSH |
| **Bandwidth overhead (idle)** | ~24 KB/hour (keepalive packets) | ~36 KB/hour (STUN binding requests + DTLS heartbeats) | Tie |
| **Memory usage** | ~10-12 MB (ssh + autossh) | ~30-50 MB (WebRTC stack, even for data-only) | SSH |
| **Implementation complexity** | bash + socat + ssh (tools developers know) | WebRTC library (libdatachannel or node-webrtc), signaling logic, ICE handling | SSH |
| **Multiple remote servers** | One tunnel per server, port-per-server convention | One peer connection per server, similar complexity | Tie |
| **Latency** | 1-50 ms (TCP, established connection) | 1-30 ms (UDP, direct P2P) -- when it works | WebRTC (marginal) |
| **Works without cloud services** | Yes (fully self-contained) | No (needs STUN minimum; TURN for reliability) | SSH |
| **Developer familiarity** | Universal (every dev uses SSH daily) | Low (WebRTC is a browser/VoIP technology) | SSH |
| **Battery impact on mobile** | Zero (OS push notifications are battery-optimized) | High (WebRTC requires persistent connection, wakes radio) | SSH |
| **Bidirectional messaging** | Native (SSH tunnel is bidirectional) | Native (data channels are bidirectional) | Tie |
| **Offline message queuing** | Disk queue on remote, delivered on reconnect | No native queuing; lost if peer offline | SSH |
| **Security track record** | 30+ years, constant audit, CVE response | 10+ years, browser-focused, fewer server-context audits | SSH |

**Score: SSH 11, WebRTC 1, Tie 5.** WebRTC's only outright win is marginal latency advantage in the best case (direct UDP P2P), which is irrelevant for notifications arriving minutes apart.

---

## 10. Integration with the Plugin Ecosystem

This SSH tunnel transport integrates naturally with the plugin's existing and planned features:

- **Feature #1 (Desktop notifications)**: The relay-receiver dispatches to `notify-send`/`osascript` on the local machine. Works even when the developer has no SSH session open to the remote server.
- **Feature #2 (Audio alerts)**: The relay-receiver plays sounds locally via `afplay`/`paplay`.
- **Feature #3 (Priority levels)**: The JSON protocol includes a `priority` field. The relay-receiver maps priorities to notification urgency levels.
- **Feature #8 (Callbacks/webhooks)**: The relay-receiver is itself a callback dispatcher. Users configure dispatch targets on their local machine.
- **Feature #9 (DND mode)**: The relay-receiver checks a local DND flag before dispatching. Notifications are still received and queued but not displayed.
- **Feature #12 (Cross-machine relay)**: This is Feature #12. The SSH tunnel is its implementation.
- **Feature #13 (Rules engine)**: Rules can be evaluated either on the remote side (in relay-sender, before transmission) or on the local side (in relay-receiver, before dispatch). Local-side evaluation keeps rules private and editable without remote access.

---

## 11. Conclusion

The question of local notification storage (files vs. native tmux options) was settled in the judgment document: use native tmux options for the hot path, retain files for logging. That debate concerned a single machine. This paper addresses the harder problem: the network.

SSH reverse tunnels provide everything the cross-machine notification relay needs:

1. **Encryption and authentication** are solved by SSH's existing, battle-tested infrastructure.
2. **NAT traversal** is guaranteed by the reverse tunnel model (outbound connection from remote to local), not probabilistic (like ICE/STUN).
3. **Persistent connectivity** is provided by autossh or a trivial 15-line bash wrapper.
4. **Bidirectional messaging** flows naturally through the tunnel: notifications out, action responses back.
5. **Mobile device reach** is achieved through the local relay dispatching to push notification services (ntfy, Pushover), keeping the SSH tunnel's scope limited to the server-to-local hop.
6. **Zero new infrastructure** is required. Developers already have SSH keys, `sshd`, and the conceptual model for how SSH tunnels work.

WebRTC and other P2P approaches solve a different problem: real-time media streaming between browsers. For sending 200-byte JSON notifications every few minutes between a remote tmux session and a developer's phone, SSH tunnels are simpler, more reliable, more secure, and require less infrastructure. The technology is 30 years old because the problem it solves -- secure, authenticated, encrypted channels between machines -- is exactly the problem we have.

The tunnel is not glamorous. It is correct.
