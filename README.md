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
    workspace: "./workspace"
```

### 运行

```bash
# 设置 API Key
export ANTHROPIC_API_KEY="your-key"
export AIDO_TOKEN="your-token"

# 启动服务
./aido serve

# 或指定配置文件
./aido serve --config /path/to/config.yaml
```

访问 http://localhost:19800 进入 Web UI。

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

发送消息：
```json
{
  "type": "message",
  "content": "帮我写一个 Go 程序",
  "agent": "default"
}
```

#### REST API

- `GET /health` - 健康检查
- `GET /config` - 获取配置
- `POST /chat/send` - 发送消息
- `GET /sessions` - 会话管理
- `GET /bridges` - 桥接器状态

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
├── api/                 # API 定义
├── bridges/            # 平台桥接器
│   ├── feishu/        # 飞书桥接器示例
│   ├── SPEC.md        # 桥接器开发规范
│   └── README.md      # 桥接器说明
├── cmd/
│   └── aido/          # CLI 入口
├── internal/
│   ├── agent/         # Agent 逻辑
│   ├── bridge/        # 桥接器管理
│   ├── config/        # 配置管理
│   ├── gateway/       # HTTP/WebSocket 网关
│   ├── llm/           # LLM 客户端
│   ├── mcp/           # MCP 协议支持
│   ├── memory/        # 记忆管理
│   ├── message/       # 消息处理
│   ├── prompts/       # 提示词模板
│   ├── session/       # 会话管理
│   ├── skills/        # 技能系统
│   ├── tool/          # 工具注册
│   └── workspace/    # 工作空间
└── go.mod
```

## 🎯 配置文件详解

### Gateway

```yaml
gateway:
  port: 19800                 # 服务端口
  currentAgent: "default"     # 默认 Agent
  toolsProfile: "coding"      # 工具档位：minimal/coding/messaging/full
  locale: "zh"               # 语言：en/zh
  auth:
    token: "${AIDO_TOKEN}"   # 认证 Token
```

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
    fallbacks:             # 备用模型
      - "openai/gpt-4o"
    workspace: "./workspace"
    skills:
      dirs: ["./workspace/skills"]
```

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

Options:
  --config PATH  指定配置文件路径
  --port PORT    指定端口（覆盖配置文件）
```

## 🤝 贡献

欢迎贡献代码！请查看 [CONTRIBUTING.md](CONTRIBUTING.md) 了解详情。

## 📄 许可证

本项目采用 AGPL-3.0 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🙏 致谢

感谢所有为这个项目做出贡献的人！

---

**Stars** 和 **Issues** 欢迎！
