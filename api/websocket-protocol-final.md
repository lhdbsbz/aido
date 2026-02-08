# Aido WebSocket 协议 · 最终版（审阅用）

## 一、设计前提

- **AI 私人助理**：单用户（主人），协议不暴露 userId。
- **多端多会话**：Web、飞书、Telegram 为**不同对话**，各端有各自的历史；不要求「第一句在飞书、第二句在 Telegram」同属一段。
- **会话身份**：用 **(channel, channelChatId)** 标识一段对话；协议统一用这两者，服务端内部用同一对做存储 key（见下），**不掺入 agentId**，以便换模型不丢历史。
- **agent 配置**：用哪套模型/工具由**网关全局配置**决定（config 或管理页），协议不传 agentId；换模型不丢历史。
- **一套协议**：网页、飞书、Telegram、Bridge/Client 均用同一套连接、发消息、收事件。

---

## 二、连接

- **URL**：`ws://<host>:<port>/ws`
- **首帧**：必须是 `connect`（type=req, method=connect, params=…）

### Connect 参数（单一约定，无可选）

**Bridge** 连接时传：

| 字段 | 说明 |
|------|------|
| `role` | 固定 `"bridge"` |
| `token` | 网关认证 token |
| `channel` | 订阅的渠道（如 `telegram`、`feishu`） |
| `capabilities` | 数组，如 `["text","media"]` |

**Client** 连接时传：

| 字段 | 说明 |
|------|------|
| `role` | 固定 `"client"` |
| `token` | 网关认证 token |

Client 不传 channel、channelChatId。服务端向所有 Client 推送所有会话的 `agent`、`user_message` 事件；客户端根据 payload 里的 `channel`、`channelChatId` 自行过滤出当前关注的对话。

**响应**（成功）：`{ type: "res", id, ok: true, payload: { connId, protocol: 1 } }`

---

## 三、发消息：唯一方法 `message.send`

- **语义**：向对话 (channel, channelChatId) 投递一条用户消息并触发 Agent；回复按 channel + channelChatId 路由回对应端。
- **为何 message.send 还要带 channel**：Connect 时的 channel 表示「这条连接订阅哪个渠道」（用于收事件），不是「这条消息属于哪段对话」。每条消息必须自带 (channel, channelChatId) 才能唯一定位会话；且 Client 在 Connect 本就不传 channel，发消息时必须传。为统一规则，**Bridge 与 Client 的 message.send 一律必传 channel + channelChatId**，不依赖连接状态。
- **不使用 agentId**：使用哪个 agent（模型/工具）由**网关全局配置**决定，客户端不传、不决定；message.send 不包含 agentId。

### channelChatId 是谁产生的

| 渠道 | 谁产生 channelChatId | 说明 |
|------|---------------------|------|
| **Telegram / 飞书 / 其他平台** | **平台** | 平台在用户与 bot 对话时分配会话 id（如 Telegram 的 `chat_id`、飞书的 `open_chat_id`）。Bridge 从平台拿到后，在 `message.send` 里传给网关；网关在 `outbound.message` 里原样带回，Bridge 据此把回复发回平台对应会话。 |
| **Web (webchat)** | **我们（前端或网关）** | 无第三方平台，由前端生成并持久化（如 deviceId、随机 UUID 存 localStorage），发消息时带上；网关只透传与存储，回复时按此 id 推回给对应 Client。 |

### 参数

| 字段 | 必填 | 说明 |
|------|------|------|
| `channel` | ✅ | 渠道：`webchat`、`telegram`、`feishu` 等 |
| `channelChatId` | ✅ | 该渠道下的会话 id；来源见上表（平台分配 或 前端生成） |
| `text` | ✅ | 消息正文 |
| `senderId` | 否 | 发送方展示 id（如平台用户 id） |
| `messageId` | 否 | 去重/引用 |
| `attachments` | 否 | `[{ type, url?, base64?, mime? }]` |

### 请求示例

```json
{
  "type": "req",
  "id": "1",
  "method": "message.send",
  "params": {
    "channel": "webchat",
    "channelChatId": "device-abc",
    "text": "今天有什么重要财经新闻？"
  }
}
```

### 响应（成功）

```json
{
  "type": "res",
  "id": "1",
  "ok": true,
  "payload": { "text": "…", "toolSteps": [] }
}
```

- **移除的旧方法**：`inbound.message`、`chat.send`，统一为 `message.send`。

---

## 四、事件（服务端推送）

所有事件 payload 均带 **channel**、**channelChatId**，用于标识「属于哪段对话」和「回复发到哪」。

### 4.1 agent

- **含义**：Agent 运行过程（流式、工具调用、完成等）。
- **Payload 必含**：`type`、`runId`、`seq`；按事件类型另有 `text`、`toolName`、`toolParams`、`toolResult`、`error` 等。
- **Payload 含**：`channel`、`channelChatId`（本次请求的对话标识）。
- **投递**：
  - 发给该 **channel** 的所有 Bridge（便于做「正在输入」、流式回传）；
  - 发给所有 Client（由前端按 payload 的 channel/channelChatId 过滤）。

### 4.2 outbound.message

- **含义**：Agent 的最终回复，需发回用户所在端。
- **Payload**：`channel`、`channelChatId`、`text`。
- **投递**：仅发给该 **channel** 的 Bridge；Bridge 根据 channelChatId 发到对应会话。

### 4.3 user_message

- **含义**：用户刚发的消息回显（供 UI 展示「自己说了什么」）。
- **Payload**：`channel`、`channelChatId`、`text`。
- **投递**：发给所有 Client，由前端按 payload 的 channel/channelChatId 过滤。

---

## 五、其他方法

| 方法 | 说明 | 参数 | 返回 |
|------|------|------|------|
| `chat.history` | 某段对话的历史 | `channel`（必）、`channelChatId`（必） | `{ messages: [...] }` |
| `sessions.list` | 当前所有会话列表（多端多会话） | 无 | `{ sessions: [ { channel, channelChatId, updatedAt, inputTokens, outputTokens, ... } ] }` |
| `config.get` | 网关配置（脱敏） | 无 | 配置对象 |
| `health` | 健康与连接数 | 无 | `{ status, bridges, clients }` |

- 会话列表与历史均以 **(channel, channelChatId)** 为标识，不暴露内部 sessionKey。

---

## 六、角色与订阅小结

| 角色 | Connect 传参 | 收到的事件 |
|------|--------------|------------|
| **Bridge** | role, token, channel, capabilities | 该 channel 的 `outbound.message`、`agent`（payload 含 channelChatId 便于区分多会话） |
| **Client** | role, token | 所有会话的 `agent`、`user_message`（前端按 payload 的 channel/channelChatId 过滤） |

---

## 七、会话与存储（实现约定）

- **协议层**：仅使用 **(channel, channelChatId)** 表示「哪段对话」；不暴露 sessionKey、userId。
- **服务端内部**：存储与锁的 key 只含 **(channel, channelChatId)**，例如 `sessionKey = channel + ":" + channelChatId`（如 `webchat:device-abc`）。**不掺入 agentId**，这样换网关全局 agent 配置时仍是同一段对话、同一份历史；agent 配置只影响「用哪套模型/工具跑」，不影响存储 key。
- **单用户**：不存 userId；多端多会话仅由 channel + channelChatId 区分。

---

## 八、最终形态一句话

- **连接**：connect；Bridge 带 channel+capabilities，Client 只带 role+token；Client 收全量事件后按 channel/channelChatId 过滤。
- **发消息**：唯一方法 message.send(channel, channelChatId, text, …)。
- **会话**：多端多会话，以 (channel, channelChatId) 为标识；历史、事件、列表均围绕此二者。
- **事件**：agent / outbound.message / user_message，payload 带 channel、channelChatId；Bridge 按 channel 收，Client 按订阅或全量+前端过滤。

若审阅无异议，可按此最终版落地实现与文档更新。
