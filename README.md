# Aido - AI Agent Gateway

<div align="center">

**🚀 Aido - 你的 AI Agent 网关**

一个功能强大的 AI Agent 框架，支持多 LLM 提供商、工具系统、可视化界面和平台桥接。

[![Go Version](https://img.shields.io/badge/Go-1.25-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-AGPL--3.0-yellow.svg)](LICENSE)

</div>

## ✨ 特性

- 🔌 **多 LLM 支持**：OpenAI、Anthropic、DeepSeek、Minimax 等
- 🛠️ **丰富的工具集**：文件系统、执行命令、Web 搜索、MCP 服务器
- 🌉 **平台桥接器**：支持集成飞书等平台（见 `bridges/`）
- 🎯 **多 Agent 管理**：为不同场景配置专属 Agent
- 💻 **技能系统**：加载和管理 AI 技能
- 🎨 **Web UI**：可视化配置管理界面
- 🔄 **热重载**：配置变更无需重启
- 💾 **会话管理**：持久化对话历史

## 🚀 快速开始

### 安装

```bash
# 克隆仓库
git clone https://github.com/lhdbsbz/aido.git
cd aido

# 构建
go build -o aido ./cmd/aido

# 或使用 Go 运行
go run ./cmd/aido serve
```

### 配置

首次运行会自动创建配置文件：
- **Linux/macOS**: `~/.aido/config.yaml`
- **Windows**: `%USERPROFILE%\.aido\config.yaml`

编辑配置文件：

```yaml
gateway:
  port: 19800
  auth:
    token: "${AIDO_TOKEN}"  # 设置环境变量或直接填写

providers:
  anthropic:
    apiKey: "${ANTHROPIC_API_KEY}"
    type: "anthropic"
  openai:
    apiKey: "${OPENAI_API_KEY}"
    type: "openai"

agents:
  default:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
```

### 运行

```bash
# 设置 API Key
export ANTHROPIC_API_KEY="your-key"
export AIDO_TOKEN="your-token"

# 启动服务
./aido serve
```

访问 http://localhost:19800 进入 Web UI。

> ⚠️ **注意**：当前版本仅支持通过配置文件设置端口，CLI 不支持 `--port` 和 `--config` 参数。

## 📖 使用指南

### Web UI

- **首页**：对话界面，支持多轮对话
- **配置**：管理 Agent、Provider、工具和桥接器
- **健康检查**：查看服务状态

### API 接口

#### WebSocket 实时对话

```bash
ws://localhost:19800/ws
```

**连接认证：**
```json
{
  "type": "req",
  "id": "connect-1",
  "method": "connect",
  "params": {
    "role": "client",  // 或 "bridge"
    "token": "<your-token>"
  }
}
```

**发送消息：**
```json
{
  "type": "req",
  "id": "msg-1",
  "method": "message.send",
  "params": {
    "channel": "webchat",
    "channelChatId": "device-abc",
    "text": "帮我写一个 Go 程序"
  }
}
```

**支持的 WebSocket 方法：**
- `connect` - 建立连接（Client 或 Bridge 角色）
- `message.send` - 发送消息
- `chat.history` - 获取对话历史
- `sessions.list` - 获取会话列表
- `health` - 健康检查
- `config.get` - 获取配置

**服务端推送事件：**
- `user_message` - 用户消息已接收
- `agent` - Agent 运行过程（流式输出、工具调用等）
- `outbound.message` - 最终回复（仅 Bridge 收到）

#### REST API

**无需认证：**
- `GET /health` - 健康检查（返回状态、连接数等）

**需要认证（Header `Authorization: Bearer <token>`）：**
- `GET /api/health` - 认证版健康检查
- `GET /api/config` - 获取配置（脱敏）
- `PUT /api/config` - 更新配置
- `GET /api/bridges` - 查询桥接器状态
- `POST /api/chat/send` - 发送消息（无状态模式）
- `GET /api/chat/history` - 获取对话历史
- `GET /api/sessions` - 获取会话列表

**OpenAI 兼容接口：**
- `POST /v1/chat/completions` - OpenAI 兼容的 Chat API，支持流式和非流式

### MCP 工具集成

在配置文件中添加 MCP 服务器：

```yaml
tools:
  mcp:
    - name: "github"
      command: "npx"
      args: ["@anthropic/mcp-server-github"]
      transport: "stdio"
      env:
        GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

## 🏗️ 项目结构

```
aido/
├── api/                 # API 接入指南文档
├── bridges/            # 平台桥接器
│   ├── feishu/        # 飞书桥接器示例
│   ├── SPEC.md        # 桥接器开发规范
│   └── README.md      # 桥接器说明
├── cmd/
│   └── aido/          # CLI 入口
├── internal/
│   ├── agent/         # Agent 逻辑核心
│   ├── bridge/        # 桥接器生命周期管理
│   ├── config/        # 配置加载和管理
│   ├── gateway/       # HTTP/WebSocket 网关
│   ├── llm/           # LLM 客户端（OpenAI/Anthropic 兼容）
│   ├── mcp/           # MCP 协议客户端
│   ├── session/       # 会话存储管理
│   ├── skills/        # 技能系统
│   ├── tool/          # 工具注册和策略控制
│   └── ...
└── go.mod
```

> **说明**：`memory/`、`message/`、`prompts/` 等为内部辅助模块，详细设计见源码注释。

## 🎯 配置文件详解

### Gateway

```yaml
gateway:
  port: 19800                 # 服务端口
  currentAgent: "default"     # 默认 Agent
  locale: "zh"               # 语言：en/zh
  auth:
    token: "${AIDO_TOKEN}"   # 认证 Token
```

内置工具与 MCP 工具均全部开放，无权限或档位配置。

### Providers

```yaml
providers:
  openai:
    apiKey: ""               # API Key
    type: "openai"          # 类型
    baseURL: ""             # 可选：自定义 API 地址
  anthropic:
    apiKey: ""
    type: "anthropic"
```

### Agents

```yaml
agents:
  default:
    provider: "anthropic"   # 使用的 LLM 提供商
    model: "claude-sonnet-4-20250514"
```

工作目录与技能目录固定为 `~/.aido/workspace` 与 `~/.aido/workspace/skills`（可通过 `AIDO_HOME` 修改根目录），无需在配置里指定。

**目录规范（均基于 Home = `~/.aido`）：**

- **Workspace**：默认工作区，代码、MEMORY.md、memory/*.md 等。
- **Temp**（`~/.aido/tmp`）：仅放任务产生的临时文件，可被定期清理；勿放重要数据。
- **Store**（`~/.aido/data/store`）：密钥、重要配置等需长期保存的文件；勿与工作区或 Temp 混用。
- 技能、工具、MCP 均在此 Home 下；模型被要求只使用上述目录，临时用 Temp、重要用 Store。

### 技能目录 (Skills)

技能从固定目录加载：**`~/.aido/workspace/skills`**（即 `{AIDO_HOME}/workspace/skills`）。目录结构必须为「一个技能一个子目录，子目录内放 `SKILL.md`」：

```
~/.aido/workspace/skills/
├── feishu-wiki/
│   └── SKILL.md
├── feishu-doc/
│   └── SKILL.md
└── my-skill/
    └── SKILL.md
```

- **错误**：在 `skills/` 下直接放 `SKILL.md`（没有以技能名为名的子目录）不会被识别。
- **正确**：`skills/<技能名>/SKILL.md`，技能名即子目录名。

`SKILL.md` 建议使用 YAML frontmatter，其中 `description` 会出现在系统提示里供模型选择技能：

```markdown
---
description: "在需要查阅/搜索飞书知识库时使用本技能"
---

# 飞书知识库技能
...
```

启动时或通过 Web 配置页触发重载时会扫描该目录；模型通过 `read_file` 按 `<location>`（相对工作目录，如 `skills/feishu-wiki/SKILL.md`）读取技能内容。

### Bridges（桥接器）

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

## 🛠️ 开发

### 添加新工具

1. 在 `internal/tool/` 目录创建工具文件
2. 实现 `Tool` 接口
3. 在 `tool.RegisterXXX()` 中注册

### 添加桥接器

参考 `bridges/SPEC.md` 开发规范。

### 添加 MCP 服务器

```go
// 在 config.yaml 中配置
tools:
  mcp:
    - name: "server-name"
      command: "./mcp-server"
      transport: "stdio"
      env:
        KEY: "value"
```

## 📝 命令行选项

```bash
Usage:
  aido serve     启动网关服务
  aido version   显示版本信息
```

> ⚠️ 当前版本仅支持通过配置文件设置端口（`gateway.port`）。

## 🤝 贡献

欢迎贡献代码！请查看 [CONTRIBUTING.md](CONTRIBUTING.md) 了解详情。

## 📄 许可证

本项目采用 AGPL-3.0 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🙏 致谢

感谢所有为这个项目做出贡献的人！

---

**Stars** 和 **Issues** 欢迎！
