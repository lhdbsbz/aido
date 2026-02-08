# Aido WebSocket Protocol

## Design

- **Single user** (no userId in protocol).
- **Multiple conversations per channel**: Web, Feishu, Telegram each have their own history; identified by `(channel, channelChatId)`.
- **Agent** is determined by gateway config only; clients do not send or choose agentId.
- **One protocol** for all: Bridge and Client use the same `message.send`; only Connect params differ by role.

## Connection

- **URL**: `ws://<host>:<port>/ws`
- **First frame**: must be `connect` (type=req, method=connect, params=…)

### Connect params (single convention)

**Bridge** sends:

| Field | Description |
|-------|-------------|
| `role` | `"bridge"` |
| `token` | Gateway auth token |
| `channel` | Channel to subscribe (e.g. `telegram`, `feishu`) |
| `capabilities` | Array, e.g. `["text","media"]` |

**Client** sends:

| Field | Description |
|-------|-------------|
| `role` | `"client"` |
| `token` | Gateway auth token |

Client does not send channel or channelChatId. Server pushes all `agent` and `user_message` events to all clients; client filters by payload `channel` and `channelChatId`.

**Success response**: `{ "type": "res", "id": "<id>", "ok": true, "payload": { "connId": "<id>", "protocol": 1 } }`

## Sending messages: `message.send`

Both Bridge and Client use this method.

**Params**:

| Field | Required | Description |
|-------|----------|-------------|
| `channel` | yes | e.g. `webchat`, `telegram`, `feishu` |
| `channelChatId` | yes | Conversation id on that channel (platform-assigned or client-generated for webchat) |
| `text` | yes | Message body |
| `senderId` | no | Sender display id |
| `messageId` | no | Dedup / reference |
| `attachments` | no | `[{ "type", "url"?, "base64"?, "mime"? }]` |

**Example**:

```json
{
  "type": "req",
  "id": "1",
  "method": "message.send",
  "params": {
    "channel": "webchat",
    "channelChatId": "device-abc",
    "text": "Hello"
  }
}
```

**Success response**: `{ "type": "res", "id": "1", "ok": true, "payload": { "text": "…", "toolSteps": [] } }`

### channelChatId source

| Channel | Produced by |
|---------|-------------|
| Telegram / Feishu / other platforms | **Platform** (e.g. Telegram `chat_id`). Bridge passes it in `message.send`; server returns it in `outbound.message` so Bridge can route the reply. |
| Web (webchat) | **Client** (e.g. deviceId or UUID in localStorage). |

## Events (server push)

All event payloads include `channel` and `channelChatId` where relevant.

| Event | Payload | Recipients |
|-------|---------|------------|
| **agent** | `type`, `runId`, `seq`, `channel`, `channelChatId`, plus type-specific fields (`text`, `toolName`, …) | All clients; all bridges for that `channel` |
| **outbound.message** | `channel`, `channelChatId`, `text` | Bridges for that `channel` only |
| **user_message** | `channel`, `channelChatId`, `text` | All clients (filter by channel/channelChatId on client) |

## Other methods (client only)

| Method | Params | Description |
|--------|--------|-------------|
| `chat.history` | `channel`, `channelChatId` (required) | Get messages for that conversation |
| `sessions.list` | none | List all conversations; each item has `channel`, `channelChatId`, `updatedAt`, token counts, etc. |
| `config.get` | none | Gateway config (sanitized) |
| `health` | none | Status, bridge/client counts |

## HTTP API

| Endpoint | Description |
|----------|-------------|
| `GET /` | Management UI |
| `GET /static/*` | Static assets |
| `GET /health` | Health check (JSON) |
| `GET /api/chat/history?channel=…&channelChatId=…` | Chat history |
| `POST /api/chat/send` | Body: `{ "channel", "channelChatId", "text", "attachments"? }` |
| `GET /api/sessions` | Sessions list |
| `POST /v1/chat/completions` | OpenAI-compatible chat API |

## Internal session key

Server uses `sessionKey = channel + ":" + channelChatId` for storage and locking (no agentId in key; changing agent config does not create a new session).
