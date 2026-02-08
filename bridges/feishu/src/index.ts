import "dotenv/config";
import { AidoClient } from "./aido-client.js";
import { FeishuHandler } from "./feishu-handler.js";

const AIDO_WS_URL = process.env.AIDO_WS_URL ?? "ws://localhost:8080/ws";
const AIDO_TOKEN = process.env.AIDO_TOKEN ?? "";
const FEISHU_APP_ID = process.env.FEISHU_APP_ID ?? "";
const FEISHU_APP_SECRET = process.env.FEISHU_APP_SECRET ?? "";
const FEISHU_DOMAIN = process.env.FEISHU_DOMAIN === "lark" ? "lark" : "feishu";
const FEISHU_WELCOME_P2P = process.env.FEISHU_WELCOME_P2P ?? "";
const FEISHU_WELCOME_GROUP = process.env.FEISHU_WELCOME_GROUP ?? "";

function main(): void {
  if (!AIDO_TOKEN) {
    console.error("Missing AIDO_TOKEN");
    process.exit(1);
  }
  if (!FEISHU_APP_ID || !FEISHU_APP_SECRET) {
    console.error("Missing FEISHU_APP_ID or FEISHU_APP_SECRET");
    process.exit(1);
  }

  const aido = new AidoClient(AIDO_WS_URL, AIDO_TOKEN);
  const feishu = new FeishuHandler({
    appId: FEISHU_APP_ID,
    appSecret: FEISHU_APP_SECRET,
    domain: FEISHU_DOMAIN,
  });

  feishu.setOnMessage(async ({ chatId, messageId, senderId, text, chatType }) => {
    console.log(`[feishu] ${chatType} ${chatId} from ${senderId}: ${text.slice(0, 80)}`);
    try {
      await aido.sendMessage({
        channelChatId: chatId,
        text,
        senderId,
        messageId,
      });
    } catch (err) {
      console.error("[feishu] send to Aido failed:", err);
    }
  });

  if (FEISHU_WELCOME_P2P) {
    feishu.setOnP2PChatEntered(async (chatId) => {
      console.log("[feishu] user entered P2P chat", chatId);
      try {
        await feishu.sendText(chatId, FEISHU_WELCOME_P2P);
      } catch (err) {
        console.error("[feishu] welcome P2P failed:", err);
      }
    });
  }
  if (FEISHU_WELCOME_GROUP) {
    feishu.setOnBotAddedToGroup(async (chatId) => {
      console.log("[feishu] bot added to group", chatId);
      try {
        await feishu.sendText(chatId, FEISHU_WELCOME_GROUP);
      } catch (err) {
        console.error("[feishu] welcome group failed:", err);
      }
    });
  }
  feishu.setOnBotRemovedFromGroup((chatId) => {
    console.log("[feishu] bot removed from group", chatId);
  });
  feishu.setOnMessageRead(() => {
    // 消息已读事件，仅记录或后续扩展
  });

  aido.onOutboundMessage(async (payload) => {
    if (payload.channel !== "feishu") return;
    console.log(`[aido] outbound -> feishu ${payload.channelChatId}`);
    try {
      await feishu.sendText(payload.channelChatId, payload.text);
    } catch (err) {
      console.error("[aido] send to Feishu failed:", err);
    }
  });

  console.log("[feishu] starting WebSocket long connection (no callback URL needed)");
  feishu.start();

  aido.connect().then(
    () => {
      console.log("[aido] WebSocket connected");
    },
    (err) => {
      console.error("[aido] WebSocket connect failed:", err);
      process.exit(1);
    }
  );

  process.on("SIGINT", () => {
    aido.close();
    process.exit(0);
  });
}

main();
