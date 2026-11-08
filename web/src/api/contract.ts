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
import type { ChangeEvent, ExternalState, SessionEvent, State } from "./types";
import { ApiError } from "./types";
import { roundContract } from "./contract.rounds";
import { MOVE_EXCEPT } from "./actions/move";

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
  // summary, moving it to In Review. on names the work's branch and head
  // commit (ticket 71).
  finish(key: string, id: string, summary: string, on?: { branch: string; commit: string }): Promise<void>;
  // addBlocker makes id wait on blocker, as an agent's add_blocker.
  addBlocker(key: string, id: string, blocker: string): Promise<void>;
  // claim claims a Ready Ticket as an agent with a session ID, moving it
  // to In Progress (ticket 35).
  claim(key: string, id: string, sessionId: string): Promise<void>;
  // linkTicket creates a Draft Ticket linked to an external item as an
  // Integration would, and records its external state (ticket 42).
  linkTicket(key: string, title: string, link: { integration: string; id: string; url: string }, external: ExternalState): Promise<string>;
  // postSessionEvents posts a session's Session Events as a Factory would
  // (ticket 109): they go to the newest Claim with that session ID.
  postSessionEvents(sessionId: string, events: SessionEvent[]): Promise<void>;
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

    roundContract(() => d);

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

    it("leaves finished Tickets out of findTickets unless the filter names them", async () => {
      await d.createProject("acme", "Acme");
      const a = await d.createTicket("acme", "A", "Ready");
      const b = await d.createTicket("acme", "B", "Ready");
      await d.api.act("acme", b, "move", { to_state: "Done" });

      expect((await d.api.findTickets("acme")).map((r) => r.id)).toEqual([a]);
      expect((await d.api.findTickets("acme", { state: ["Done", "Cancelled"] })).map((r) => [r.id, r.state])).toEqual([[b, "Done"]]);
    });

    it("archives a Project only once its work is finished, and unarchives it", async () => {
      await d.createProject("acme", "Acme");
      const a = await d.createTicket("acme", "A", "Ready");
      expect((await d.api.listProjects())[0]).toMatchObject({ key: "acme", unfinished: 1, archived: false });

      const early = await d.api.archiveProject("acme").catch((e) => e);
      expect(early).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect(early.message).toContain(a);

      await d.api.act("acme", a, "move", { to_state: "Done" });
      const archived = await d.api.archiveProject("acme");
      expect(archived).toMatchObject({ key: "acme", archived: true });
      expect(Number.isNaN(Date.parse(archived.archived_at!))).toBe(false);
      expect((await d.api.listProjects())[0]).toMatchObject({ key: "acme", unfinished: 0, archived: true });

      // Every write is refused, listing unarchive_project; reads keep working.
      const refused = await d.api.act("acme", a, "move", { to_state: "Ready" }).catch((e) => e);
      expect(refused).toMatchObject({ status: 409, code: "action_not_allowed", allowed_actions: ["unarchive_project"] });
      expect((await d.api.getTicket("acme", a)).state).toBe("Done");

      const back = await d.api.unarchiveProject("acme");
      expect(back.archived).toBe(false);
      expect(back.archived_at).toBeUndefined();
      expect((await d.api.act("acme", a, "move", { to_state: "Ready" })).state).toBe("Ready");
    });

    it("renames a Project's display name, never its key, and refuses a blank one", async () => {
      await d.createProject("acme", "Acme");
      const v = await d.api.renameProject("acme", "  Acme Web ");
      expect(v).toMatchObject({ key: "acme", display_name: "Acme Web" });
      expect((await d.api.listProjects())[0]).toMatchObject({ key: "acme", display_name: "Acme Web" });

      const blank = await d.api.renameProject("acme", " ").catch((e) => e);
      expect(blank).toMatchObject({ status: 422, code: "invalid", field: "display_name" });
      const unknown = await d.api.renameProject("nope", "X").catch((e) => e);
      expect(unknown).toMatchObject({ status: 404, code: "unknown_project" });
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

    // ---- Accepted and land (ticket 71) ----

    it("accepts work on a branch into Accepted, then lands it into Done with the commit", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });

      const a = await d.api.act("acme", id, "accept", {});
      expect(a).toMatchObject({ id, state: "Accepted", branch: "impl-7", commit: "abc123" });
      expect(a.moves.at(-1)).toMatchObject({ from: "In Review", to: "Accepted", actor: "human" });

      const blank = await d.api.act("acme", id, "land", { commit: " " }).catch((e) => e);
      expect(blank).toMatchObject({ status: 422, code: "invalid", field: "commit" });

      const t = await d.api.act("acme", id, "land", { commit: "def456" });
      expect(t).toMatchObject({ id, state: "Done" });
      expect(t.moves.at(-1)).toMatchObject({ from: "Accepted", to: "Done", actor: "human", via: "merge", commit: "def456" });
    });

    it("cancels an Accepted Ticket", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
      await d.api.act("acme", id, "accept", {});

      expect(await d.api.act("acme", id, "cancel", {})).toMatchObject({ id, state: "Cancelled" });
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
      // MOVE_EXCEPT is what the UI leaves out of its Move to list.
      expect(MOVE_EXCEPT).toHaveLength(2);
      for (const to of MOVE_EXCEPT) {
        const err = await d.api.act("acme", id, "move", { to_state: to }).catch((e) => e);
        expect(err).toMatchObject({ status: 422, code: "invalid", field: "to_state" });
      }
      const t = await d.api.act("acme", id, "move", { to_state: "Done" });
      expect(t.state).toBe("Done");
      expect(t.moves.at(-1)).toMatchObject({ from: "Ready", to: "Done", actor: "human" });
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

    // ---- Requirements (ticket 74) ----
    it("submits a Requirement as its Spec and three chained Stage Tickets, waiting for a Factory", async () => {
      await d.createProject("acme", "Acme");
      const r = await d.api.submitRequirement("acme", "Export a Spec as Markdown\nOne file, Tickets included.");
      expect(r.spec).toMatchObject({ title: "Export a Spec as Markdown", body: "Export a Spec as Markdown\nOne file, Tickets included." });
      expect(r.stages.map((t) => [t.labels, t.state])).toEqual([
        [["stage:grilling"], "Ready"],
        [["stage:spec"], "Ready"],
        [["stage:tickets"], "Ready"],
      ]);
      expect(r.stages[1].blocked_by).toEqual([r.stages[0].id]);
      expect(r.stages[2].blocked_by).toEqual([r.stages[1].id]);

      const s = await d.api.getSpec("acme", r.spec.id);
      expect(s).toMatchObject({ id: r.spec.id, title: "Export a Spec as Markdown", finished: false });
      expect(Number.isNaN(Date.parse(s.created))).toBe(false);
      expect(s.tickets.map((t) => t.id)).toEqual(r.stages.map((t) => t.id));
      expect(s.requirement).toEqual({
        stages: [
          { name: "grilling", ticket: r.stages[0].id, state: "Ready" },
          { name: "spec", ticket: r.stages[1].id, state: "Ready" },
          { name: "tickets", ticket: r.stages[2].id, state: "Ready" },
        ],
        build: { done: 0, total: 0 },
        waiting_for_factory: true,
      });
    });

    it("refuses a Requirement with no text", async () => {
      await d.createProject("acme", "Acme");
      const e = await d.api.submitRequirement("acme", "  \n ").catch((e) => e);
      expect(e).toBeInstanceOf(ApiError);
      expect(e).toMatchObject({ status: 422, code: "invalid", field: "text" });
    });

    it("finds only Requirements with requirements: true, finished ones included when asked", async () => {
      await d.createProject("acme", "Acme");
      await d.createTicket("acme", "Not a Requirement", "Ready");
      const r = await d.api.submitRequirement("acme", "Export a Spec");

      const all = await d.api.findSpecs("acme");
      expect(all.length).toBeGreaterThan(1);
      expect(all.find((s) => s.id === r.spec.id)?.requirement?.waiting_for_factory).toBe(true);
      expect(all.filter((s) => s.id !== r.spec.id).every((s) => s.requirement === undefined)).toBe(true);

      const reqs = await d.api.findSpecs("acme", { requirements: true });
      expect(reqs.map((s) => [s.id, s.title, s.finished])).toEqual([[r.spec.id, "Export a Spec", false]]);

      for (const t of r.stages) await d.api.act("acme", t.id, "move", { to_state: "Done" });
      expect(await d.api.findSpecs("acme", { requirements: true })).toEqual([]);
      const delivered = await d.api.findSpecs("acme", { requirements: true, finished: true });
      expect(delivered.map((s) => [s.id, s.finished])).toEqual([[r.spec.id, true]]);
      expect(delivered[0].requirement).toMatchObject({ build: { done: 0, total: 0 }, waiting_for_factory: false });
      expect(delivered[0].requirement!.stages.map((s) => s.state)).toEqual(["Done", "Done", "Done"]);
    });

    // ---- the Integration contract (ticket 42) ----
    it("reads a linked Ticket closed externally, and unlinks it", async () => {
      await d.createProject("acme", "Acme");
      const gh = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7" };
      const id = await d.linkTicket("acme", "Issue", gh, "closed");
      const link = { ...gh, external_state: "closed", detached: false };

      expect(await d.api.getTicket("acme", id)).toMatchObject({ link, needs_me_reason: "closed_externally" });
      const nm = await d.api.needsMe();
      expect(nm.groups.map((g) => g.reason)).toEqual(["closed_externally"]);
      expect(nm.groups[0].tickets[0]).toMatchObject({ id, reason: "closed_externally", link });
      expect((await d.api.findTickets("acme")).find((r) => r.id === id)?.link).toEqual(link);

      const row = await d.api.unlink("acme", id);
      expect(row).toMatchObject({ kind: "ticket", id, link: { ...link, detached: true } });
      const t = await d.api.getTicket("acme", id);
      expect(t.link?.detached).toBe(true);
      expect(t.needs_me_reason).toBeUndefined();
      expect((await d.api.needsMe()).count).toBe(0);

      const plain = await d.createTicket("acme", "Local", "Draft");
      const err = await d.api.unlink("acme", plain).catch((e) => e);
      expect(err).toMatchObject({ status: 422, code: "invalid", field: "target" });
    });

    // ---- the latest Session Event (ticket 109) ----
    it("shows a Claim's latest Session Event, shown short, on its Claim and on the Ticket's rows", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Busy", "Ready");
      const quiet = await d.createTicket("acme", "Quiet", "Ready");
      await d.claim("acme", id, "sess-busy");
      await d.claim("acme", quiet, "sess-quiet");
      const spec = (await d.api.getTicket("acme", id)).spec.id;
      const rowOf = async (t: string) => (await d.api.findTickets("acme")).find((r) => r.id === t)!;
      const specRowOf = async (t: string) => (await d.api.getSpec("acme", spec)).tickets.find((r) => r.id === t)!;

      expect((await d.api.getTicket("acme", id)).claims[0].latest_event).toBeUndefined();
      expect((await rowOf(id)).latest_event).toBeUndefined();

      await d.postSessionEvents("sess-busy", [
        { time: "2026-11-08T10:00:00Z", kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: go test ./..." },
        { time: "2026-11-08T10:00:05Z", kind: "tool_result", tool_use_id: "tu-1", text: "FAIL\nexit 1", is_error: true },
      ]);
      const failed = { kind: "tool_result", text: "Bash failed", time: "2026-11-08T10:00:05Z" };
      expect((await d.api.getTicket("acme", id)).claims[0].latest_event).toEqual(failed);
      expect((await rowOf(id)).latest_event).toEqual(failed);
      expect((await specRowOf(id)).latest_event).toEqual(failed);
      expect((await d.api.getTicket("acme", quiet)).claims[0].latest_event).toBeUndefined();
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
      expect((await d.api.getTicket("acme", id)).claims[0].latest_event).toMatchObject({ kind: "factory" });
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
