// The scenario builder reaches each state through real operations, so
// what it built reads back as Core derives it.
import { describe, expect, it } from "vitest";
import { newWorld } from "./core/core";
import { scenario } from "./scenario";
import type { State } from "./types";

async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  return { w, s };
}

describe("the scenario builder", () => {
  it("makes Tickets under the Project's own Spec unless told another, with the IDs the writes returned", async () => {
    const { w, s } = await acme();
    const a = await s.ticket("acme", { title: "A" });
    const other = await s.spec("acme", { title: "Export" });
    const b = await s.ticket("acme", { title: "B", spec: other, labels: ["ui"] });
    const ta = await w.api.getTicket("acme", a);
    const tb = await w.api.getTicket("acme", b);
    expect(ta.spec).toMatchObject({ title: "A Spec" });
    expect(tb).toMatchObject({ spec: { id: other, title: "Export" }, labels: ["ui"] });
    expect(new Set([ta.spec!.id, a, other, b]).size).toBe(4);
  });

  it("takes a Ticket to every State the way it gets there", async () => {
    const { w, s } = await acme();
    const want: [State, string | undefined][] = [
      ["Draft", undefined],
      ["Ready", undefined],
      ["In Progress", undefined],
      ["Needs Answer", "needs_answer"],
      ["Answered", "answered"],
      ["Failed", "failed"],
      ["In Review", "in_review"],
      ["Accepted", undefined],
      ["Done", undefined],
      ["Cancelled", undefined],
    ];
    for (const [state, reason] of want) {
      const id = await s.ticket("acme", { title: state, state });
      const t = await w.api.getTicket("acme", id);
      expect([t.state, t.needs_me_reason], state).toEqual([state, reason]);
    }
  });

  it("asks a Round and answers it with Core's Question numbers, then the agent picks the answers up", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Pick a colour", state: "Needs Answer" });
    await s.answerRound("acme", id);
    await s.pickUpAnswers("acme", id);
    const second = await s.askRound("acme", id, [
      { title: "Which size?", body: "S or M" },
      { title: "Which font?", body: "Serif or not", recommended: "Sans" },
    ]);
    expect(second).toBe(2);
    expect((await w.api.needsMe()).groups[0].tickets[0]).toMatchObject({ id, round: { number: 2, open: 2, questions: 2 } });

    await s.answerRound("acme", id, ["M"]);
    const t = await w.api.getTicket("acme", id);
    expect(t.state).toBe("Answered");
    expect(t.rounds[1].questions.map((q) => [q.number, q.answer])).toEqual([
      [2, "M"],
      [3, "Sans"],
    ]);
  });

  it("finishes a Ticket with its work, accepts it, and pushes its commit with CI Tracking on", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Login page" });
    await s.accepted("acme", id, { branch: "impl-7", commit: "def456" });
    await s.pushed("acme", id, "def456");
    expect(await w.api.getTicket("acme", id)).toMatchObject({ state: "Accepted", branch: "impl-7", pushed_commit: { commit: "def456" }, ci_result: "running" });
    expect((await w.api.listProjects())[0].ci.tracking).toBe(true);
  });

  it("reaches each CI result through the CI Watcher's runs", async () => {
    const { w, s } = await acme();
    for (const [outcome, result] of [
      ["running", "running"],
      ["passed", "passed"],
      ["failed", "failed"],
    ] as const) {
      const id = await s.ticket("acme", { title: outcome, state: "Accepted" });
      await s.pushed("acme", id, `c-${outcome}`);
      await s.ci("acme", id, outcome, `c-${outcome}`, outcome.length);
      expect((await w.api.getTicket("acme", id)).ci_result, outcome).toBe(result);
    }
  });

  it("runs a Factory session with its Session Events and Usage", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Build it" });
    const at = w.now().toISOString();
    await s.factorySession("acme", id, {
      session: "sess-1",
      events: [{ time: at, kind: "text", text: "Reading the Spec." }],
      usage: [{ time: at, kind: "implementation", model: "claude-opus-5-5", input_tokens: 10, cache_write_tokens: 0, cache_read_tokens: 0, output_tokens: 5 }],
    });
    const t = await w.api.getTicket("acme", id);
    expect(t).toMatchObject({ state: "In Progress", claims: [{ session_id: "sess-1", latest_event: { text: "Reading the Spec." } }] });
    expect((await w.api.transcript("acme", id, 1)).events.map((e) => e.text)).toEqual(["Reading the Spec."]);
    expect((await w.api.usage()).sessions).toMatchObject([{ project: "acme", ticket: id, session_id: "sess-1", models: ["claude-opus-5-5"] }]);
  });
});
