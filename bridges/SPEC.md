# Aido Bridge 标准

本文档定义「可被 Aido 主程序自动发现、配置并拉起的桥接器」的约定。符合该标准的 bridge 可被 Aido 根据配置自动启动，并在管理端配置与前端中维护。

## 1. 目录与清单文件

- 每个 bridge 占一个**独立目录**（如 `bridges/feishu`）。
- 目录内必须包含清单文件 **`aido-bridge.json`**，用于声明运行方式与环境需求，便于 Aido 与 AI 自动配置。

## 2. 清单格式 `aido-bridge.json`

```json
{
  "id": "feishu",
  "name": "飞书 / Lark",
  "description": "将飞书/Lark 消息对接到 Aido，使用长连接接收事件。",
  "runtime": "node",
  "commands": [
    ["npm", "install"],
    ["npm", "run", "build"],
    ["npm", "run", "start"]
  ],
  "cwd": ".",
  "envFile": ".env",
  "envSchema": [
    { "key": "AIDO_WS_URL", "description": "Aido WebSocket 地址，由主程序注入", "required": false },
    { "key": "AIDO_TOKEN", "description": "网关认证 Token，由主程序注入", "required": false },
    { "key": "FEISHU_APP_ID", "description": "飞书应用 App ID", "required": true },
    { "key": "FEISHU_APP_SECRET", "description": "飞书应用 App Secret", "required": true },
    { "key": "FEISHU_DOMAIN", "description": "可选，lark | feishu", "required": false }
  ]
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | 是 | 唯一标识，与配置中 `bridges.instances[].id` 对应。 |
| `name` | 是 | 展示名称。 |
| `description` | 否 | 简短说明，可供 AI 或 UI 展示。 |
| `runtime` | 是 | 运行环境：`node`（需系统 PATH 有 node/npm）、`npx`、`python`、`exec`（直接执行每条命令的首元素）。 |
| `commands` | 是 | 命令数组；每项为一条命令（字符串数组）。**前 N-1 条**按顺序执行并等待完成，**最后一条**为常驻进程。只需一条时写一项即可，如 `[["npm","run","start"]]`。 |
| `cwd` | 否 | 工作目录，相对 bridge 目录；默认 `"."`。 |
| `envFile` | 否 | 相对 bridge 目录的 .env 文件；若存在则被加载后再叠加配置中的 `env` 与主程序注入变量。 |
| `envSchema` | 否 | 环境变量说明数组，用于 UI 展示或 AI 生成配置；每项含 `key`、`description`、`required`。 |

## 3. 主程序注入的环境变量

Aido 启动 bridge 进程时会自动注入：

- **`AIDO_WS_URL`**：当前网关 WebSocket 地址，如 `ws://127.0.0.1:19800/ws`。
- **`AIDO_TOKEN`**：网关认证 Token（来自 `gateway.auth.token`）。

bridge 无需在配置或 .env 中重复填写二者（可留空或占位）；主程序会覆盖/注入。

## 4. 配置（config.yaml）

在 Aido 主配置中增加 `bridges` 段：

```yaml
bridges:
  instances:
    - id: feishu
      enabled: true
      path: "bridges/feishu"
      env:
        FEISHU_APP_ID: "xxx"
        FEISHU_APP_SECRET: "xxx"
```

| 字段 | 说明 |
|------|------|
| `bridges.instances` | 列表，每项对应一个 bridge 实例。 |
| `id` | 与 `aido-bridge.json` 的 `id` 一致，用于发现清单。 |
| `enabled` | 是否随 Aido 启动时自动拉起。 |
| `path` | bridge 目录，相对 AIDO 工作目录或 AIDO_HOME；也可为绝对路径。 |
| `env` | 可选；覆盖或补充环境变量（含平台凭证等），主程序注入的 `AIDO_WS_URL`、`AIDO_TOKEN` 优先级更高。 |

## 5. 行为约定

- **发现**：Aido 根据 `bridges.instances[].path` 找到目录，读取该目录下的 `aido-bridge.json`；若缺少清单则跳过或报错。
- **启动**：对 `enabled: true` 的实例，在 bridge 目录（或清单 `cwd`）下按 `runtime` 执行 `commands`：前 N-1 条依次执行并等待完成，最后一条为常驻进程。
- **退出**：Aido 关闭时向已启动的 bridge 进程发送 SIGTERM，等待其退出。
- **AI 辅助**：可将本文档与各 bridge 的 `envSchema`、README 提供给大模型，由 AI 生成或补全 `bridges.instances` 与 `.env` 配置。

## 6. 多语言 / 多运行时

- **Node**：`runtime: "node"`，`commands` 如 `[["npm","run","start"]]` 或 `[["node","dist/index.js"]]`。
- **npx**：`runtime: "npx"`，`commands` 如 `[["npx","tsx","src/index.ts"]]`。
- **Python**：`runtime: "python"`，`commands` 如 `[["python","main.py"]]`。
- **任意可执行文件**：`runtime: "exec"`，每条命令为可执行路径与参数数组；Aido 不解析 runtime，仅按条执行。

同一仓库内官方 bridge 建议放在 `bridges/<id>/` 下，并均提供 `aido-bridge.json`。
