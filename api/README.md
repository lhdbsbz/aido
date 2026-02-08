# Aido 接入指南

本文档面向**希望接入 Aido 的开发者**，按场景说明如何快速对接：发消息、收回复、拉历史、传图片等。  
网关地址记为 `http(s)://<host>:<port>`（HTTP）或 `ws(s)://<host>:<port>`（WebSocket），认证 token 由部署方提供。

---

## 一、先搞清楚你要做哪种接入

| 场景 | 推荐方式 | 说明 |
|------|----------|------|
| **网页/App 聊天界面** | WebSocket（Client 角色）+ 可选 HTTP | 建一条长连接，发 `message.send`，收 `agent` 流式事件和 `outbound.message` 最终回复；也可用 HTTP 发消息、再轮询或另接 WebSocket 收事件。 |
| **对接 Telegram / 飞书等平台（Bridge）** | WebSocket（Bridge 角色） | 建一条按渠道订阅的长连接，收到用户消息后转成 `message.send` 发给网关，监听 `outbound.message` 把回复发回平台。 |
| **只发一条消息、拿一条回复（无状态）** | HTTP `POST /api/chat/send` | 请求体里带 channel、channelChatId、text（和可选 attachments），响应里直接返回 AI 回复。 |
| **用 OpenAI 官方 SDK 或兼容 OpenAI 的客户端** | HTTP `POST /v1/chat/completions` | 与 OpenAI 的 chat 接口兼容，支持流式与非流式；可传图片（content 数组里带 `image_url`）。 |

下面按场景写具体步骤。

---

## 二、场景 1：网页/App 聊天（WebSocket Client）

目标：你的前端建一条 WebSocket，能发用户消息、收流式回复和最终回复。

### 2.1 建立连接

- **URL**：`ws://<host>:<port>/ws`
- **首帧必须发**：连接成功后，先发一帧 **connect**，否则服务端会断开。

**Connect 请求（Client 角色）**：

```json
{
  "type": "req",
  "id": "connect-1",
  "method": "connect",
  "params": {
    "role": "client",
    "token": "<你的 token>"
  }
}
```

**成功响应**：

```json
{
  "type": "res",
  "id": "connect-1",
  "ok": true,
  "payload": { "connId": "<连接 id>", "protocol": 1 }
}
```

之后所有请求都带 `type: "req"`、`id`、`method`、`params`。响应为 `type: "res"`、`id`、`ok`；成功时带 `payload`（JSON 对象），失败时带 `error`（含 `code`、`message`）。

### 2.2 发一条消息（触发 AI 回复）

用**同一个连接**发 `message.send`，指定这段对话的「渠道」和「会话 id」：

```json
{
  "type": "req",
  "id": "msg-1",
  "method": "message.send",
  "params": {
    "channel": "webchat",
    "channelChatId": "device-abc",
    "text": "今天有什么重要财经新闻？"
  }
}
```

- **channel**：网页端一般用 `webchat`。
- **channelChatId**：由你生成并持久化（例如设备 id、或 localStorage 里的 UUID），同一用户同一设备建议固定，这样历史会连在一起。
- **text**：用户输入；也可以只发附件（见 [附录：附件](#附录附件)）。

**成功响应**：

```json
{
  "type": "res",
  "id": "msg-1",
  "ok": true,
  "payload": { "text": "AI 的完整回复…", "toolSteps": [] }
}
```

同时，服务端会通过 **事件** 推送流式内容（见下）。

### 2.3 收事件（流式 + 最终回复）

服务端会主动推 `type: "event"` 的帧，`event` 字段表示事件名，`payload` 里带 `channel`、`channelChatId`，便于你过滤到当前对话。

| 事件 | 何时收到 | 你用 payload 做什么 |
|------|----------|----------------------|
| **user_message** | 用户消息已接受 | 在 UI 里展示「用户刚发了什么」（channel、channelChatId、text） |
| **agent** | Agent 运行过程 | 流式：`payload.type` 为 `text_delta` 时用 `payload.text` 拼成回复；工具调用时见 `toolName`、`toolParams`、`toolResult`；结束时 `type` 为 `done`。其他类型还有 `stream_start`、`tool_start`、`tool_end`、`assistant`、`error` 等。 |
| **outbound.message** | Agent 最终回复已就绪 | **仅订阅了该 channel 的 Bridge 会收到**；Client 不会收到。Client 用 **message.send 的 res.payload** 或 **agent 流式拼出来的结果** 即可。 |

**示例（agent 流式一段文字）**：

```json
{
  "type": "event",
  "event": "agent",
  "seq": 2,
  "payload": {
    "type": "text_delta",
    "runId": "run_xxx",
    "seq": 2,
    "channel": "webchat",
    "channelChatId": "device-abc",
    "text": "根据"
  }
}
```

前端可按 `payload.channel`、`payload.channelChatId` 过滤到当前会话，再根据 `payload.type` 更新 UI（流式文字 / 工具调用 / 完成）。

### 2.4 拉历史、会话列表

以下方法**仅 Client 角色**可调用（Bridge 连接调用会报错）。

- **某段对话的历史**：`method: "chat.history"`，params 里 `channel`、`channelChatId` 必填；返回 `{ "messages": [ { "role", "content", "toolCalls"? } ] }`。
- **所有会话列表**：`method: "sessions.list"`，params 可为 `{}` 或不传；返回 `{ "sessions": [ { "channel", "channelChatId", "createdAt", "updatedAt", "inputTokens", "outputTokens", "compactions" } ] }`。
- **健康**：`method: "health"`；**配置（脱敏）**：`method: "config.get"`。

会话唯一标识就是 **(channel, channelChatId)**，没有 sessionKey 等内部概念暴露给你。

---

## 三、场景 2：对接 Telegram / 飞书等（WebSocket Bridge）

目标：你的服务从平台收到用户消息后，转给 Aido；再把 Aido 的回复发回平台。

### 3.1 建立连接（Bridge 角色）

- **URL**：`ws://<host>:<port>/ws`
- **首帧**：connect，且必须带**订阅的渠道**：

```json
{
  "type": "req",
  "id": "connect-1",
  "method": "connect",
  "params": {
    "role": "bridge",
    "token": "<token>",
    "channel": "telegram",
    "capabilities": ["text", "media"]
  }
}
```

- **channel**：你这条连接负责的渠道（如 `telegram`、`feishu`）。服务端只会把该渠道的 `outbound.message` 推给这条连接。
- **capabilities**：可选，如 `["text","media"]`。

### 3.2 把平台消息转成 message.send

平台（如 Telegram）每次发来一条用户消息时，你拼成一条 `message.send`：

```json
{
  "type": "req",
  "id": "msg-1",
  "method": "message.send",
  "params": {
    "channel": "telegram",
    "channelChatId": "123456789",
    "text": "用户发的文字",
    "senderId": "tg-user-id",
    "messageId": "platform-msg-id",
    "attachments": []
  }
}
```

- **channelChatId**：**平台给的会话 id**（如 Telegram 的 `chat_id`、飞书的 `open_chat_id`），不要自己发明。回复会按这个 id 通过 `outbound.message` 带回，你再根据它发回对应会话。
- 若有图片/语音等，见 [附录：附件](#附录附件)，往 `attachments` 里塞。

### 3.3 收 AI 回复并发回平台

监听事件里的 **outbound.message**：

```json
{
  "type": "event",
  "event": "outbound.message",
  "seq": 5,
  "payload": {
    "channel": "telegram",
    "channelChatId": "123456789",
    "text": "AI 的完整回复"
  }
}
```

用 `channel` + `channelChatId` 对应到平台会话，把 `text` 发回平台即可。  
流式展示（如「正在输入」）可用 **agent** 事件的 `text_delta` 等，但最终以 **outbound.message** 为准。

---

## 四、场景 3：只用 HTTP 发消息、拿回复（无 WebSocket）

不建长连接，只调 HTTP 接口。

### 4.1 发一条消息并拿到回复

**请求**：

- **Method**：`POST /api/chat/send`
- **认证**：Header `Authorization: Bearer <token>` 或 Query `token=...`（与 WebSocket 使用同一 token）
- **Body（JSON）**：

```json
{
  "channel": "webchat",
  "channelChatId": "device-abc",
  "text": "你好",
  "attachments": []
}
```

**响应**：JSON，如 `{ "text": "AI 的完整回复", "toolSteps": [] }`。

- 若需传图/文件，在 `attachments` 里按 [附录：附件](#附录附件) 格式传。

### 4.2 其他常用 HTTP 接口

| 用途 | 方法 | 说明 |
|------|------|------|
| 健康检查（无需认证） | `GET /health` | 返回 `{ "status": "ok", "uptime": "...", "bridges": <数量>, "clients": <数量> }` |
| 健康检查（需认证） | `GET /api/health` | 返回 `{ "status": "ok", "bridges": [ {...} ], "clients": <数量> }`，bridges 为连接详情数组 |
| 某段对话历史（需认证） | `GET /api/chat/history?channel=…&channelChatId=…` | 返回 `{ "messages": [ { "role", "content", "toolCalls"? } ] }` |
| 会话列表（需认证） | `GET /api/sessions` | 返回 `{ "sessions": [ { "channel", "channelChatId", "createdAt", "updatedAt", "inputTokens", "outputTokens", "compactions" } ] }` |
| 管理页 | `GET /` | 浏览器打开网关管理界面 |

---

## 五、场景 4：用 OpenAI SDK 或兼容 OpenAI 的客户端

若你已有 OpenAI 的 chat 代码（或兼容 OpenAI 的客户端），可直接对接到 Aido 的兼容接口。

### 5.1 请求格式

- **URL**：`POST /v1/chat/completions`
- **Header**：`Authorization: Bearer <token>`
- **Body**：与 OpenAI 一致，例如：

```json
{
  "model": "default",
  "messages": [
    { "role": "user", "content": "你好" }
  ],
  "stream": false,
  "user": "optional-session-id"
}
```

- **model**：可填网关配置的 agent 名（如 `default`），或由网关忽略而用默认 agent。
- **user**：可选，用作会话标识；不传则每次可能新会话。
- **stream**：`true` 为 SSE 流式返回，与 OpenAI 一致。

### 5.2 带图片的请求

支持 `messages[].content` 为**数组**，其中可带 `type: "image_url"`：

```json
{
  "model": "default",
  "messages": [
    {
      "role": "user",
      "content": [
        { "type": "text", "text": "这张图里有什么？" },
        { "type": "image_url", "image_url": { "url": "https://…" } }
      ]
    }
  ],
  "stream": false
}
```

`url` 可以是普通 URL，也可以是 `data:image/png;base64,...` 形式。限制（数量、大小）与 WebSocket/HTTP 的附件一致，见 [附录：附件](#附录附件)。

---

## 附录：附件

所有发消息的入口（WebSocket `message.send`、HTTP `/api/chat/send`、OpenAI `/v1/chat/completions`）都支持**文本 + 附件**。附件会原样传给模型（在模型支持的前提下）。

### 格式（WebSocket / HTTP）

在 `params` 或 body 里带 `attachments` 数组，每项：

| 字段 | 必填 | 说明 |
|------|------|------|
| type | 是 | `image` \| `audio` \| `video` \| `file` |
| url | 与 base64 二选一 | 可访问的 URL（内网或白名单） |
| base64 | 与 url 二选一 | 内联数据（需配合 mime） |
| mime | base64 时建议 | 如 `image/png`、`audio/mpeg` |

- 当前默认限制：单条消息最多 20 个附件；base64 单附件解码后不超过 15MB；`type` 仅允许 `image`、`audio`、`video`、`file`。超限会返回错误。
- 安全：仅会拉取允许的 URL；原始二进制不会写日志。

### 示例（带一张图）

```json
{
  "channel": "webchat",
  "channelChatId": "device-abc",
  "text": "描述一下这张图",
  "attachments": [
    { "type": "image", "url": "https://example.com/photo.jpg" }
  ]
}
```

或内联 base64：

```json
{
  "attachments": [
    { "type": "image", "base64": "<base64 字符串>", "mime": "image/png" }
  ]
}
```

---

## 附录：认证

- **WebSocket**：在 connect 的 `params` 里带 `token`（由部署方提供）。
- **HTTP**：常见为 Header `Authorization: Bearer <token>`，具体以部署说明为准。

---

## 附录：会话与多端

- 一段对话由 **(channel, channelChatId)** 唯一标识；不暴露 sessionKey、userId。
- 网页端：同一设备建议固定一个 `channelChatId`（如存 localStorage），这样历史连续。
- Bridge：`channelChatId` 必须用平台给的会话 id，以便回复能准确发回对应会话。
- 使用哪套模型/工具由**网关配置**决定，协议里不传 agentId。

若你遇到的具体问题（如某个字段含义、错误码）未覆盖，可结合运行时的报错或部署方提供的补充说明排查；本文档侧重「按场景快速接入」的步骤与约定。
