// End-to-end (`make e2e`): the Chat exchange docs/writing-a-factory.md
// shows, against a real service. A Heartbeat with chat: true makes Chat
// available (the icon's condition), the human sends a message, the Factory
// takes the turn with next_chat_work and posts a reply, and the reply
// streams from the Conversation's stream, the pop-up's feed.
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the documented Chat exchange", () => {
  let svc: Service;
  beforeAll(async () => {
    svc = await serve();
  }, 30_000);
  afterAll(async () => {
    await svc?.stop();
  });

  it("a Heartbeat with chat shows Chat, and a posted reply streams into the Conversation", async () => {
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const factory = { factory: "sandcastle", host: "build-1" };
    const heartbeat = (chat: boolean) =>
      fetch(`${svc.url}/api/p/acme/report_heartbeat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...factory, state: "online", note: "", interval_seconds: 30, chat }),
      });
    const project = async () => (await fetch(`${svc.url}/api/projects/acme`)).json();

    // Without Chat support there is no icon.
    expect((await heartbeat(false)).status).toBe(200);
    expect((await project()).factory_status.factories).toMatchObject([{ chat: false }]);

    // With it, the Factory reports Chat, which is what shows the icon.
    expect((await heartbeat(true)).status).toBe(200);
    expect((await project()).factory_status.factories).toMatchObject([{ chat: true }]);

    // Open the pop-up's stream before the reply, as the UI does.
    const abort = new AbortController();
    const stream = await fetch(`${svc.url}/api/p/acme/chat/stream`, { signal: abort.signal });
    expect(stream.status).toBe(200);
    const reader = stream.body!.getReader();
    const decoder = new TextDecoder();
    let received = "";
    const until = async (text: string) => {
      while (!received.includes(text)) {
        const { value, done } = await reader.read();
        if (done) throw new Error(`the stream ended before ${JSON.stringify(text)}; got ${received}`);
        received += decoder.decode(value, { stream: true });
      }
    };

    await svc.human("/api/p/acme/send_chat_message", { text: "What does Acme do?" }).catch(async () => {
      await svc.human("/api/p/acme/start_conversation", {});
      await svc.human("/api/p/acme/send_chat_message", { text: "What does Acme do?" });
    });

    const work = await (await fetch(`${svc.url}/api/p/acme/chat/next-work?factory=${factory.factory}&host=${factory.host}`)).json();
    expect(work.turn).toMatchObject({ text: "What does Acme do?", first: true });

    const post = (body: object) =>
      fetch(`${svc.url}/api/p/acme/chat/events`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ conversation: work.turn.conversation, ...factory, ...body }),
      });
    const item = (text: string) => ({ time: "2026-11-14T10:00:00Z", kind: "text", text });

    expect((await post({ session_id: "s-1", resume_command: "cd /w && claude --resume s-1", items: [item("Acme sells anvils.")] })).status).toBe(204);
    await until("Acme sells anvils.");
    expect((await post({ items: [item("Next day delivery.")], outcome: "done" })).status).toBe(204);
    await until("Next day delivery.");
    abort.abort();

    const chat = await (await fetch(`${svc.url}/api/p/acme/chat`)).json();
    expect(chat).toMatchObject({ availability: "available", running: false, session_id: "s-1" });
    expect(chat.messages).toMatchObject([
      { from: "human", text: "What does Acme do?" },
      { from: "claude", outcome: "done", items: [{ text: "Acme sells anvils." }, { text: "Next day delivery." }] },
    ]);
  });
});
