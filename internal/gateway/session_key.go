package gateway

// SessionKey returns the internal session key for storage/lock: channel:channelChatId.
// No agentId; switching agent config does not change session.
func SessionKey(channel, channelChatId string) string {
	if channel == "" {
		channel = "direct"
	}
	if channelChatId == "" {
		channelChatId = "main"
	}
	return channel + ":" + channelChatId
}
