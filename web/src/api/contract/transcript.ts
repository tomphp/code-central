import { beforeEach, expect, it } from "vitest";
import type { ChangeEvent, HeldSessionEvent, SessionEvent } from "../types";
import type { Driver } from "../driver";

export function transcriptContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- the latest Session Event (ticket 109) ----
  it("shows a Claim's latest Session Event as one short line, on its Claim and on the Ticket's rows", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Busy", "Ready");
    const quiet = await d.createTicket("acme", "Quiet", "Ready");
    await d.claim("acme", id, "sess-busy");
    await d.claim("acme", quiet, "sess-quiet");
    const spec = (await d.api.getTicket("acme", id)).spec.id;
    const rowOf = async (t: string) => (await d.api.findTickets("acme")).find((r) => r.id === t)!;
    const specRowOf = async (t: string) => (await d.api.getSpec("acme", spec)).tickets!.find((r) => r.id === t)!;

    expect((await d.api.getTicket("acme", id)).claims![0].latest_event).toBeUndefined();
    expect((await rowOf(id)).latest_event).toBeUndefined();

    await d.postSessionEvents("sess-busy", [
      { time: "2026-11-08T10:00:00Z", kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: go test ./..." },
      { time: "2026-11-08T10:00:05Z", kind: "tool_result", tool_use_id: "tu-1", text: "FAIL\nexit 1", is_error: true },
    ]);
    const failed = { kind: "tool_result", text: "Bash failed", time: "2026-11-08T10:00:05Z" };
    expect((await d.api.getTicket("acme", id)).claims![0].latest_event).toEqual(failed);
    expect((await rowOf(id)).latest_event).toEqual(failed);
    expect((await specRowOf(id)).latest_event).toEqual(failed);
    expect((await d.api.getTicket("acme", quiet)).claims![0].latest_event).toBeUndefined();
    expect((await rowOf(quiet)).latest_event).toBeUndefined();
    expect((await specRowOf(quiet)).latest_event).toBeUndefined();

    await d.postSessionEvents("sess-busy", [
      { time: "2026-11-08T10:01:00Z", kind: "text", text: "\n  I'll fix the fixture.  \nThen run them again." },
    ]);
    const said = { kind: "text", text: "I'll fix the fixture.", time: "2026-11-08T10:01:00Z" };
    expect((await rowOf(id)).latest_event).toEqual(said);

    await d.postSessionEvents("sess-busy", [{ time: "2026-11-08T10:02:00Z", kind: "factory", text: "x".repeat(200) }]);
    const long = (await rowOf(id)).latest_event!;
    expect(long.text).toBe("x".repeat(119) + "…");

    // Once the Claim ends, it keeps its latest event; the rows have no live Claim to show.
    await d.api.act("acme", id, "release", {});
    expect((await d.api.getTicket("acme", id)).claims![0].latest_event).toMatchObject({ kind: "factory" });
    expect((await rowOf(id)).latest_event).toBeUndefined();
    expect((await specRowOf(id)).latest_event).toBeUndefined();
  });

  it("sends ticket.changed over the live feed when a Claim's latest Session Event changes", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Busy", "Ready");
    await d.claim("acme", id, "sess-busy");
    const got: ChangeEvent[] = [];
    let opened!: () => void;
    const open = new Promise<void>((r) => (opened = r));
    const stop = d.api.subscribe({ onEvent: (e) => got.push(e), onOpen: () => opened() });
    try {
      await open;
      await d.postSessionEvents("sess-busy", [{ time: "2026-11-08T10:00:00Z", kind: "text", text: "Reading the Spec." }]);
      await expect.poll(() => got.some((e) => e.type === "ticket.changed" && e.id === id), { timeout: 5000 }).toBe(true);
    } finally {
      stop();
    }
  });

  // ---- a Claim's Transcript (ticket 110) ----
  const said = (text: string, time = "2026-11-08T10:00:00Z"): SessionEvent => ({ time, kind: "text", text });
  const textsOf = (events: HeldSessionEvent[]) => events.map((e) => `${e.seq} ${e.text}`);

  it("reads a Claim's Transcript: its held events in order, each with its seq", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Busy", "Ready");
    await d.claim("acme", id, "sess-busy");
    expect(await d.api.transcript("acme", id, 1)).toEqual({ events: [], tail: false });

    await d.postSessionEvents("sess-busy", [
      { time: "2026-11-08T10:00:00Z", kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: go test ./..." },
      { time: "2026-11-08T10:00:05Z", kind: "tool_result", tool_use_id: "tu-1", text: "FAIL", is_error: true, truncated: true },
    ]);
    await d.postSessionEvents("sess-busy", [{ time: "2026-11-08T10:01:00Z", kind: "factory", text: "acme-2 turn ended" }]);
    expect(await d.api.transcript("acme", id, 1)).toEqual({
      events: [
        { seq: 1, time: "2026-11-08T10:00:00Z", kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: go test ./..." },
        { seq: 2, time: "2026-11-08T10:00:05Z", kind: "tool_result", tool_use_id: "tu-1", text: "FAIL", is_error: true, truncated: true },
        { seq: 3, time: "2026-11-08T10:01:00Z", kind: "factory", text: "acme-2 turn ended" },
      ],
      tail: false,
    });
  });

  it("refuses the Transcript of an unknown Project, Ticket or Claim as not_found", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Busy", "Ready");
    const unclaimed = await d.createTicket("acme", "Idle", "Ready");
    await d.claim("acme", id, "sess-busy");
    for (const [key, ticket, claim] of [["nope", id, 1], ["acme", "acme-99", 1], ["acme", unclaimed, 1], ["acme", id, 2]] as const) {
      const err = await d.api.transcript(key, ticket, claim).catch((e) => e);
      expect(err, `${key} ${ticket} ${claim}`).toMatchObject({ status: 404, code: "not_found" });
    }
  });

  it("follows a Claim's Transcript: the held events, then live, then on after a dropped stream with no gaps or repeats", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Busy", "Ready");
    await d.claim("acme", id, "sess-busy");
    await d.postSessionEvents("sess-busy", [said("one"), said("two")]);

    const got: HeldSessionEvent[] = [];
    const opens: boolean[] = [];
    let tails = 0;
    const stop = d.api.subscribeTranscript("acme", id, 1, {
      onEvent: (e) => got.push(e),
      onTail: () => tails++,
      onOpen: ({ reconnect }) => opens.push(reconnect),
    });
    try {
      await expect.poll(() => textsOf(got), { timeout: 5000 }).toEqual(["1 one", "2 two"]);
      expect(got[0]).toEqual({ seq: 1, ...said("one") });
      await d.postSessionEvents("sess-busy", [said("three")]);
      await expect.poll(() => textsOf(got), { timeout: 5000 }).toEqual(["1 one", "2 two", "3 three"]);

      d.dropTranscriptStreams();
      await d.postSessionEvents("sess-busy", [said("four"), said("five")]);
      await expect.poll(() => opens, { timeout: 5000 }).toEqual([false, true]);
      await d.postSessionEvents("sess-busy", [said("six")]);
      await expect.poll(() => got.length, { timeout: 5000 }).toBe(6);

      // The stream stays open after the Claim ends: a late Factory line still arrives.
      await d.api.act("acme", id, "release", {});
      await d.postSessionEvents("sess-busy", [{ time: "2026-11-08T10:02:00Z", kind: "factory", text: "Claim ended" }]);
      await expect.poll(() => got.length, { timeout: 5000 }).toBe(7);
      expect(textsOf(got)).toEqual(["1 one", "2 two", "3 three", "4 four", "5 five", "6 six", "7 Claim ended"]);
      expect(tails).toBe(0);
    } finally {
      stop();
    }
  });

  it("sends a tail notice before the events of a Transcript that dropped its earliest", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Busy", "Ready");
    await d.claim("acme", id, "sess-busy");
    await d.postSessionEvents("sess-busy", Array.from({ length: 2002 }, (_, i) => said(`${i + 1}`)));

    const read = await d.api.transcript("acme", id, 1);
    expect([read.tail, read.events.length, read.events[0].seq, read.events.at(-1)!.seq]).toEqual([true, 2000, 3, 2002]);

    const order: string[] = [];
    const stop = d.api.subscribeTranscript("acme", id, 1, {
      onEvent: (e) => order.push(String(e.seq)),
      onTail: () => order.push("tail"),
    });
    try {
      await expect.poll(() => order.length, { timeout: 5000 }).toBe(2001);
      expect(order.slice(0, 3)).toEqual(["tail", "3", "4"]);
      expect(order.at(-1)).toBe("2002");

      // A reconnect misses nothing, so it brings no second notice.
      d.dropTranscriptStreams();
      await d.postSessionEvents("sess-busy", [said("2003")]);
      await expect.poll(() => order.length, { timeout: 5000 }).toBe(2002);
      expect(order.at(-1)).toBe("2003");
    } finally {
      stop();
    }
  });
}
