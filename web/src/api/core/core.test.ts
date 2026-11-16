// The WASM Core's worlds: each a fresh Core, reached through the real
// createHttpApi, with its own clock and the writes it was asked for.
import { describe, expect, it, onTestFinished, vi } from "vitest";
import { scenario } from "../scenario";
import type { ChangeEvent } from "../types";
import { dropWorlds, newWorld, type World } from "./core";

describe("a WASM Core world", () => {
  it("is a fresh Core with an empty store of its own", async () => {
    const a = await newWorld();
    await scenario(a).project("acme", "Acme");
    expect(await a.api.listProjects()).toMatchObject([{ key: "acme", display_name: "Acme" }]);
    expect(await (await newWorld()).api.listProjects()).toEqual([]);
  });

  it("starts its clock where told, and moves it only on advance", async () => {
    const w = await newWorld("2026-03-01T12:00:00Z");
    expect(w.now().toISOString()).toBe("2026-03-01T12:00:00.000Z");
    w.advance(90 * 60_000);
    expect(w.now().toISOString()).toBe("2026-03-01T13:30:00.000Z");
    await scenario(w).project("acme");
    const id = await scenario(w).ticket("acme", { title: "Later" });
    expect((await w.api.getTicket("acme", id)).created).toBe("2026-03-01T13:30:00Z");
  });

  it("records each Api method called, and the writes alone as actions", async () => {
    const w = await newWorld();
    await scenario(w).project("acme");
    await w.api.listProjects();
    await w.api.renameProject("acme", "Acme Web");
    const id = await scenario(w).ticket("acme", { title: "A", state: "Draft" });
    await w.api.act("acme", id, "approve", {});
    expect(w.calls).toEqual(["listProjects", "renameProject", "approve"]);
    expect(w.actions()).toEqual(["renameProject", "approve"]);
  });

  it("ends its live feed when dropped, and fails a later request as an unreachable service", async () => {
    const w = await newWorld();
    let opened!: () => void;
    const open = new Promise<void>((r) => (opened = r));
    const stop = w.api.subscribe({ onEvent: () => {}, onOpen: () => opened() });
    try {
      await open;
      dropWorlds();
      await expect(w.api.listProjects()).rejects.toMatchObject({ code: "service_unavailable" });
      expect(() => w.now()).toThrow(/dropped/);
    } finally {
      stop();
    }
  });
});

type OnOpen = (info: { reconnect: boolean }) => void;

describe("a WASM Core world's settled", () => {
  // feed subscribes a change feed on w until the test ends, recording its
  // notices and, each time it opens, whether it reconnected.
  function feed(w: World) {
    const events: ChangeEvent[] = [];
    const opens: boolean[] = [];
    onTestFinished(w.api.subscribe({ onEvent: (e) => events.push(e), onOpen: ({ reconnect }) => opens.push(reconnect) }));
    return { events, opens };
  }

  it("resolves straight away on a fresh world", async () => {
    const w = await newWorld();
    await w.settled();
    expect(w.fetchesStarted()).toBe(0);
  });

  it("resolves only once a subscribed feed has opened", async () => {
    const w = await newWorld();
    const { opens } = feed(w);
    await w.settled();
    expect(opens).toEqual([false]);
  });

  it("resolves only once a write's notice has reached the feed", async () => {
    const w = await newWorld();
    await scenario(w).project("acme");
    const { events } = feed(w);
    await w.settled();
    const renamed = w.api.renameProject("acme", "Acme Web");
    await w.settled();
    expect(events).toEqual([{ type: "project.changed", project: "acme" }]);
    await renamed;
  });

  it("resolves after dropped streams only once the feed has reconnected", async () => {
    const w = await newWorld();
    const { opens } = feed(w);
    await w.settled();
    w.dropStreams();
    await w.settled();
    expect(opens).toEqual([false, true]);
  });

  it.each([
    ["Transcript", (w: World, id: string, onOpen: OnOpen) => w.api.subscribeTranscript("acme", id, 1, { onEvent: () => {}, onOpen })],
    ["Chat", (w: World, _id: string, onOpen: OnOpen) => w.api.subscribeChat("acme", { onConversation: () => {}, onOpen })],
  ])("counts a %s stream as it counts the change feed", async (_kind, subscribe) => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme");
    const id = await s.ticket("acme", { title: "Fix login", state: "In Progress" });
    const opens: boolean[] = [];
    onTestFinished(subscribe(w, id, ({ reconnect }) => opens.push(reconnect)));
    await w.settled();
    expect(opens).toEqual([false]);

    w.dropStreams();
    await w.settled();
    expect(opens).toEqual([false, true]);
  });

  it("is held by a request in flight", async () => {
    const w = await newWorld();
    let answered = false;
    void w.api.listProjects().then(() => (answered = true));
    await w.settled();
    expect(answered).toBe(true);
    expect(w.fetchesStarted()).toBe(1);
  });

  it("rejects, naming the feed, when a subscribed feed can't open", async () => {
    const w = await newWorld();
    onTestFinished(w.api.subscribeTranscript("acme", "acme-9", 1, { onEvent: () => {} }));
    await expect(w.settled()).rejects.toThrow(/Transcript streams: 1 subscribed, 0 open/);
  });

  it("polls on a real timer while the test fakes them", async () => {
    vi.useFakeTimers();
    onTestFinished(() => void vi.useRealTimers());
    const w = await newWorld();
    const { opens } = feed(w);
    let answered = false;
    void w.api.listProjects().then(() => (answered = true));
    await w.settled();
    expect(opens).toEqual([false]);
    expect(answered).toBe(true);
  });
});
