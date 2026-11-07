// The API contract suite: one set of tests run against FakeApi in the fast
// suite (fake.test.ts) and against the real module and a real
// `code-central serve --store <tmpdir>` in the e2e tier (http.e2e.test.ts),
// so the fake can't drift from the service.
//
// A Driver sets up the world the way agents would: the fake's driver edits
// the fake directly, the real one calls agent operations over HTTP. Tests
// use the IDs the driver returns, never literal IDs (the real service
// numbers Specs and Tickets from one counter).
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { Api } from "./api";
import type { ChangeEvent, State } from "./types";
import { ApiError } from "./types";

export interface Driver {
  api: Api;
  createProject(key: string, displayName: string): Promise<void>;
  createTicket(key: string, title: string, state: "Draft" | "Ready"): Promise<string>;
  // postRound claims a Ready Ticket as an agent and posts a Round of
  // Questions, moving it to Needs Answer.
  postRound(key: string, id: string, questions: { title: string; body: string; recommended?: string }[]): Promise<void>;
  // addBlocker makes id wait on blocker, as an agent's add_blocker.
  addBlocker(key: string, id: string, blocker: string): Promise<void>;
  close(): Promise<void>;
}

export function apiContract(name: string, newDriver: () => Promise<Driver>) {
  describe(`API contract: ${name}`, () => {
    let d: Driver;
    beforeEach(async () => {
      d = await newDriver();
    });
    afterEach(async () => {
      await d.close();
    });

    it("lists every Project with its Needs Me count", async () => {
      await d.createProject("beta", "Beta");
      await d.createProject("acme", "Acme Corp");
      const id = await d.createTicket("acme", "Pick a colour", "Ready");
      await d.postRound("acme", id, [{ title: "Which colour?", body: "Red or blue", recommended: "Blue" }]);

      const rows = await d.api.listProjects();
      expect(rows.map((r) => [r.key, r.display_name, r.needs_me, r.archived])).toEqual([
        ["acme", "Acme Corp", 1, false],
        ["beta", "Beta", 0, false],
      ]);
    });

    it("reads Needs Me: empty, then grouped with each row's detail", async () => {
      expect(await d.api.needsMe()).toEqual({ count: 0, groups: [] });

      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Pick a colour", "Ready");
      await d.postRound("acme", id, [
        { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
        { title: "Which size?", body: "S or M" },
      ]);

      const v = await d.api.needsMe();
      expect(v.count).toBe(1);
      expect(v.groups.map((g) => g.reason)).toEqual(["needs_answer"]);
      const row = v.groups[0].tickets[0];
      expect(row).toMatchObject({
        project: "acme",
        id,
        title: "Pick a colour",
        state: "Needs Answer",
        reason: "needs_answer",
        round: { number: 1, open: 2, questions: 2 },
      });
      expect(Number.isNaN(Date.parse(row.since))).toBe(false);
    });

    it("finds a Project's Tickets, optionally by State", async () => {
      await d.createProject("acme", "Acme");
      const a = await d.createTicket("acme", "A", "Draft");
      const b = await d.createTicket("acme", "B", "Ready");

      const all = await d.api.findTickets("acme");
      expect(all.map((r) => [r.id, r.title, r.state]).sort()).toEqual([
        [a, "A", "Draft"],
        [b, "B", "Ready"],
      ]);
      const ready = await d.api.findTickets("acme", { state: ["Ready"] });
      expect(ready.map((r) => r.id)).toEqual([b]);
      const either = await d.api.findTickets("acme", { state: ["Ready", "Draft"] as State[] });
      expect(either).toHaveLength(2);
    });

    it("gets one Ticket in full, with its Needs Me reason", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Pick a colour", "Ready");
      await d.postRound("acme", id, [{ title: "Which colour?", body: "Red or blue", recommended: "Blue" }]);

      const t = await d.api.getTicket("acme", id);
      expect(t).toMatchObject({ id, title: "Pick a colour", state: "Needs Answer", needs_me_reason: "needs_answer" });
      expect(t.rounds).toHaveLength(1);
      expect(t.rounds[0].questions[0]).toMatchObject({ number: 1, title: "Which colour?", recommended: "Blue", status: "open" });
      expect(Array.isArray(t.labels) && Array.isArray(t.moves) && Array.isArray(t.comments)).toBe(true);
    });

    it("rejects with the one error shape", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Draft");

      const unknown = await d.api.getTicket("nope", "nope-1").catch((e) => e);
      expect(unknown).toBeInstanceOf(ApiError);
      expect(unknown).toMatchObject({ status: 404, code: "unknown_project" });
      expect(unknown.message).not.toBe("");

      const missing = await d.api.getTicket("acme", "acme-99").catch((e) => e);
      expect(missing).toMatchObject({ status: 404, code: "not_found" });

      const same = await d.api.act("acme", id, "move", { to_state: "Draft" }).catch((e) => e);
      expect(same).toMatchObject({ status: 422, code: "invalid", field: "to_state" });
    });

    it("runs a human action on a Ticket", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Draft");

      const t = await d.api.act("acme", id, "move", { to_state: "Ready" });
      expect(t).toMatchObject({ id, state: "Ready" });
      expect((await d.api.getTicket("acme", id)).state).toBe("Ready");
    });

    // ---- Blockers, move and cancel (ticket 37) ----

    it("removes a Blocker, and rejects one that isn't there", async () => {
      await d.createProject("acme", "Acme");
      const a = await d.createTicket("acme", "A", "Ready");
      const b = await d.createTicket("acme", "B", "Ready");
      await d.addBlocker("acme", b, a);
      expect(await d.api.getTicket("acme", b)).toMatchObject({ blocked_by: [a], blocked: true });

      const t = await d.api.act("acme", b, "remove_blocker", { blocker: a });
      expect(t).toMatchObject({ id: b, blocked_by: [], blockers: [], blocked: false });

      const again = await d.api.act("acme", b, "remove_blocker", { blocker: a }).catch((e) => e);
      expect(again).toMatchObject({ status: 422, code: "invalid", field: "blocker" });
    });

    it("cancels as the human with no Replacement, leaving its dependant blocked by a Cancelled Ticket", async () => {
      await d.createProject("acme", "Acme");
      const a = await d.createTicket("acme", "A", "Ready");
      const b = await d.createTicket("acme", "B", "Ready");
      await d.addBlocker("acme", b, a);

      const t = await d.api.act("acme", a, "cancel", {});
      expect(t).toMatchObject({ id: a, state: "Cancelled", replacements: [] });
      expect(t.moves.at(-1)).toMatchObject({ from: "Ready", to: "Cancelled", actor: "human" });
      const dep = await d.api.getTicket("acme", b);
      expect(dep).toMatchObject({ blocked: true, blockers: [{ id: a, title: "A", state: "Cancelled" }] });
    });

    it("moves to any State but Needs Answer and Failed", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Ready");
      for (const to of ["Failed", "Needs Answer"] as State[]) {
        const err = await d.api.act("acme", id, "move", { to_state: to }).catch((e) => e);
        expect(err).toMatchObject({ status: 422, code: "invalid", field: "to_state" });
      }
      const t = await d.api.act("acme", id, "move", { to_state: "Done" });
      expect(t.state).toBe("Done");
      expect(t.moves.at(-1)).toMatchObject({ from: "Ready", to: "Done", actor: "human" });
    });

    it("delivers a change notice for each write over the live feed", async () => {
      const got: ChangeEvent[] = [];
      let opened!: () => void;
      const open = new Promise<void>((r) => (opened = r));
      const stop = d.api.subscribe({ onEvent: (e) => got.push(e), onOpen: () => opened() });
      try {
        await open;
        await d.createProject("acme", "Acme");
        const id = await d.createTicket("acme", "A", "Draft");
        await d.api.act("acme", id, "move", { to_state: "Ready" });

        await expect.poll(() => got.filter((e) => e.type === "ticket.changed").length, { timeout: 5000 }).toBe(2);
        expect(got[0]).toEqual({ type: "project.changed", project: "acme" });
        expect(got.filter((e) => e.type === "ticket.changed")).toEqual([
          { type: "ticket.changed", project: "acme", id },
          { type: "ticket.changed", project: "acme", id },
        ]);
      } finally {
        stop();
      }
    });
  });
}
