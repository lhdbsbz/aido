import * as Lark from "@larksuiteoapi/node-sdk";

export type FeishuConfig = {
  appId: string;
  appSecret: string;
  domain?: "feishu" | "lark";
};

export type OnFeishuMessage = (params: {
  chatId: string;
  messageId: string;
  senderId: string;
  text: string;
  chatType: "p2p" | "group";
}) => void;

export type OnChatId = (chatId: string) => void;

type FeishuMessageEvent = {
  message?: {
    message_id?: string;
    chat_id?: string;
    chat_type?: "p2p" | "group";
    message_type?: string;
    content?: string;
  };
  sender?: { sender_id?: { open_id?: string; user_id?: string } };
};

type FeishuChatIdEvent = { chat_id?: string; event?: { chat_id?: string } };

export class FeishuHandler {
  private client: Lark.Client;
  private wsClient: Lark.WSClient | null = null;
  private onMessage: OnFeishuMessage | null = null;
  private onP2PChatEntered: OnChatId | null = null;
  private onBotAddedToGroup: OnChatId | null = null;
  private onBotRemovedFromGroup: OnChatId | null = null;
  private onMessageRead: ((data: unknown) => void) | null = null;

  constructor(private config: FeishuConfig) {
    const domain =
      config.domain === "lark" ? Lark.Domain.Lark : Lark.Domain.Feishu;
    this.client = new Lark.Client({
      appId: config.appId,
      appSecret: config.appSecret,
      appType: Lark.AppType.SelfBuild,
      domain,
    });
  }

  setOnMessage(handler: OnFeishuMessage): void {
    this.onMessage = handler;
  }

  setOnP2PChatEntered(handler: OnChatId | null): void {
    this.onP2PChatEntered = handler;
  }

  setOnBotAddedToGroup(handler: OnChatId | null): void {
    this.onBotAddedToGroup = handler;
  }

  setOnBotRemovedFromGroup(handler: OnChatId | null): void {
    this.onBotRemovedFromGroup = handler;
  }

  setOnMessageRead(handler: ((data: unknown) => void) | null): void {
    this.onMessageRead = handler;
  }

  /**
   * 使用飞书 WebSocket 长连接接收事件，无需配置回调地址。
   * 需在飞书开放平台「事件与回调」中选择「使用长连接接收事件」并保存（本客户端需在线）。
   */
  start(): void {
    const domain =
      this.config.domain === "lark" ? Lark.Domain.Lark : Lark.Domain.Feishu;
    const wsClient = new Lark.WSClient({
      appId: this.config.appId,
      appSecret: this.config.appSecret,
      domain,
      loggerLevel: Lark.LoggerLevel.info,
    });
    this.wsClient = wsClient;

    const eventDispatcher = new Lark.EventDispatcher({
      verificationToken: "",
      encryptKey: undefined,
    });

    eventDispatcher.register({
      "im.message.receive_v1": async (data: unknown) => {
        const event = data as FeishuMessageEvent;
        if (!event.message || !this.onMessage) return;
        const chatId = event.message.chat_id ?? "";
        const messageId = event.message.message_id ?? "";
        const chatType =
          event.message.chat_type === "group" ? "group" : "p2p";
        const senderId =
          event.sender?.sender_id?.open_id ??
          event.sender?.sender_id?.user_id ??
          "";
        let text = "";
        if (event.message.message_type === "text") {
          try {
            const content = JSON.parse(event.message.content ?? "{}");
            text = content.text ?? "";
          } catch {
            text = event.message.content ?? "";
          }
        }
        if (chatId || messageId) {
          this.onMessage({ chatId, messageId, senderId, text, chatType });
        }
      },
      "im.chat.access_event.bot_p2p_chat_entered_v1": async (data: unknown) => {
        const event = data as FeishuChatIdEvent;
        const chatId = event.chat_id ?? event.event?.chat_id ?? "";
        if (chatId && this.onP2PChatEntered) this.onP2PChatEntered(chatId);
      },
      "im.chat.member.bot.added_v1": async (data: unknown) => {
        const event = data as FeishuChatIdEvent;
        const chatId = event.chat_id ?? event.event?.chat_id ?? "";
        if (chatId && this.onBotAddedToGroup) this.onBotAddedToGroup(chatId);
      },
      "im.chat.member.bot.deleted_v1": async (data: unknown) => {
        const event = data as FeishuChatIdEvent;
        const chatId = event.chat_id ?? event.event?.chat_id ?? "";
        if (chatId && this.onBotRemovedFromGroup) this.onBotRemovedFromGroup(chatId);
      },
      "im.message.message_read_v1": async (data: unknown) => {
        if (this.onMessageRead) this.onMessageRead(data);
      },
    });

    wsClient.start({ eventDispatcher });
  }

  async sendText(chatId: string, text: string): Promise<void> {
    await this.client.im.v1.message.create({
      params: { receive_id_type: "chat_id" },
      data: {
        receive_id: chatId,
        msg_type: "text",
        content: JSON.stringify({ text }),
      },
    });
  }
}
