import { beforeEach, expect, it } from "vitest";
import type { Conversation } from "../types";
import { ApiError } from "../types";
import type { Driver } from "../driver";

export function chatContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- the Conversation stream (Spec code-central-633) ----

  const chatHeartbeat = { factory: "code-central-factory", host: "laptop", state: "online" as const, interval_seconds: 30, chat: true };

  it("follows a Conversation: it opens as it stands, then each message, item and outcome, resuming after a dropped stream with no gaps or repeats", async () => {
    await d.createProject("acme", "Acme");
    await d.reportHeartbeat("acme", chatHeartbeat);
    await d.sendChatMessage("acme", "What is acme?");
    const conversation = await d.takeChatTurn("acme", "code-central-factory");
    await d.postChatEvents("acme", "code-central-factory", conversation, ["one"]);

    const heard: string[] = [];
    const opens: boolean[] = [];
    let opening: Conversation | undefined;
    const stop = d.api.subscribeChat("acme", {
      onConversation: (c) => {
        opening = c;
        heard.push("conversation");
      },
      onMessage: (m) => heard.push(`message from ${m.from}`),
      onItem: (i) => heard.push(`item ${i.text}`),
      onOutcome: (o) => heard.push(`outcome ${o.outcome}`),
      onReplaced: () => heard.push("replaced"),
      onOpen: ({ reconnect }) => opens.push(reconnect),
    });
    try {
      await expect.poll(() => heard, { timeout: 5000 }).toEqual(["conversation"]);
      expect(opening).toMatchObject({ availability: "available", running: true, messages: [{ from: "human", text: "What is acme?" }, { from: "claude", items: [{ text: "one" }] }] });

      await d.postChatEvents("acme", "code-central-factory", conversation, ["two"]);
      await expect.poll(() => heard.length, { timeout: 5000 }).toBe(2);

      d.dropTranscriptStreams();
      await d.postChatEvents("acme", "code-central-factory", conversation, ["three"], "done");
      await expect.poll(() => opens, { timeout: 5000 }).toEqual([false, true]);
      await expect.poll(() => heard.length, { timeout: 5000 }).toBe(4);
      expect(heard).toEqual(["conversation", "item two", "item three", "outcome done"]);

      // A second message streams as the human's, then the reply's.
      await d.sendChatMessage("acme", "And?");
      await expect.poll(() => heard.length, { timeout: 5000 }).toBe(6);
      expect(heard.slice(4)).toEqual(["message from human", "message from claude"]);
    } finally {
      stop();
    }
  });

  it("says replaced when the Conversation is replaced, then sends the new one", async () => {
    await d.createProject("acme", "Acme");
    await d.reportHeartbeat("acme", chatHeartbeat);
    await d.sendChatMessage("acme", "Hello");

    const heard: string[] = [];
    let latest: Conversation | undefined;
    const stop = d.api.subscribeChat("acme", {
      onConversation: (c) => {
        latest = c;
        heard.push(`conversation of ${c.messages.length}`);
      },
      onReplaced: () => heard.push("replaced"),
      onMessage: (m) => heard.push(`message from ${m.from}`),
    });
    try {
      await expect.poll(() => heard, { timeout: 5000 }).toEqual(["conversation of 2"]);
      const first = latest!.id;
      await d.startConversation("acme");
      await expect.poll(() => heard, { timeout: 5000 }).toEqual(["conversation of 2", "replaced", "conversation of 0"]);
      expect(latest!.id).not.toBe(first);

      // The stream goes on with the new Conversation, also across a drop.
      d.dropTranscriptStreams();
      await d.sendChatMessage("acme", "Again");
      await expect.poll(() => heard.length, { timeout: 5000 }).toBe(5);
      expect(heard.slice(3)).toEqual(["message from human", "message from claude"]);
      expect(heard.filter((h) => h === "replaced")).toHaveLength(1);
    } finally {
      stop();
    }
  });

  // ---- Chat actions (Spec code-central-633) ----

  it("reads no Conversation before one starts, and starts one with the Chat row's defaults on the first send", async () => {
    await d.createProject("acme", "Acme");
    await d.reportHeartbeat("acme", chatHeartbeat);
    expect(await d.api.getChat("acme")).toMatchObject({ availability: "none", running: false, messages: [] });

    await d.api.setModels("acme", { default: { model: "", effort: "" }, chat: { model: "sonnet", effort: "low" }, kinds: [] });
    const sent = await d.api.sendChatMessage("acme", "Hello");
    expect(sent).toMatchObject({ availability: "available", model: "sonnet", effort: "low", running: true, messages: [{ from: "human", text: "Hello" }, { from: "claude" }] });
    expect(await d.api.getChat("acme")).toEqual(sent);
  });

  it("starts a Conversation with a picked model and effort, replacing the current one, and refuses a bad effort", async () => {
    await d.createProject("acme", "Acme");
    await d.reportHeartbeat("acme", chatHeartbeat);
    const first = await d.api.startConversation("acme", { model: "", effort: "" });
    expect(first).toMatchObject({ availability: "available", model: "haiku", running: false, messages: [] });

    const second = await d.api.startConversation("acme", { model: "opus", effort: "high" });
    expect(second).toMatchObject({ model: "opus", effort: "high" });
    expect(second.id).not.toBe(first.id);

    const refused = await d.api.startConversation("acme", { model: "opus", effort: "extreme" }).catch((e) => e);
    expect(refused).toMatchObject({ status: 422, code: "invalid", field: "effort" });
    expect((await d.api.getChat("acme")).id).toBe(second.id);
  });

  it("refuses a Chat Message while a reply runs, stops the reply, and refuses one with no Factory to answer", async () => {
    await d.createProject("acme", "Acme");
    const unavailable = await d.api.sendChatMessage("acme", "Hello").catch((e) => e);
    expect(unavailable).toBeInstanceOf(ApiError);

    await d.reportHeartbeat("acme", chatHeartbeat);
    await d.api.sendChatMessage("acme", "Hello");
    const busy = await d.api.sendChatMessage("acme", "Again").catch((e) => e);
    expect(busy).toMatchObject({ status: 409, code: "action_not_allowed", allowed_actions: ["stop_chat_reply"] });

    // No Factory has taken the turn, so Stop ends the reply at once.
    const stopped = await d.api.stopChatReply("acme");
    expect(stopped).toMatchObject({ running: false, messages: [{ from: "human" }, { from: "claude", outcome: "stopped" }] });
    expect((await d.api.stopChatReply("acme")).running).toBe(false);
  });
}
