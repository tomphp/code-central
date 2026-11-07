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
  // reportFailure claims a Ready Ticket as an agent (session
  // "contract-session") and posts a Failure Report, moving it to Failed.
  reportFailure(key: string, id: string, report: FailureReportBody): Promise<void>;
  // finish claims a Ready Ticket as an agent and finishes it with the
  // summary, moving it to In Review.
  finish(key: string, id: string, summary: string): Promise<void>;
  // claim claims a Ready Ticket as an agent with a session ID, moving it
  // to In Progress (ticket 35).
  claim(key: string, id: string, sessionId: string): Promise<void>;
  close(): Promise<void>;
}

export interface FailureReportBody {
  what_broke: string;
  why: string;
  what_to_fix: string;
  where_work_stands: string;
}

export const sampleReport: FailureReportBody = {
  what_broke: "Tests fail on CI\nlogin_test.go times out",
  why: "The fixture server never starts",
  what_to_fix: "Start the fixture in TestMain",
  where_work_stands: "Branch impl/7, two commits, tests red",
};

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

    // ---- Failed and In Review (ticket 36) ----

    it("shows a Failed Ticket's Failure Reports and lists it in Needs Me", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Fix login", "Ready");
      await d.reportFailure("acme", id, sampleReport);

      const t = await d.api.getTicket("acme", id);
      expect(t).toMatchObject({ state: "Failed", needs_me_reason: "failed" });
      expect(t.failure_reports).toHaveLength(1);
      expect(t.failure_reports[0]).toMatchObject({ number: 1, claim: 1, session_id: "contract-session", ...sampleReport });
      const v = await d.api.needsMe();
      expect(v.groups[0]).toMatchObject({ reason: "failed", tickets: [{ id, failure: { headline: "Tests fail on CI", reports: 1 } }] });
    });

    it("sends a Failed Ticket back to Ready, with the note as a human comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Fix login", "Ready");
      await d.reportFailure("acme", id, sampleReport);

      const t = await d.api.act("acme", id, "send_back", { note: "Fixture fixed on main" });
      expect(t).toMatchObject({ id, state: "Ready" });
      expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Fixture fixed on main" });
      expect(t.moves.at(-1)).toMatchObject({ from: "Failed", to: "Ready", actor: "human" });
      expect((await d.api.needsMe()).count).toBe(0);
    });

    it("sends a Failed Ticket back with no note and adds no comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Fix login", "Ready");
      await d.reportFailure("acme", id, sampleReport);
      const before = (await d.api.getTicket("acme", id)).comments.length;

      const t = await d.api.act("acme", id, "send_back", {});
      expect(t.state).toBe("Ready");
      expect(t.comments).toHaveLength(before);
    });

    it("moves a Failed Ticket to Draft", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Fix login", "Ready");
      await d.reportFailure("acme", id, sampleReport);

      expect(await d.api.act("acme", id, "to_draft", {})).toMatchObject({ id, state: "Draft" });
      expect((await d.api.needsMe()).count).toBe(0);
    });

    it("cancels a Failed Ticket", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Fix login", "Ready");
      await d.reportFailure("acme", id, sampleReport);

      const t = await d.api.act("acme", id, "cancel", {});
      expect(t).toMatchObject({ id, state: "Cancelled", replacements: [] });
      expect((await d.api.needsMe()).count).toBe(0);
    });

    it("refuses send_back and to_draft on a Ticket that isn't Failed", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Ready");

      for (const action of ["send_back", "to_draft"] as const) {
        const e = await d.api.act("acme", id, action, {}).catch((e) => e);
        expect(e).toBeInstanceOf(ApiError);
        expect(e).toMatchObject({ status: 409, code: "action_not_allowed" });
      }
    });

    it("shows an In Review Ticket with its finish summary", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page\nBranch impl/7; check the redirect");

      const t = await d.api.getTicket("acme", id);
      expect(t).toMatchObject({ state: "In Review", needs_me_reason: "in_review" });
      const entered = t.moves.at(-1)!;
      expect(entered).toMatchObject({ from: "In Progress", to: "In Review", actor: "agent" });
      expect(t.comments.at(-1)).toMatchObject({
        actor: "agent",
        time: entered.time,
        body: "Added the login page\nBranch impl/7; check the redirect",
      });
      expect((await d.api.needsMe()).groups[0]).toMatchObject({
        reason: "in_review",
        tickets: [{ id, summary: "Added the login page" }],
      });
    });

    it("accepts an In Review Ticket into Done", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page");

      const t = await d.api.act("acme", id, "accept", {});
      expect(t).toMatchObject({ id, state: "Done" });
      expect(t.moves.at(-1)).toMatchObject({ from: "In Review", to: "Done", actor: "human" });
      expect((await d.api.needsMe()).count).toBe(0);
    });

    it("rejects an In Review Ticket back to Ready only with a comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page");

      const blank = await d.api.act("acme", id, "reject", { comment: "  " }).catch((e) => e);
      expect(blank).toBeInstanceOf(ApiError);
      expect(blank).toMatchObject({ status: 422, code: "invalid", field: "comment" });
      expect((await d.api.getTicket("acme", id)).state).toBe("In Review");

      const t = await d.api.act("acme", id, "reject", { comment: "The redirect loops" });
      expect(t).toMatchObject({ id, state: "Ready" });
      expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "The redirect loops" });
      expect((await d.api.needsMe()).count).toBe(0);
    });

    it("refuses accept and reject on a Ticket that isn't In Review", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Ready");

      const a = await d.api.act("acme", id, "accept", {}).catch((e) => e);
      expect(a).toMatchObject({ status: 409, code: "action_not_allowed" });
      const r = await d.api.act("acme", id, "reject", { comment: "no" }).catch((e) => e);
      expect(r).toMatchObject({ status: 409, code: "action_not_allowed" });
    });

    // ---- release (ticket 35) ----
    it("releases a Claim back to Ready, ending it, with the note as a comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Ready");
      await d.claim("acme", id, "sess-1");

      const t = await d.api.act("acme", id, "release", { note: "Session died" });
      expect(t).toMatchObject({ id, state: "Ready" });
      expect(t.claims).toHaveLength(1);
      expect(t.claims[0]).toMatchObject({ session_id: "sess-1", ended_by: "release" });
      expect(t.claims[0].ended).toBeTruthy();
      expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Session died" });
      expect((await d.api.getTicket("acme", id)).state).toBe("Ready");

      const again = await d.api.act("acme", id, "release", {}).catch((e) => e);
      expect(again).toMatchObject({ status: 409, code: "action_not_allowed" });
    });

    it("releases without a note, adding no comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Ready");
      await d.claim("acme", id, "sess-1");

      const t = await d.api.act("acme", id, "release", {});
      expect(t.state).toBe("Ready");
      expect(t.comments).toEqual([]);
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
