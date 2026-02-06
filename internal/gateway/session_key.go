package gateway

import (
	"strings"

	"github.com/lhdbsbz/aido/internal/agent"
)

// SessionKey 约定（gateway 层）：
//
// 1. 存储格式（agent 层）
//    sessionKey = agentID:channel:chatID，例如 default:webchat:web-xxx。
//    Web 场景下 chatID 仅用 deviceId，不再把整段 webchat:default:xxx 塞进 chatID，避免 default:webchat:webchat:default:xxx 这种重复。
//
// 2. Web 客户端格式
//    前端使用 webchat:default:deviceId（如 webchat:default:web-xxx）。API 入参/出参保持该格式。
//
// 3. 转换
//    - 发消息：前端传 webchat:default:deviceId → gateway 只取 deviceId 作为 chatID 调 agent → 存 default:webchat:deviceId。
//    - 拉历史/列表：clientKey ⇄ storageKey 在 gateway 边界转换。
const webchatClientPrefix = "webchat:default:"
const webchatChannel = "webchat"

// WebchatChatID 从 Web 客户端 sessionKey 中取出作为 chatID 的部分（deviceId）。
// 传入 webchat:default:deviceId 返回 deviceId；否则原样返回（如 conn.ID、manager）。
func WebchatChatID(sessionKey string) string {
	if strings.HasPrefix(sessionKey, webchatClientPrefix) {
		return sessionKey[len(webchatClientPrefix):]
	}
	return sessionKey
}

// ToStorageKey 将客户端 sessionKey 转为存储 key。
// Web 格式 webchat:default:deviceId → default:webchat:deviceId（仅三层）。
func ToStorageKey(clientKey string) string {
	if strings.HasPrefix(clientKey, webchatClientPrefix) {
		deviceID := clientKey[len(webchatClientPrefix):]
		return agent.DeriveSessionKey("default", webchatChannel, deviceID)
	}
	return clientKey
}

// ToClientKey 将存储 key 转为 Web 客户端格式。仅对 webchat 有效。
// default:webchat:deviceId → webchat:default:deviceId
func ToClientKey(storageKey string) string {
	parts := strings.SplitN(storageKey, ":", 3)
	if len(parts) < 3 || parts[1] != webchatChannel {
		return ""
	}
	return webchatClientPrefix + parts[2]
}
