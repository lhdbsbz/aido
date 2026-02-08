# Aido 官方 Bridge

本目录存放 Aido 官方维护的桥接器，用于将各类聊天平台（飞书、Telegram 等）对接到 Aido 网关。  
每个 bridge 为独立进程，通过 WebSocket 连接 Aido，按 [API 文档](../api/README.md) 中的「场景 2：WebSocket Bridge」收发消息。

| Bridge   | 语言        | 说明           |
|----------|-------------|----------------|
| [feishu](./feishu) | TypeScript | 飞书 / Lark 桥接器 |

## 自动加载

Aido 主程序可按配置**自动发现并拉起**本目录下的 bridge，无需手动起进程。约定见 [SPEC.md](./SPEC.md)：

- 每个 bridge 目录内需有 **`aido-bridge.json`** 清单（id、runtime、command、envSchema 等）。
- 在 `config.yaml` 的 **`bridges.instances`** 中增加一项：`id`、`enabled`、`path`（相对配置目录或绝对路径）、可选 `env`。
- 主程序启动时解析 path、读取清单、注入 `AIDO_WS_URL` 与 `AIDO_TOKEN`，并执行 `command` 拉起进程。
- 管理端「配置」页有 **Bridges** 区块，可增删改实例；**GET /api/bridges** 可查看各实例及运行状态（running、pid、startedAt）。

运行任一 bridge 前需先启动 Aido 网关；若使用自动加载，在配置中启用对应实例并填好平台凭证即可。
