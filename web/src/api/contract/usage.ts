import { beforeEach, expect, it } from "vitest";
import type { UsageResponse } from "../types";
import { ApiError } from "../types";
import type { Driver } from "../driver";
import { startedAt } from "./support";

export function usageContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- Usage (Spec code-central-201) ----

  it("reads no Usage before any agent call, with a row of no tokens for every day in the range", async () => {
    await d.createProject("acme", "Acme");
    for (const [u, days] of [
      [await d.api.usage(), 7],
      [await d.api.usage({ project: "acme", days: 90 }), 90],
    ] as const) {
      expect({ ...u, days: [] }).toEqual({ operations: [], kinds: [], sessions: [], days: [] });
      expect(u.days).toHaveLength(days);
      expect(u.days.every((r) => r.kinds.length === 0)).toBe(true);
      // One row a day, oldest first, with no day missed.
      const noon = (day: string) => Date.parse(`${day}T12:00:00Z`) / 86_400_000;
      expect(u.days.map((r) => noon(r.day) - noon(u.days[0].day))).toEqual([...Array(days).keys()]);
    }
  });

  it("counts each agent MCP call by operation, with its sizes and errors, and never the UI's own reads", async () => {
    await d.createProject("acme", "Acme");
    await d.createProject("beta", "Beta");
    const id = await d.createTicket("acme", "Pick a colour", "Ready");
    const found = await d.mcpCall("acme", "find_tickets", {});
    const got = await d.mcpCall("acme", "get_ticket", { ticket: id });
    const missing = await d.mcpCall("acme", "get_ticket", { ticket: "acme-99" });
    const other = await d.mcpCall("beta", "find_tickets", { state: ["Ready"] });
    expect([got.error, missing.error]).toEqual([undefined, "not_found"]);
    await d.api.findTickets("acme");
    await d.api.getTicket("acme", id);

    const bytes = (s: string) => new TextEncoder().encode(s).length;
    const row = (calls: { text: string; error?: string }[], args: unknown[], op: string) => {
      const req = args.reduce<number>((n, a) => n + bytes(JSON.stringify(a)), 0);
      const resp = calls.reduce((n, c) => n + bytes(c.text), 0);
      return {
        op,
        calls: calls.length,
        request_bytes: req,
        average_request_bytes: Math.floor((2 * req + calls.length) / (2 * calls.length)),
        response_bytes: resp,
        average_response_bytes: Math.floor((2 * resp + calls.length) / (2 * calls.length)),
        errors: calls.filter((c) => c.error).length,
      };
    };
    const all = await d.api.usage();
    expect(all.operations).toEqual([
      row([found, other], [{}, { state: ["Ready"] }], "find_tickets"),
      row([got, missing], [{ ticket: id }, { ticket: "acme-99" }], "get_ticket"),
    ]);
    expect((await d.api.usage({ days: 30 })).operations).toEqual(all.operations);

    const acme = await d.api.usage({ project: "acme" });
    expect(acme.operations.map((r) => [r.op, r.calls, r.errors])).toEqual([
      ["get_ticket", 2, 1],
      ["find_tickets", 1, 0],
    ]);
    const beta = await d.api.usage({ project: "beta", days: 7 });
    expect(beta.operations).toEqual([row([other], [{ state: ["Ready"] }], "find_tickets")]);
  });

  it("files posted token usage under the newest Claim with its session, as each kind's peaks and the recent sessions", async () => {
    await d.createProject("acme", "Acme");
    await d.createProject("beta", "Beta");
    const impl = await d.createTicket("acme", "Build it", "Ready");
    const spec = await d.createTicket("beta", "Write it", "Ready");
    await d.claim("acme", impl, "sess-impl");
    await d.claim("beta", spec, "sess-spec");
    // The service's now, from the Claim, so the responses fall in range.
    const base = Date.parse((await d.api.getTicket("acme", impl)).claims![0].started);
    const at = (s: number) => new Date(base + s * 1000).toISOString();
    const resp = (s: number, kind: string, input: number, write: number, read: number, agent?: string): UsageResponse => ({
      time: at(s),
      ...(agent ? { agent } : {}),
      kind,
      input_tokens: input,
      cache_write_tokens: write,
      cache_read_tokens: read,
      output_tokens: 7,
    });
    await d.postUsage("sess-impl", [
      { ...resp(2, "implementation", 5, 1_000, 60_000), model: "claude-sonnet-4-6" },
      { ...resp(1, "implementation", 5, 20_000, 0), model: "claude-opus-4-7" },
      { ...resp(3, "review", 3, 9_000, 0, "toolu_a"), model: "claude-haiku-4-5" },
      resp(4, "custom", 1, 100, 0, "toolu_b"),
    ]);
    await d.postUsage("sess-spec", [resp(5, "spec", 2, 30_000, 0)]);
    await d.postUsage("sess-nobody", [resp(6, "spec", 2, 90_000, 0)]);

    const all = await d.api.usage();
    expect(all.kinds).toEqual([
      { kind: "spec", sessions: 1, average_peak: 30_002, largest_peak: 30_002 },
      { kind: "implementation", sessions: 1, average_peak: 61_005, largest_peak: 61_005 },
      { kind: "review", sessions: 1, average_peak: 9_003, largest_peak: 9_003 },
      { kind: "custom", sessions: 1, average_peak: 101, largest_peak: 101 },
    ]);
    const session = (
      project: string,
      ticket: string,
      session_id: string,
      kind: string,
      started: number,
      peak: number,
      responses: number,
      models: string[],
      tokens: number,
      agent?: string,
    ) => ({
      project,
      ticket,
      claim: 1,
      session_id,
      ...(agent ? { agent } : {}),
      kind,
      started,
      peak,
      responses,
      models,
      tokens,
      subagents: [],
    });
    const sessions = (u: typeof all) => u.sessions.map(startedAt);
    expect(sessions(all)).toEqual([
      { ...session("beta", spec, "sess-spec", "spec", Date.parse(at(5)), 30_002, 1, [], 30_009), total_tokens: 30_009 },
      {
        // Posted out of order: the Opus response came first.
        ...session("acme", impl, "sess-impl", "implementation", Date.parse(at(1)), 61_005, 2, ["claude-opus-4-7", "claude-sonnet-4-6"], 81_024),
        total_tokens: 81_024 + 9_010 + 108,
        // Oldest first, each with its own peak and tokens.
        subagents: [
          session("acme", impl, "sess-impl", "review", Date.parse(at(3)), 9_003, 1, ["claude-haiku-4-5"], 9_010, "toolu_a"),
          session("acme", impl, "sess-impl", "custom", Date.parse(at(4)), 101, 1, [], 108, "toolu_b"),
        ],
      },
    ]);
    expect(sessions(await d.api.usage({ project: "beta", days: 30 }))).toEqual([sessions(all)[0]]);
    expect((await d.api.usage({ project: "acme" })).kinds.map((k) => k.kind)).toEqual(["implementation", "review", "custom"]);
    expect(all.operations).toEqual([]);

    // The day's tokens by kind, in the kinds' order, on the local day they were used.
    const tokens = (kind: string, input: number, write: number, read: number, output: number) => ({
      kind,
      input_tokens: input,
      cache_write_tokens: write,
      cache_read_tokens: read,
      output_tokens: output,
    });
    // The service and the tests share a machine, and so a time zone.
    const pad = (n: number) => String(n).padStart(2, "0");
    const t = new Date(at(1));
    const day = `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())}`;
    expect(all.days.filter((r) => r.kinds.length)).toEqual([
      {
        day,
        kinds: [tokens("spec", 2, 30_000, 0, 7), tokens("implementation", 10, 21_000, 60_000, 14), tokens("review", 3, 9_000, 0, 7), tokens("custom", 1, 100, 0, 7)],
      },
    ]);
    expect((await d.api.usage({ project: "beta", days: 30 })).days.filter((r) => r.kinds.length)).toEqual([{ day, kinds: [tokens("spec", 2, 30_000, 0, 7)] }]);
  });

  // ---- Nested Subagents (Spec code-central-638) ----

  it("names each Subagent by its latest Agent Type and description, and lists a main session whose only responses in the range are its Subagents'", async () => {
    await d.createProject("acme", "Acme");
    const impl = await d.createTicket("acme", "Build it", "Ready");
    await d.claim("acme", impl, "sess-impl");
    const base = Date.parse((await d.api.getTicket("acme", impl)).claims![0].started);
    const at = (s: number) => new Date(base + s * 1000).toISOString();
    const resp = (s: number, kind: string, input: number, sub?: Pick<UsageResponse, "agent" | "agent_type" | "description">): UsageResponse => ({
      time: at(s),
      ...sub,
      kind,
      input_tokens: input,
      cache_write_tokens: 0,
      cache_read_tokens: 0,
      output_tokens: 1,
    });
    await d.postUsage("sess-impl", [
      // The turn started twenty days before: out of the 7 day range.
      { ...resp(-20 * 86_400, "implementation", 500), model: "claude-opus-4-7" },
      resp(2, "subagent", 10, { agent: "toolu_a", agent_type: "Explore", description: "Map it" }),
      resp(3, "subagent", 20, { agent: "toolu_a", description: "Map it again" }),
      // Recorded before Subagents were named.
      resp(4, "subagent", 30, { agent: "toolu_old" }),
      resp(5, "review", 40, { agent: "toolu_review", description: "Review the diff" }),
    ]);

    const sub = (agent: string, kind: string, started: number, peak: number, responses: number, tokens: number, named: Pick<UsageResponse, "agent_type" | "description">) => ({
      project: "acme",
      ticket: impl,
      claim: 1,
      session_id: "sess-impl",
      agent,
      ...named,
      kind,
      started: Date.parse(at(started)),
      peak,
      responses,
      models: [],
      tokens,
      subagents: [],
    });
    const subagents = [
      // Its Agent Type from the response that gave one, its description from the latest.
      sub("toolu_a", "subagent", 2, 20, 2, 10 + 20 + 2, { agent_type: "Explore", description: "Map it again" }),
      sub("toolu_old", "subagent", 4, 30, 1, 31, {}),
      sub("toolu_review", "review", 5, 40, 1, 41, { description: "Review the diff" }),
    ];
    const week = await d.api.usage();
    expect(week.sessions.map(startedAt)).toEqual([
      {
        project: "acme",
        ticket: impl,
        claim: 1,
        session_id: "sess-impl",
        kind: "",
        started: Date.parse(at(2)),
        peak: 0,
        responses: 0,
        models: [],
        tokens: 0,
        total_tokens: 32 + 31 + 41,
        subagents,
      },
    ]);
    // The kinds count only sessions with responses, Subagents as their own.
    expect(week.kinds).toEqual([
      { kind: "review", sessions: 1, average_peak: 40, largest_peak: 40 },
      { kind: "subagent", sessions: 2, average_peak: 25, largest_peak: 30 },
    ]);

    // Over 30 days its own response is in range.
    const month = await d.api.usage({ days: 30 });
    expect(month.sessions.map(startedAt)).toEqual([
      {
        project: "acme",
        ticket: impl,
        claim: 1,
        session_id: "sess-impl",
        kind: "implementation",
        started: Date.parse(at(-20 * 86_400)),
        peak: 500,
        responses: 1,
        models: ["claude-opus-4-7"],
        tokens: 501,
        total_tokens: 501 + 32 + 31 + 41,
        subagents,
      },
    ]);
  });

  it("lists the 50 newest main sessions, each with all its Subagents", async () => {
    await d.createProject("acme", "Acme");
    const first = await d.createTicket("acme", "Ticket 0", "Ready");
    await d.claim("acme", first, "sess-00");
    const base = Date.parse((await d.api.getTicket("acme", first)).claims![0].started);
    const resp = (s: number, agent?: string): UsageResponse => ({
      time: new Date(base + s * 1000).toISOString(),
      ...(agent ? { agent } : {}),
      kind: agent ? "subagent" : "implementation",
      input_tokens: 1,
      cache_write_tokens: 0,
      cache_read_tokens: 0,
      output_tokens: 0,
    });
    for (let i = 0; i < 51; i++) {
      const session = `sess-${String(i).padStart(2, "0")}`;
      if (i > 0) await d.claim("acme", await d.createTicket("acme", `Ticket ${i}`, "Ready"), session);
      await d.postUsage(session, [resp(i), resp(i, "toolu_a"), resp(i, "toolu_b")]);
    }

    const u = await d.api.usage();
    expect(u.sessions.map((s) => s.session_id)).toEqual(Array.from({ length: 50 }, (_, i) => `sess-${String(50 - i).padStart(2, "0")}`));
    expect(u.sessions.every((s) => s.subagents.map((a) => a.agent).join() === "toolu_a,toolu_b")).toBe(true);
    // Every session counts in the kinds, listed or not.
    expect(u.kinds).toEqual([
      { kind: "implementation", sessions: 51, average_peak: 1, largest_peak: 1 },
      { kind: "subagent", sessions: 102, average_peak: 1, largest_peak: 1 },
    ]);
  });

  it("reads one session's responses back in order, with each one's Context Size and token counts", async () => {
    await d.createProject("acme", "Acme");
    const impl = await d.createTicket("acme", "Build it", "Ready");
    await d.claim("acme", impl, "sess-impl");
    const base = Date.parse((await d.api.getTicket("acme", impl)).claims![0].started);
    const at = (s: number) => new Date(base + s * 1000).toISOString();
    const resp = (s: number, write: number, read: number, agent?: string): UsageResponse => ({
      time: at(s),
      ...(agent ? { agent } : {}),
      kind: agent ? "review" : "implementation",
      input_tokens: 5,
      cache_write_tokens: write,
      cache_read_tokens: read,
      output_tokens: 7,
    });
    await d.postUsage("sess-impl", [resp(3, 1_000, 60_000), resp(1, 20_000, 0), resp(2, 9_000, 0, "toolu_a")]);
    await d.postUsage("sess-impl", [resp(4, 500, 9_000, "toolu_a")]);

    const read = async (agent?: string) =>
      (await d.api.sessionUsage({ project: "acme", session: "sess-impl", ...(agent ? { agent } : {}) })).responses.map((r) => ({ ...r, time: Date.parse(r.time) }));
    const want = (s: number, write: number, read: number) => ({
      time: Date.parse(at(s)),
      context_size: 5 + write + read,
      input_tokens: 5,
      cache_write_tokens: write,
      cache_read_tokens: read,
      output_tokens: 7,
    });
    expect(await read()).toEqual([want(1, 20_000, 0), want(3, 1_000, 60_000)]);
    expect(await read("toolu_a")).toEqual([want(2, 9_000, 0), want(4, 500, 9_000)]);
    expect(await d.api.sessionUsage({ project: "acme", session: "nobody" })).toEqual({ responses: [] });

    const unknown = await d.api.sessionUsage({ project: "nope", session: "sess-impl" }).catch((e) => e);
    expect(unknown).toBeInstanceOf(ApiError);
    expect(unknown).toMatchObject({ status: 404, code: "not_found" });
  });

  it("reads a Subagent's session with its latest Agent Type and description (Spec code-central-638)", async () => {
    await d.createProject("acme", "Acme");
    const impl = await d.createTicket("acme", "Build it", "Ready");
    await d.claim("acme", impl, "sess-impl");
    const base = Date.parse((await d.api.getTicket("acme", impl)).claims![0].started);
    const resp = (s: number, sub?: Pick<UsageResponse, "agent" | "agent_type" | "description">): UsageResponse => ({
      time: new Date(base + s * 1000).toISOString(),
      ...sub,
      kind: sub ? "subagent" : "implementation",
      input_tokens: 5,
      cache_write_tokens: 0,
      cache_read_tokens: 0,
      output_tokens: 7,
    });
    await d.postUsage("sess-impl", [
      resp(1),
      resp(2, { agent: "toolu_a", agent_type: "Explore", description: "Map it" }),
      resp(3, { agent: "toolu_a", description: "Map it again" }),
      resp(4, { agent: "toolu_old" }),
    ]);

    const named = async (agent?: string) => {
      const { responses, ...rest } = await d.api.sessionUsage({ project: "acme", session: "sess-impl", ...(agent ? { agent } : {}) });
      return { ...rest, responses: responses.length };
    };
    expect(await named("toolu_a")).toEqual({ agent_type: "Explore", description: "Map it again", responses: 2 });
    expect(await named("toolu_old")).toEqual({ responses: 1 });
    expect(await named()).toEqual({ responses: 1 });
  });

  it("refuses Usage for an unknown Project and any range but 7, 30 or 90 days", async () => {
    const unknown = await d.api.usage({ project: "nope" }).catch((e) => e);
    expect(unknown).toBeInstanceOf(ApiError);
    expect(unknown).toMatchObject({ status: 404, code: "not_found" });

    const bad = await d.api.usage({ days: 14 as never }).catch((e) => e);
    expect(bad).toMatchObject({ status: 422, code: "invalid", field: "days" });
  });
}
