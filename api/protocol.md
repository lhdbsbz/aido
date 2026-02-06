# Aido WebSocket Protocol

## Connection

Connect to `ws://<host>:18789/ws`

### Handshake

First message must be a `connect` request:

```json
{
  "type": "req",
  "id": "1",
  "method": "connect",
  "params": {
    "role": "bridge",
    "token": "your-gateway-token",
    "channel": "telegram",
    "capabilities": ["text", "media", "reactions"]
  }
}
```

Response:
```json
{
  "type": "res",
  "id": "1",
  "ok": true,
  "payload": { "connId": "conn_123", "protocol": 1 }
}
```

## Roles

### Bridge Role
For chat platform adapters (Telegram, Discord, etc.)

**Send user messages:**
```json
{
  "type": "req",
  "id": "2",
  "method": "inbound.message",
  "params": {
    "channel": "telegram",
    "chatId": "12345",
    "senderId": "user1",
    "text": "Hello!",
    "messageId": "msg_abc"
  }
}
```

**Receive agent replies (event):**
```json
{
  "type": "event",
  "event": "outbound.message",
  "seq": 1,
  "payload": { "chatId": "12345", "text": "Hello! How can I help?" }
}
```

**Receive typing indicators (event):**
```json
{
  "type": "event",
  "event": "agent",
  "seq": 2,
  "payload": { "type": "stream_start", "sessionKey": "default:telegram:12345" }
}
```

### Client Role
For management UIs (uni-app, Web, CLI)

**Send direct messages:**
```json
{
  "type": "req",
  "id": "3",
  "method": "chat.send",
  "params": { "text": "What's the weather?" }
}
```

**List sessions:**
```json
{ "type": "req", "id": "4", "method": "sessions.list" }
```

**Get health:**
```json
{ "type": "req", "id": "5", "method": "health" }
```

## Available Methods

| Method | Role | Description |
|--------|------|-------------|
| `connect` | both | Handshake and authenticate |
| `inbound.message` | bridge | Push user message to agent |
| `chat.send` | client | Send message directly to agent |
| `chat.history` | client | Get conversation history |
| `sessions.list` | client | List all sessions |
| `config.get` | client | Get current config (secrets redacted) |
| `health` | client | Get gateway health status |

## Events

| Event | Recipients | Description |
|-------|-----------|-------------|
| `agent` | all | Agent lifecycle events (stream, tool calls, done) |
| `outbound.message` | bridges | Agent response to send back to user |

## HTTP Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Built-in management UI (config, chat test, sessions, health) |
| `GET /static/*` | Static assets for the UI |
| `GET /health` | Health check (JSON) |
| `POST /v1/chat/completions` | OpenAI-compatible chat API |
