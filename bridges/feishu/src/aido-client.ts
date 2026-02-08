import WebSocket from "ws";

const CHANNEL = "feishu";

export type MessageSendParams = {
  channelChatId: string;
  text: string;
  senderId?: string;
  messageId?: string;
  attachments?: { type: string; url?: string; base64?: string; mime?: string }[];
};

export type OutboundHandler = (payload: { channel: string; channelChatId: string; text: string }) => void;

type Frame = {
  type: string;
  id?: string;
  method?: string;
  params?: unknown;
  ok?: boolean;
  payload?: unknown;
  error?: { code: string; message: string };
  event?: string;
  seq?: number;
};

export class AidoClient {
  private ws: WebSocket | null = null;
  private url: string;
  private token: string;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reqId = 0;
  private onOutbound: OutboundHandler | null = null;

  constructor(url: string, token: string) {
    this.url = url;
    this.token = token;
  }

  onOutboundMessage(handler: OutboundHandler): void {
    this.onOutbound = handler;
  }

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(this.url);
      this.ws = ws;

      ws.on("open", () => {
        const id = `connect-${++this.reqId}`;
        ws.send(
          JSON.stringify({
            type: "req",
            id,
            method: "connect",
            params: {
              role: "bridge",
              token: this.token,
              channel: CHANNEL,
              capabilities: ["text", "media"],
            },
          })
        );

        const onMsg = (raw: Buffer) => {
          const frame = JSON.parse(raw.toString()) as Frame;
          if (frame.type === "res" && frame.id === id) {
            ws.off("message", onMsg);
            if (frame.ok) {
              resolve();
            } else {
              reject(new Error(frame.error?.message ?? "connect failed"));
            }
          }
        };
        ws.on("message", onMsg);
      });

      ws.on("message", (data: Buffer) => {
        const frame = JSON.parse(data.toString()) as Frame;
        if (frame.type === "event" && frame.event === "outbound.message" && frame.payload) {
          const pl = frame.payload as { channel?: string; channelChatId?: string; text?: string };
          if (pl.channel === CHANNEL && this.onOutbound) {
            this.onOutbound({
              channel: pl.channel,
              channelChatId: String(pl.channelChatId ?? ""),
              text: String(pl.text ?? ""),
            });
          }
        }
      });

      ws.on("close", () => {
        this.ws = null;
        this.scheduleReconnect(resolve);
      });

      ws.on("error", (err) => {
        reject(err);
      });
    });
  }

  private scheduleReconnect(onReconnect?: () => void): void {
    if (this.reconnectTimer) return;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect()
        .then(() => onReconnect?.())
        .catch(() => this.scheduleReconnect(onReconnect));
    }, 3000);
  }

  sendMessage(params: MessageSendParams): Promise<{ text?: string }> {
    return new Promise((resolve, reject) => {
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
        reject(new Error("Aido WebSocket not connected"));
        return;
      }
      const id = `msg-${++this.reqId}`;
      const handler = (raw: Buffer) => {
        const frame = JSON.parse(raw.toString()) as Frame;
        if (frame.type === "res" && frame.id === id) {
          this.ws?.off("message", handler);
          if (frame.ok && frame.payload) {
            resolve((frame.payload as { text?: string }) ?? {});
          } else {
            reject(new Error(frame.error?.message ?? "message.send failed"));
          }
        }
      };
      this.ws.on("message", handler);
      this.ws.send(
        JSON.stringify({
          type: "req",
          id,
          method: "message.send",
          params: {
            channel: CHANNEL,
            channelChatId: params.channelChatId,
            text: params.text,
            senderId: params.senderId,
            messageId: params.messageId,
            attachments: params.attachments ?? [],
          },
        })
      );
    });
  }

  close(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}
