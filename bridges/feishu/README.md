# Aido 飞书 Bridge

将飞书 / Lark 群聊与私聊对接到 Aido，用户在场内发消息由本 bridge 转发给 Aido，AI 回复通过本 bridge 发回飞书。

**本 bridge 使用飞书「长连接（WebSocket）直连」接收事件，无需配置回调地址、无需公网 IP 或内网穿透。**

## 开发前准备（飞书侧）

使用本 bridge 前，需在飞书开放平台完成应用创建与权限配置。官方步骤见：

**[飞书 Node.js SDK - 开发前准备](https://open.feishu.cn/document/server-side-sdk/nodejs-sdk/preparation-before-development)**

简要对应关系：

- **创建应用**：在 [飞书开放平台](https://open.feishu.cn/app) 创建**企业自建应用**，在「凭证与基础信息」中获取 **App ID**、**App Secret**（对应 `.env` 的 `FEISHU_APP_ID`、`FEISHU_APP_SECRET`）。
- **权限**：在「权限管理」中申请并启用「接收消息」「发送消息」等所需权限（如 `im:message`、`im:message.group_at_msg` 等，按机器人能力需求勾选）。
- **事件订阅**：在「事件与回调」中订阅 `im.message.receive_v1`，订阅方式选择 **「使用长连接接收事件」**（无需填回调 URL；保存时本 bridge 需已启动并连上飞书，否则可能无法生效）。

## 前置条件（本 bridge）

- 已启动 Aido 网关，并拿到可用的 `token`。
- 已完成上述飞书侧开发前准备（应用、权限、长连接事件订阅）。

## 配置

复制 `.env.example` 为 `.env` 并填写：

| 变量 | 说明 |
|------|------|
| `AIDO_WS_URL` | Aido WebSocket 地址，如 `ws://localhost:8080/ws` |
| `AIDO_TOKEN` | 网关认证 token |
| `FEISHU_APP_ID` | 飞书应用 App ID |
| `FEISHU_APP_SECRET` | 飞书应用 App Secret |
| `FEISHU_DOMAIN` | 可选，海外 Lark 填 `lark`，国内飞书默认 `feishu` |

## 运行

```bash
cd bridges/feishu
npm install    # 或 pnpm install
npm run dev    # 开发：tsx src/index.ts
# 或
npm run build && npm start
```

启动后：

1. 使用飞书 SDK 的 **WebSocket 长连接**连到飞书，直接接收 `im.message.receive_v1` 等事件，无需 HTTP 回调。
2. 连接 Aido WebSocket（`role: bridge`, `channel: feishu`）。
3. 收到飞书消息 → 调用 Aido `message.send`；收到 Aido `outbound.message` → 用飞书 Open API 发回对应会话。

## 会话约定

- **channel** 固定为 `feishu`。
- **channelChatId** 使用飞书的 `chat_id`（群聊为群 ID，私聊为单聊会话 ID），回复时按此 ID 发回对应会话。

## 长连接模式说明

飞书支持两种事件订阅方式：

- **Webhook**：需配置公网回调 URL、验证 Token、可选加密，适合生产固定部署。
- **长连接（本 bridge 使用）**：本机作为客户端连到飞书，无需回调地址、无需公网、无需验证/解密逻辑，适合本地开发与内网部署。每个应用最多 50 个连接。详见 [飞书文档 - 使用长连接接收事件](https://feishu.apifox.cn/doc-7518469)。

## 后续可扩展

- 富文本 / 卡片回复、图片与文件收发（见 [API 附录：附件](../api/README.md#附录附件)）。
- 群内仅 @ 机器人时回复、白名单等策略（可参考 [clawdbot-feishu](https://github.com/m1heng/clawdbot-feishu) 的 policy 与 reply 逻辑）。
