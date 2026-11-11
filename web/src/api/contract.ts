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
import type { ChangeEvent, CIProvider, CIRunView, CIWatcherReport, ExternalState, HeartbeatReport, HeldSessionEvent, LandingMethod, ModelsReport, PullRequestReport, SessionEvent, State, TicketAcknowledgement, UsageResponse } from "./types";
import { ApiError, TRIAGE_ACTIONS } from "./types";
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
  // incomingIssue creates a Draft Ticket on spec linked to a GitHub issue,
  // as the Issue Watcher's create_linked_ticket does (code-central-287).
  incomingIssue(key: string, spec: string, title: string, link: { integration: string; id: string; url: string }): Promise<string>;
  // postSessionEvents posts a session's Session Events as a Factory would
  // (ticket 109): they go to the newest Claim with that session ID.
  postSessionEvents(sessionId: string, events: SessionEvent[]): Promise<void>;
  // mcpCall makes one agent MCP tool call to key's Project, as Claude Code
  // would, and returns the text the agent was given and, for a rejected
  // call, its error code. The fake's driver plays find_tickets and
  // get_ticket only (Spec code-central-201).
  mcpCall(key: string, op: "find_tickets" | "get_ticket", args: Record<string, unknown>): Promise<{ text: string; error?: string }>;
  // postUsage posts a session's model responses' token usage as a Factory
  // would (Spec code-central-201): they go to the newest Claim with that
  // session ID.
  postUsage(sessionId: string, responses: UsageResponse[]): Promise<void>;
  // dropTranscriptStreams drops every open Transcript stream, as a flaky
  // connection would; each reconnects by itself (ticket 110).
  dropTranscriptStreams(): void;
  // reportPushRemote reports the Push remote's URL as a Factory does when
  // it starts (code-central-251).
  reportPushRemote(key: string, url: string): Promise<void>;
  // reportModels reports what the Factory's claude accepts, with its
  // flags, as a Factory does when it starts (code-central-311).
  reportModels(key: string, report: ModelsReport): Promise<void>;
  // reportHeartbeat sends a Factory's Heartbeat (code-central-324).
  reportHeartbeat(key: string, report: HeartbeatReport): Promise<void>;
  // logOutOfGH makes the gh the service finds logged out, so the CI check
  // refuses turning CI Tracking on (code-central-252).
  logOutOfGH(): Promise<void>;
  // letGHTriage makes the gh the service finds able to triage
  // repository's issues, so the Issues check passes (code-central-284).
  letGHTriage(repository: string): Promise<void>;
  // letGHWrite makes the gh the service finds able to write to
  // repository, so the Pull Request check passes on GitHub
  // (code-central-389).
  letGHWrite(repository: string): Promise<void>;
  // reportCIWatcherStatus reports the CI Watcher's status as the CI
  // Watcher does on each poll (code-central-253).
  reportCIWatcherStatus(key: string, status: CIWatcherReport): Promise<void>;
  // reportPushedCommit reports the commit a Factory pushed for an
  // Accepted Ticket, and reportCIRuns the CI Runs the CI Watcher saw
  // (code-central-256).
  // The branch it was pushed as is what the Pull Request Watcher opens a
  // pull request from; under Pull Request a push is recorded with CI
  // Tracking off too (code-central-388).
  reportPushedCommit(key: string, id: string, commit: string, branch?: string): Promise<void>;
  // reportPullRequest reports the pull request of the work pushed as
  // branch, as the Pull Request Watcher does; it is recorded on every
  // Ticket the work covers (code-central-388).
  reportPullRequest(key: string, report: PullRequestReport): Promise<void>;
  reportCIRuns(key: string, runs: Omit<CIRunView, "ignored">[]): Promise<void>;
  close(): Promise<void>;
}

export interface FailureReportBody {
  what_broke: string;
  why: string;
  what_to_fix: string;
  where_work_stands: string;
}

// wantAcknowledgement checks a human action's result is its Ticket's
// acknowledgement (ADR 0009): the ID, updated time, the names of the
// fields it changed (changed among them) and the State, and no record. An
// operation's next_step may ride along.
export function wantAcknowledgement(ack: TicketAcknowledgement, id: string, state: State, changed: string[] = []) {
  expect(Object.keys(ack).filter((k) => k !== "edit" && k !== "next_step").sort()).toEqual(["changed", "id", "state", "updated"]);
  expect(ack).toMatchObject({ id, state });
  expect(Number.isNaN(Date.parse(ack.updated))).toBe(false);
  expect(ack.changed).toEqual(expect.arrayContaining(changed));
  expect(ack.changed).not.toContain("updated");
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
      expect(rows.map((r) => r.landing)).toEqual([
        { method: "merge", push: false, remote: "origin", land_per_spec: true, code_host: "", repository: "", pat_env: "ADO_PAT", watcher: { state: "not_reported" }, open_pull_requests: [] },
        { method: "merge", push: false, remote: "origin", land_per_spec: true, code_host: "", repository: "", pat_env: "ADO_PAT", watcher: { state: "not_reported" }, open_pull_requests: [] },
      ]);
      const off = {
        tracking: false,
        provider: "github_actions",
        repository: "",
        project_url: "",
        pat_env: "ADO_PAT",
        ignored_workflows: [],
        unblock_on_push: false,
        push_remote_url: "",
        rules: [],
        watcher: { state: "not_reported" },
      };
      expect(rows.map((r) => r.ci)).toEqual([off, off]);
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
      wantAcknowledgement(await d.api.act("acme", a, "move", { to_state: "Ready" }), a, "Ready", ["state", "moves"]);
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

    it("sets a Project's Landing Method, trimming the remote and refusing an empty one under Push", async () => {
      await d.createProject("acme", "Acme");
      const v = await d.api.setLandingMethod("acme", { method: "branch", push: true, remote: " upstream " });
      expect(v).toMatchObject({ key: "acme", landing: { method: "branch", push: true, remote: "upstream" } });
      expect((await d.api.listProjects())[0].landing).toEqual({ method: "branch", push: true, remote: "upstream", land_per_spec: true, code_host: "", repository: "", pat_env: "ADO_PAT", watcher: { state: "not_reported" }, open_pull_requests: [] });

      // Without Push, an empty remote reads as origin.
      expect((await d.api.setLandingMethod("acme", { method: "merge", push: false, remote: "" })).landing).toEqual({
        method: "merge",
        push: false,
        remote: "origin",
        land_per_spec: true,
        code_host: "",
        repository: "",
        pat_env: "ADO_PAT",
        watcher: { state: "not_reported" },
        open_pull_requests: [],
      });

      const empty = await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: " " }).catch((e) => e);
      expect(empty).toMatchObject({ status: 422, code: "invalid", field: "remote" });
      expect(empty.message).toContain("Push needs a git remote");
      const method = await d.api.setLandingMethod("acme", { method: "squash" as LandingMethod, push: false, remote: "origin" }).catch((e) => e);
      expect(method).toMatchObject({ status: 422, code: "invalid", field: "method" });
    });

    it("sets Pull Request once the Push remote URL names a Code Host, ignoring Push, and refuses it before", async () => {
      await d.createProject("acme", "Acme");
      const none = await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "" }).catch((e) => e);
      expect(none).toMatchObject({ status: 422, code: "invalid", field: "method" });
      expect(none.fix).toContain("start the Factory");

      await d.reportPushRemote("acme", "git@gitlab.com:acme/web.git");
      const unsupported = await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "" }).catch((e) => e);
      expect(unsupported).toMatchObject({ status: 422, code: "invalid", field: "method" });

      // On GitHub: turning it on on Azure Repos checks the PAT against Azure DevOps itself, which a test can't reach.
      await d.reportPushRemote("acme", "git@github.com:acme/web.git");
      await d.letGHWrite("acme/web");
      const v = await d.api.setLandingMethod("acme", { method: "pull_request", push: true, remote: "", pat_env: " REPOS_PAT " });
      expect(v.landing).toMatchObject({ method: "pull_request", push: false, code_host: "github", repository: "acme/web", pat_env: "REPOS_PAT" });
      const kept = await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "" });
      expect(kept.landing.pat_env).toBe("REPOS_PAT");
      const bad = await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "", pat_env: "not a name" }).catch((e) => e);
      expect(bad).toMatchObject({ status: 422, field: "pat_env" });
    });

    it("keeps Land per Spec on by default, sets it, and keeps it when a later set leaves it out", async () => {
      await d.createProject("acme", "Acme");
      expect((await d.api.listProjects())[0].landing.land_per_spec).toBe(true);
      const off = await d.api.setLandingMethod("acme", { method: "merge", push: false, remote: "", land_per_spec: false });
      expect(off.landing.land_per_spec).toBe(false);
      // Left out, the stored value stays, and it is shown whatever the method.
      const kept = await d.api.setLandingMethod("acme", { method: "branch", push: false, remote: "" });
      expect(kept.landing).toMatchObject({ method: "branch", land_per_spec: false });
      const on = await d.api.setLandingMethod("acme", { method: "merge", push: false, remote: "", land_per_spec: true });
      expect(on.landing.land_per_spec).toBe(true);
    });

    it("refuses turning CI Tracking on when the CI check fails, with its reason and fix", async () => {
      await d.createProject("acme", "Acme");
      await d.logOutOfGH();
      const refused = await d.api
        .setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false })
        .catch((e) => e);
      expect(refused).toBeInstanceOf(ApiError);
      expect(refused).toMatchObject({ status: 422, code: "invalid", field: "tracking" });
      expect(refused.message).toContain("isn't logged in");
      expect(refused.fix).toContain("gh auth login");
      expect((await d.api.listProjects())[0].ci.tracking).toBe(false);
    });

    it("sets a Project's GitHub Issues settings, off by default and checked when turned on", async () => {
      await d.createProject("acme", "Acme");
      const off = { on: false, repository: "", label: "", new_issues_need_me: false };
      const notReported = { state: "not_reported" };
      expect((await d.api.listProjects())[0].github_issues).toEqual({ ...off, watcher: notReported });

      const none = await d.api.setGitHubIssues("acme", { ...off, on: true, repository: " " }).catch((e) => e);
      expect(none).toMatchObject({ status: 422, code: "invalid", field: "repository" });
      expect(none.message).toContain("owner/name");
      const url = await d.api.setGitHubIssues("acme", { ...off, repository: "https://github.com/acme/web" }).catch((e) => e);
      expect(url).toMatchObject({ status: 422, code: "invalid", field: "repository" });

      // Off, nothing is checked and no Issues Spec is made.
      await d.logOutOfGH();
      const kept = await d.api.setGitHubIssues("acme", { on: false, repository: " acme/web ", label: " bug ", new_issues_need_me: true });
      expect(kept.github_issues).toEqual({ on: false, repository: "acme/web", label: "bug", new_issues_need_me: true, watcher: notReported });

      const refused = await d.api.setGitHubIssues("acme", { ...kept.github_issues, on: true }).catch((e) => e);
      expect(refused).toBeInstanceOf(ApiError);
      expect(refused).toMatchObject({ status: 422, code: "invalid", field: "on" });
      expect(refused.message).toContain("isn't logged in");
      expect(refused.fix).toContain("gh auth login");
      expect((await d.api.listProjects())[0].github_issues).toEqual(kept.github_issues);
    });

    it("sets a Project's Models, a row per kind of work, all at Default to begin with", async () => {
      await d.createProject("acme", "Acme");
      const kinds = ["grilling", "spec", "tickets", "map-chart", "map-grilling", "map-research", "map-task", "prototype", "implementation"];
      const unset = { model: "", effort: "" };
      expect((await d.api.listProjects())[0].models).toEqual({ default: unset, kinds: kinds.map((kind) => ({ kind, ...unset })) });

      const set = await d.api.setModels("acme", {
        default: { model: " opus ", effort: "high" },
        kinds: [{ kind: "implementation", model: "", effort: "xhigh" }],
      });
      expect(set.models.default).toEqual({ model: "opus", effort: "high" });
      expect(set.models.kinds.map((k) => k.kind)).toEqual(kinds);
      expect(set.models.kinds.at(-1)).toEqual({ kind: "implementation", model: "", effort: "xhigh" });
      expect((await d.api.listProjects())[0].models).toEqual(set.models);

      const effort = await d.api.setModels("acme", { default: unset, kinds: [{ kind: "spec", model: "", effort: "extreme" }] }).catch((e) => e);
      expect(effort).toBeInstanceOf(ApiError);
      expect(effort).toMatchObject({ status: 422, code: "invalid", field: "kinds.spec.effort" });
      expect(effort.message).toContain("low, medium, high, xhigh, max");
      const blank = await d.api.setModels("acme", { default: { model: " ", effort: "" }, kinds: [] }).catch((e) => e);
      expect(blank).toMatchObject({ status: 422, code: "invalid", field: "default.model" });
      const other = await d.api.setModels("acme", { default: unset, kinds: [{ kind: "review", model: "opus", effort: "" }] }).catch((e) => e);
      expect(other).toMatchObject({ status: 422, code: "invalid", field: "kinds" });
      expect((await d.api.listProjects())[0].models).toEqual(set.models);

      // The rows are replaced whole.
      const cleared = await d.api.setModels("acme", { default: unset, kinds: [] });
      expect(cleared.models).toEqual({ default: unset, kinds: kinds.map((kind) => ({ kind, ...unset })) });
    });

    it("serves factory_status: offline with no Factories, then each Factory from its Heartbeat", async () => {
      await d.createProject("acme", "Acme");
      expect((await d.api.listProjects())[0].factory_status).toEqual({ status: "offline", factories: [] });

      await d.reportHeartbeat("acme", { factory: "code-central-factory", host: "laptop", state: "online", note: "1 of 3 turns running", interval_seconds: 30 });
      const online = (await d.api.listProjects())[0].factory_status;
      expect(online.status).toBe("online");
      expect(online.factories).toEqual([
        { name: "code-central-factory", host: "laptop", status: "online", note: "1 of 3 turns running", last_heartbeat: expect.any(String), interval_seconds: 30 },
      ]);

      await d.reportHeartbeat("acme", { factory: "code-central-factory", host: "laptop", state: "stopping", note: "stopped: drained", interval_seconds: 30 });
      const stopped = (await d.api.listProjects())[0].factory_status;
      expect(stopped.status).toBe("offline");
      expect(stopped.factories).toMatchObject([{ status: "offline", note: "stopped: drained" }]);
    });

    it("keeps the Factory's report of what its claude accepts, and checks effort against its levels", async () => {
      await d.createProject("acme", "Acme");
      const none = { effort_levels: ["low", "medium", "high", "xhigh", "max"], aliases: [], model_ids: [], model_flag: "", effort_flag: "" };
      expect((await d.api.listProjects())[0].reported_models).toEqual(none);

      await d.reportModels("acme", { effort_levels: ["low", "high", "turbo"], aliases: ["opus", "sonnet"], model_flag: "opus", effort_flag: "" });
      const kept = (await d.api.listProjects())[0].reported_models;
      expect(kept).toMatchObject({ effort_levels: ["low", "high", "turbo"], aliases: ["opus", "sonnet"], model_ids: [], model_flag: "opus", effort_flag: "" });
      expect(kept.reported).toEqual(expect.any(String));

      // Found nothing, the kept levels and aliases stay; the flags are replaced.
      await d.reportModels("acme", { effort_levels: [], aliases: [], model_flag: "", effort_flag: "high" });
      expect((await d.api.listProjects())[0].reported_models).toMatchObject({ effort_levels: ["low", "high", "turbo"], aliases: ["opus", "sonnet"], model_flag: "", effort_flag: "high" });

      const max = await d.api.setModels("acme", { default: { model: "", effort: "max" }, kinds: [] }).catch((e) => e);
      expect(max).toMatchObject({ status: 422, code: "invalid", field: "default.effort" });
      expect(max.message).toContain("low, high, turbo");
      expect((await d.api.setModels("acme", { default: { model: "", effort: "turbo" }, kinds: [] })).models.default.effort).toBe("turbo");

      const twoLines = await d.reportModels("acme", { effort_levels: [], aliases: ["opus\nsonnet"], model_flag: "", effort_flag: "" }).catch((e) => e);
      expect(twoLines).toBeInstanceOf(Error);
      expect((await d.api.listProjects())[0].reported_models.aliases).toEqual(["opus", "sonnet"]);
    });

    it("reads a Ticket with the model and effort its next turn runs with, each with its source, and checks an effort Tag", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Pick a colour", "Ready");
      expect((await d.api.getTicket("acme", id)).model_choice).toEqual({ model: { source: "none" }, effort: { source: "none" } });

      await d.api.setModels("acme", { default: { model: "sonnet", effort: "low" }, kinds: [{ kind: "implementation", model: "opus", effort: "" }] });
      expect((await d.api.getTicket("acme", id)).model_choice).toEqual({
        model: { value: "opus", source: "project_kind", kind: "implementation" },
        effort: { value: "low", source: "project_default" },
      });

      await d.api.setTags("acme", id, { effort: "max" });
      expect((await d.api.getTicket("acme", id)).model_choice?.effort).toEqual({ value: "max", source: "ticket_tag" });
      const bad = await d.api.setTags("acme", id, { effort: "extreme" }).catch((e) => e);
      expect(bad).toBeInstanceOf(ApiError);
      expect(bad).toMatchObject({ status: 422, code: "invalid", field: "tags" });
      expect(bad.message).toContain("low, medium, high, xhigh, max");
      await d.api.removeTags("acme", id, ["effort"]);
      expect((await d.api.getTicket("acme", id)).model_choice?.effort).toEqual({ value: "low", source: "project_default" });

      // Agents' get_ticket leaves it out.
      expect((await d.mcpCall("acme", "get_ticket", { ticket: id })).text).not.toContain("model_choice");
    });

    it("creates the Issues Spec the first time GitHub Issues is turned on, retitled with the repository", async () => {
      await d.createProject("acme", "Acme");
      await d.letGHTriage("acme/web");
      const on = await d.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "", new_issues_need_me: false });
      // The service's own Issue Watcher may report by the time it reads.
      expect(on.github_issues).toEqual({
        on: true,
        repository: "acme/web",
        label: "",
        new_issues_need_me: false,
        spec: expect.stringMatching(/^acme-\d+$/),
        watcher: expect.anything(),
      });
      const spec = on.github_issues.spec!;
      const s = await d.api.getSpec("acme", spec);
      expect(s.title).toBe("GitHub Issues: acme/web");
      expect(s.link).toBeUndefined();

      await d.api.setGitHubIssues("acme", { on: false, repository: "acme/web", label: "", new_issues_need_me: false });
      expect((await d.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "", new_issues_need_me: false })).github_issues.spec).toBe(spec);

      await d.letGHTriage("acme/site");
      await d.api.setGitHubIssues("acme", { on: true, repository: "acme/site", label: "", new_issues_need_me: false });
      expect((await d.api.getSpec("acme", spec)).title).toBe("GitHub Issues: acme/site");
    });

    it("adds, edits and removes CI Rules, each with its own Spec", async () => {
      await d.createProject("acme", "Acme");
      const added = await d.api.addCIRule("acme", { workflow: " ci.yml ", branch: " main ", state: "Ready" });
      expect(added.ci.rules).toEqual([{ spec: expect.stringMatching(/^acme-\d+$/), workflow: "ci.yml", branch: "main", state: "Ready", created: expect.any(String) }]);
      const first = added.ci.rules[0].spec;
      expect((await d.api.getSpec("acme", first)).title).toBe("CI: ci.yml on main");
      const second = (await d.api.addCIRule("acme", { workflow: "any", branch: "main", state: "Draft" })).ci.rules[1].spec;
      expect((await d.api.getSpec("acme", second)).title).toBe("CI: any workflow on main");
      expect((await d.api.listProjects())[0].ci.rules.map((r) => r.spec)).toEqual([first, second]);

      const edited = await d.api.editCIRule("acme", first, { workflow: "lint.yml", branch: "develop", state: "Draft" });
      expect(edited.ci.rules[0]).toEqual({ ...added.ci.rules[0], workflow: "lint.yml", branch: "develop", state: "Draft" });
      expect((await d.api.getSpec("acme", first)).title).toBe("CI: lint.yml on develop");

      const removed = await d.api.removeCIRule("acme", first);
      expect(removed.ci.rules.map((r) => r.spec)).toEqual([second]);
      expect((await d.api.getSpec("acme", first)).title).toBe("CI: lint.yml on develop");
      expect(await d.api.removeCIRule("acme", first).catch((e) => e)).toMatchObject({ status: 404, code: "not_found" });

      const twice = await d.api.addCIRule("acme", { workflow: "any", branch: "main", state: "Ready" }).catch((e) => e);
      expect(twice).toMatchObject({ status: 422, code: "invalid" });
      expect(twice.message).toContain(second);
      const glob = await d.api.addCIRule("acme", { workflow: "ci.yml", branch: "release/*", state: "Draft" }).catch((e) => e);
      expect(glob).toMatchObject({ status: 422, code: "invalid", field: "branch" });
      const path = await d.api.addCIRule("acme", { workflow: ".github/workflows/ci.yml", branch: "main", state: "Draft" }).catch((e) => e);
      expect(path).toMatchObject({ status: 422, code: "invalid", field: "workflow" });
    });

    it("sets a Project's Azure Pipelines settings, validating them by provider", async () => {
      await d.createProject("acme", "Acme");
      const azure = { provider: "azure_pipelines" as CIProvider, repository: "", ignored_workflows: [], unblock_on_push: false };

      const none = await d.api.setCITracking("acme", { ...azure, tracking: true, project_url: " " }).catch((e) => e);
      expect(none).toMatchObject({ status: 422, code: "invalid", field: "project_url" });
      expect(none.message).toBe("CI Tracking needs the Azure DevOps project to watch, as its URL");
      const bad = await d.api.setCITracking("acme", { ...azure, tracking: false, project_url: "https://dev.azure.com/acme" }).catch((e) => e);
      expect(bad).toMatchObject({ status: 422, code: "invalid", field: "project_url" });
      const env = await d.api.setCITracking("acme", { ...azure, tracking: false, pat_env: "MY-PAT" }).catch((e) => e);
      expect(env).toMatchObject({ status: 422, code: "invalid", field: "pat_env" });
      const names = await d.api.setCITracking("acme", { ...azure, tracking: false, ignored_workflows: ["a/b"] }).catch((e) => e);
      expect(names).toMatchObject({ status: 422, code: "invalid", field: "ignored_workflows" });

      // The project URL is cut back to the project, and a pipeline's name may hold spaces.
      // Tracking stays off: turning it on checks the PAT against Azure DevOps itself, which a test can't reach.
      const v = await d.api.setCITracking("acme", { ...azure, tracking: false, project_url: " https://dev.azure.com/acme/web/_build?definitionId=3", ignored_workflows: ["Nightly build"] });
      expect(v.ci).toMatchObject({ tracking: false, provider: "azure_pipelines", project_url: "https://dev.azure.com/acme/web", pat_env: "ADO_PAT", ignored_workflows: ["Nightly build"] });

      // Each provider keeps its own fields when the provider is switched.
      const back = await d.api.setCITracking("acme", { ...azure, provider: "github_actions", tracking: false, repository: "acme/web", project_url: "https://dev.azure.com/acme/web", pat_env: "ACME_PAT" });
      expect(back.ci).toMatchObject({ provider: "github_actions", repository: "acme/web", project_url: "https://dev.azure.com/acme/web", pat_env: "ACME_PAT" });
      const unknown = await d.api.setCITracking("acme", { ...azure, provider: "circleci" as CIProvider, tracking: false }).catch((e) => e);
      expect(unknown.message).toContain("azure_pipelines");
    });

    it("sets a Project's CI settings, keeping the Push remote URL the Factory reported", async () => {
      await d.createProject("acme", "Acme");
      await d.reportPushRemote("acme", "git@github.com:acme/web.git");
      expect((await d.api.listProjects())[0].ci.push_remote_url).toBe("git@github.com:acme/web.git");

      const v = await d.api.setCITracking("acme", {
        tracking: true,
        provider: "github_actions",
        repository: " acme/web ",
        ignored_workflows: [" nightly.yml", "", "nightly.yml", "deploy.yaml"],
        unblock_on_push: true,
      });
      const on = {
        tracking: true,
        provider: "github_actions",
        repository: "acme/web",
        project_url: "",
        pat_env: "ADO_PAT",
        ignored_workflows: ["nightly.yml", "deploy.yaml"],
        unblock_on_push: true,
        push_remote_url: "git@github.com:acme/web.git",
        rules: [],
        watcher: { state: "not_reported" },
      };
      expect(v).toMatchObject({ key: "acme", ci: on });
      expect((await d.api.listProjects())[0].ci).toEqual(on);

      const none = await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: " ", ignored_workflows: [], unblock_on_push: false }).catch((e) => e);
      expect(none).toMatchObject({ status: 422, code: "invalid", field: "repository" });
      expect(none.message).toContain("owner/name");
      const url = await d.api
        .setCITracking("acme", { tracking: true, provider: "github_actions", repository: "https://github.com/acme/web", ignored_workflows: [], unblock_on_push: false })
        .catch((e) => e);
      expect(url).toMatchObject({ status: 422, code: "invalid", field: "repository" });
      const path = await d.api
        .setCITracking("acme", { tracking: false, provider: "github_actions", repository: "", ignored_workflows: [".github/workflows/ci.yml"], unblock_on_push: false })
        .catch((e) => e);
      expect(path).toMatchObject({ status: 422, code: "invalid", field: "ignored_workflows" });
      const provider = await d.api
        .setCITracking("acme", { tracking: false, provider: "circleci" as CIProvider, repository: "", ignored_workflows: [], unblock_on_push: false })
        .catch((e) => e);
      expect(provider).toMatchObject({ status: 422, code: "invalid", field: "provider" });

      // Turning off runs no check, so a logged-out gh doesn't stop it, and
      // keeps Unblock on push.
      await d.logOutOfGH();
      const off = await d.api.setCITracking("acme", { tracking: false, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: true });
      expect(off.ci).toMatchObject({ tracking: false, unblock_on_push: true });

      // Off, the repository may be empty.
      expect((await d.api.setCITracking("acme", { tracking: false, provider: "github_actions", repository: "", ignored_workflows: [], unblock_on_push: false })).ci).toEqual({
        tracking: false,
        provider: "github_actions",
        repository: "",
        project_url: "",
        pat_env: "ADO_PAT",
        ignored_workflows: [],
        unblock_on_push: false,
        push_remote_url: "git@github.com:acme/web.git",
        rules: [],
        watcher: { state: "not_reported" },
      });
    });

    it("reads the CI Watcher's status, forgotten when CI Tracking is turned off", async () => {
      await d.createProject("acme", "Acme");
      const refused = await d.reportCIWatcherStatus("acme", { state: "watching" }).catch((e) => e);
      // The fake throws an ApiError; the service's driver, its JSON.
      expect(String(refused.code ?? refused.message)).toContain("action_not_allowed");
      await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false });
      expect((await d.api.listProjects())[0].ci.watcher).toEqual({ state: "not_reported" });

      await d.reportCIWatcherStatus("acme", { state: "error", problem: "logged_out", message: "gh isn't logged in", fix: "Run gh auth login." });
      expect((await d.api.listProjects())[0].ci.watcher).toEqual({
        state: "error",
        problem: "logged_out",
        message: "gh isn't logged in",
        fix: "Run gh auth login.",
        since: expect.any(String),
      });
      await d.reportCIWatcherStatus("acme", { state: "watching" });
      expect((await d.api.listProjects())[0].ci.watcher).toEqual({ state: "watching", since: expect.any(String) });
      // Rate-limited is still Watching, until a time.
      await d.reportCIWatcherStatus("acme", { state: "watching", rate_limited_until: "2026-01-05T10:15:00Z" });
      expect((await d.api.listProjects())[0].ci.watcher).toEqual({ state: "watching", rate_limited_until: "2026-01-05T10:15:00Z", since: expect.any(String) });
      const limitedError = await d
        .reportCIWatcherStatus("acme", { state: "error", message: "no", rate_limited_until: "2026-01-05T10:15:00Z" })
        .catch((e) => e);
      expect(String(limitedError.code ?? limitedError.message)).toContain("invalid");
      await d.reportCIWatcherStatus("acme", { state: "watching" });
      expect((await d.api.listProjects())[0].ci.watcher).toEqual({ state: "watching", since: expect.any(String) });
      // Only the service reports a crash.
      const crashed = await d.reportCIWatcherStatus("acme", { state: "error", problem: "crashed", message: "crashed" }).catch((e) => e);
      expect(String(crashed.code ?? crashed.message)).toContain("invalid");

      await d.api.setCITracking("acme", { tracking: false, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false });
      expect((await d.api.listProjects())[0].ci.watcher).toEqual({ state: "not_reported" });
    });

    it("records a pull request on every Ticket its pushed work covers, and keeps the merge commit", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
      await d.api.act("acme", id, "accept", {});
      await d.reportPushRemote("acme", "git@github.com:acme/web.git");
      await d.letGHWrite("acme/web");
      await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin" });
      const report = { branch: "impl-7", number: 7, link: "https://github.com/acme/web/pull/7" };

      // Nothing was pushed as that branch yet.
      const early = await d.reportPullRequest("acme", { ...report, state: "open" }).catch((e) => e);
      expect(String(early.code ?? early.message)).toContain("not_found");

      // Under Pull Request a push is recorded without CI Tracking.
      await d.reportPushedCommit("acme", id, "abc123", "impl-7");
      await d.reportPullRequest("acme", { ...report, state: "open" });
      expect(await d.api.getTicket("acme", id)).toMatchObject({ pull_request: { number: 7, state: "open" } });
      expect(await d.api.getTicket("acme", id)).not.toHaveProperty("ci_result");

      // A closed pull request reopened is accepted, and a merged one needs its merge commit.
      await d.reportPullRequest("acme", { ...report, state: "closed" });
      await d.reportPullRequest("acme", { ...report, state: "open" });
      const bare = await d.reportPullRequest("acme", { ...report, state: "merged" }).catch((e) => e);
      expect(String(bare.code ?? bare.message)).toContain("invalid");
      await d.reportPullRequest("acme", { ...report, state: "merged", merge_commit: "def456" });
      expect(await d.api.getTicket("acme", id)).toMatchObject({ pull_request: { number: 7, state: "merged", merge_commit: "def456" } });
    });

    it("tracks a pushed Ticket's CI Runs and works its CI result out", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
      await d.api.act("acme", id, "accept", {});
      await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });

      // Only an Accepted or In Review Ticket is pushed.
      const ready = await d.createTicket("acme", "Not yet", "Ready");
      const early = await d.reportPushedCommit("acme", ready, "abc123").catch((e) => e);
      expect(String(early.code ?? early.message)).toContain("action_not_allowed");

      // With CI Tracking off, a push isn't tracked, and no runs are taken.
      await d.reportPushedCommit("acme", id, "abc123");
      expect(await d.api.getTicket("acme", id)).not.toHaveProperty("pushed_commit");

      const off = await d.reportCIRuns("acme", []).catch((e) => e);
      expect(String(off.code ?? off.message)).toContain("action_not_allowed");

      await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"], unblock_on_push: false });
      await d.reportPushedCommit("acme", id, "abc123");
      const pushed = await d.api.getTicket("acme", id);
      expect(pushed).toMatchObject({ pushed_commit: { commit: "abc123" }, ci_result: "running" });
      expect(pushed).not.toHaveProperty("ci_runs");

      const run = (n: number, workflow: string, status: CIRunView["status"], conclusion?: CIRunView["conclusion"]) => ({
        id: n,
        attempt: 1,
        workflow,
        name: workflow,
        branch: "main",
        commit: "abc123",
        status,
        ...(conclusion && { conclusion }),
        url: `https://github.com/acme/web/actions/runs/${n}`,
        jobs: [{ name: "test", ...(conclusion && { conclusion }), url: `https://github.com/acme/web/actions/runs/${n}/job/1` }],
        ticket: id,
      });
      const concluded = await d.reportCIRuns("acme", [{ ...run(1, "ci.yml", "in_progress"), conclusion: "success" }]).catch((e) => e);
      expect(String(concluded.code ?? concluded.message)).toContain("invalid");
      await d.reportCIRuns("acme", [run(1, "ci.yml", "in_progress"), run(2, "nightly.yml", "completed", "failure")]);
      const running = await d.api.getTicket("acme", id);
      expect(running.ci_result).toBe("running");
      expect(running.ci_runs).toEqual([
        { ...run(1, "ci.yml", "in_progress"), ignored: false },
        { ...run(2, "nightly.yml", "completed", "failure"), ignored: true },
      ]);

      await d.reportCIRuns("acme", [run(1, "ci.yml", "completed", "success")]);
      const passed = await d.api.getTicket("acme", id);
      expect(passed.ci_result).toBe("passed");
      expect(passed.ci_runs?.map((r) => [r.id, r.status])).toEqual([
        [1, "completed"],
        [2, "completed"],
      ]);

      // The rows the Spec view and the Activity Dashboard read carry it
      // too, for their badges; a Ticket never pushed has none.
      const rows = await d.api.findTickets("acme", { state: ["Accepted", "Ready"] });
      expect(rows.find((r) => r.id === id)?.ci_result).toBe("passed");
      expect(rows.find((r) => r.id === ready)).not.toHaveProperty("ci_result");
      const spec = await d.api.getSpec("acme", passed.spec.id);
      expect(spec.tickets.find((r) => r.id === id)?.ci_result).toBe("passed");
    });

    it("offers a CI failed Ticket Land anyway under Merge, and records the human's choice", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
      await d.api.act("acme", id, "accept", {});
      await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
      await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false });
      await d.reportPushedCommit("acme", id, "abc123");

      // While CI is running there is nothing to land anyway.
      const running = await d.api.act("acme", id, "land_anyway", {}).catch((e) => e);
      expect(running).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect(await d.api.getTicket("acme", id)).not.toHaveProperty("ci_failure_options");

      const run = {
        id: 1,
        attempt: 1,
        workflow: "ci.yml",
        name: "CI",
        branch: "main",
        commit: "abc123",
        status: "completed" as const,
        conclusion: "failure" as const,
        url: "https://github.com/acme/web/actions/runs/1",
        jobs: [],
        ticket: id,
      };
      await d.reportCIRuns("acme", [run]);
      const failed = await d.api.getTicket("acme", id);
      expect(failed).toMatchObject({ ci_result: "failed", ci_failure_options: ["rerun_ci", "land_anyway", "send_back", "new_fix_ticket"] });
      expect(failed).not.toHaveProperty("land_anyway");

      // Under Branch the human's merge decides: no Land anyway.
      await d.api.setLandingMethod("acme", { method: "branch", push: true, remote: "origin" });
      expect((await d.api.getTicket("acme", id)).ci_failure_options).toEqual(["rerun_ci", "send_back", "new_fix_ticket"]);
      const branch = await d.api.act("acme", id, "land_anyway", {}).catch((e) => e);
      expect(branch).toMatchObject({ status: 409, code: "action_not_allowed" });

      await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
      const ack = await d.api.act("acme", id, "land_anyway", {});
      wantAcknowledgement(ack, id, "Accepted", ["land_anyway", "ci_failure_options"]);
      const chosen = await d.api.getTicket("acme", id);
      expect(chosen).toMatchObject({ ci_result: "failed", land_anyway: true });
      expect(chosen).not.toHaveProperty("ci_failure_options");
      expect(chosen.needs_me_reason).not.toBe("ci_failed");
    });

    it("re-runs a CI failed Ticket's failed jobs: it waits on CI until the run's new attempt passes (code-central-265)", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
      await d.api.act("acme", id, "accept", {});
      await d.api.setLandingMethod("acme", { method: "branch", push: true, remote: "origin" });
      await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false });
      await d.reportPushedCommit("acme", id, "abc123");

      const running = await d.api.act("acme", id, "rerun_ci", {}).catch((e) => e);
      expect(running).toMatchObject({ status: 409, code: "action_not_allowed" });

      const run = {
        id: 1,
        attempt: 1,
        workflow: "ci.yml",
        name: "CI",
        branch: "main",
        commit: "abc123",
        status: "completed" as const,
        conclusion: "failure" as const,
        url: "https://github.com/acme/web/actions/runs/1",
        jobs: [],
        ticket: id,
      };
      await d.reportCIRuns("acme", [run]);
      expect(await d.api.getTicket("acme", id)).toMatchObject({ ci_result: "failed", ci_failure_options: ["rerun_ci", "send_back", "new_fix_ticket"] });

      const ack = await d.api.act("acme", id, "rerun_ci", {});
      wantAcknowledgement(ack, id, "Accepted", ["ci_result", "ci_failure_options"]);
      const waiting = await d.api.getTicket("acme", id);
      expect(waiting.ci_result).toBe("running");
      expect(waiting).not.toHaveProperty("ci_failure_options");

      // The failed attempt reported again still waits; the new one answers.
      await d.reportCIRuns("acme", [run]);
      expect((await d.api.getTicket("acme", id)).ci_result).toBe("running");
      await d.reportCIRuns("acme", [{ ...run, attempt: 2, status: "completed", conclusion: "success" }]);
      expect((await d.api.getTicket("acme", id)).ci_result).toBe("passed");
    });

    it("sends a CI failed Ticket back to fix with a note, or opens a New fix Ticket and lands it anyway (code-central-264)", async () => {
      await d.createProject("acme", "Acme");
      const failedTicket = async (title: string, run: number) => {
        const id = await d.createTicket("acme", title, "Ready");
        await d.finish("acme", id, "Done.", { branch: id, commit: "abc123" });
        await d.api.act("acme", id, "accept", {});
        await d.reportPushedCommit("acme", id, `abc12${run}`);
        await d.reportCIRuns("acme", [
          {
            id: run,
            attempt: 1,
            workflow: "ci.yml",
            name: "CI",
            branch: "main",
            commit: `abc12${run}`,
            status: "completed" as const,
            conclusion: "failure" as const,
            url: `https://github.com/acme/web/actions/runs/${run}`,
            jobs: [],
            ticket: id,
          },
        ]);
        return id;
      };
      await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
      await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false });

      const back = await failedTicket("Login page", 1);
      const noNote = await d.api.act("acme", back, "send_back", { note: " " }).catch((e) => e);
      expect(noNote).toMatchObject({ status: 422, code: "invalid" });
      const sent = await d.api.act("acme", back, "send_back", { note: "CI failed: runs/1" });
      expect(sent).toMatchObject({ id: back, state: "Ready" });
      const ready = await d.api.getTicket("acme", back);
      expect(ready.comments.at(-1)).toMatchObject({ actor: "human", body: "CI failed: runs/1" });
      expect(ready).not.toHaveProperty("ci_failure_options");

      const fixed = await failedTicket("Sign-up page", 2);
      const before = await d.api.getTicket("acme", fixed);
      const ack = await d.api.act("acme", fixed, "new_fix_ticket", { note: "CI failed: runs/2" });
      expect(ack).toMatchObject({ state: "Ready" });
      expect(ack.id).not.toBe(fixed);
      const fix = await d.api.getTicket("acme", ack.id);
      expect(fix).toMatchObject({ state: "Ready", spec: before.spec, title: `Fix the CI failure of ${fixed}: Sign-up page` });
      expect(fix.description).toContain("CI failed: runs/2");
      const landing = await d.api.getTicket("acme", fixed);
      expect(landing).toMatchObject({ state: "Accepted", land_anyway: true });
      expect(landing).not.toHaveProperty("ci_failure_options");
      expect(landing.comments.at(-1)?.body).toContain(ack.id);
    });

    it("reads the CI view: each run's latest attempt, unfinished first, for one Project or All Projects (code-central-258)", async () => {
      await d.createProject("acme", "Acme");
      await d.createProject("quiet", "Quiet");
      const id = await d.createTicket("acme", "Pick a colour", "Ready");
      await d.finish("acme", id, "Done.", { branch: id, commit: "abc123" });
      await d.api.act("acme", id, "accept", {});
      await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
      await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"], unblock_on_push: false });
      await d.reportPushedCommit("acme", id, "abc123");
      // The pushed commit's time is the service's now, which the runs' times follow.
      const pushed = Date.parse((await d.api.getTicket("acme", id)).pushed_commit!.pushed);
      const at = (minutes: number) => new Date(pushed + minutes * 60_000).toISOString();
      const run = (n: number, workflow: string, started: number, finished?: number, attempt = 1) => ({
        id: n,
        attempt,
        workflow,
        name: workflow,
        branch: "main",
        commit: "abc123",
        status: finished === undefined ? ("in_progress" as const) : ("completed" as const),
        ...(finished !== undefined && { conclusion: "success" as const, finished: at(finished) }),
        started: at(started),
        url: `https://github.com/acme/web/actions/runs/${n}`,
        jobs: [],
        ticket: id,
      });
      await d.reportCIRuns("acme", [run(1, "ci.yml", -3, -2), run(2, "lint.yml", -4), run(3, "nightly.yml", -2), run(1, "ci.yml", -1, 0, 2)]);

      const acme = await d.api.ciOverview({ project: "acme" });
      expect(acme.projects).toEqual([{ project: "acme", tracking: true, provider: "github_actions", repository: "acme/web", project_url: "", watcher: { state: "not_reported" } }]);
      expect(acme.runs.map((r) => [r.project, r.workflow, r.attempt, r.ignored])).toEqual([
        ["acme", "nightly.yml", 1, true],
        ["acme", "lint.yml", 1, false],
        ["acme", "ci.yml", 2, false],
      ]);
      expect(acme.runs[2]).toMatchObject({ ticket: id, commit: "abc123", conclusion: "success", url: "https://github.com/acme/web/actions/runs/1" });

      // All Projects is every active Project with CI Tracking on.
      const all = await d.api.ciOverview();
      expect(all.projects.map((p) => p.project)).toEqual(["acme"]);
      expect(all.runs).toEqual(acme.runs);
      expect(await d.api.ciOverview({ project: "quiet" })).toEqual({
        projects: [{ project: "quiet", tracking: false, provider: "github_actions", repository: "", project_url: "", watcher: { state: "not_reported" } }],
        runs: [],
      });
      const unknown = await d.api.ciOverview({ project: "nope" }).catch((e) => e);
      expect(unknown.code).toBe("not_found");
    });

    it("reads the CI view's running runs: those counting towards a Ticket's CI result, newest started first (code-central-276)", async () => {
      await d.createProject("acme", "Acme");
      await d.createProject("quiet", "Quiet");
      const id = await d.createTicket("acme", "Pick a colour", "Ready");
      await d.finish("acme", id, "Done.", { branch: id, commit: "abc123" });
      await d.api.act("acme", id, "accept", {});
      await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
      await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"], unblock_on_push: false });
      const spec = (await d.api.addCIRule("acme", { workflow: "deploy.yml", branch: "main", state: "Draft" })).ci.rules[0].spec;
      await d.reportPushedCommit("acme", id, "old111");
      await d.reportPushedCommit("acme", id, "abc123");
      const pushed = Date.parse((await d.api.getTicket("acme", id)).pushed_commit!.pushed);
      const at = (minutes: number) => new Date(pushed + minutes * 60_000).toISOString();
      const run = (n: number, workflow: string, started: number, status: "queued" | "in_progress" | "completed", more: Partial<CIRunView> = {}) => ({
        id: n,
        attempt: 1,
        workflow,
        name: workflow,
        branch: "main",
        commit: "abc123",
        status,
        ...(status === "completed" && { conclusion: "success" as const, finished: at(started + 1) }),
        started: at(started),
        url: `https://github.com/acme/web/actions/runs/${n}`,
        jobs: [],
        ticket: id,
        ...more,
      });
      await d.reportCIRuns("acme", [
        run(1, "ci.yml", -5, "in_progress"),
        run(2, "queued.yml", -3, "queued"),
        run(3, "lint.yml", -4, "completed"),
        run(4, "nightly.yml", -2, "in_progress"),
        run(5, "old.yml", -2, "in_progress", { commit: "old111" }),
        run(6, "deploy.yml", -1, "in_progress", { ticket: undefined, rule: spec }),
        run(7, "e2e.yml", -6, "in_progress"),
        run(7, "e2e.yml", -1, "completed", { attempt: 2 }),
      ]);

      const acme = await d.api.ciOverview({ project: "acme", running: true });
      expect(acme.projects.map((p) => [p.project, p.tracking, p.repository])).toEqual([["acme", true, "acme/web"]]);
      expect(acme.runs.map((r) => [r.project, r.workflow, r.attempt])).toEqual([
        ["acme", "queued.yml", 1],
        ["acme", "ci.yml", 1],
      ]);
      expect((await d.api.ciOverview({ running: true })).runs).toEqual(acme.runs);
      expect((await d.api.ciOverview({ project: "acme" })).runs).toHaveLength(7);
      expect(await d.api.ciOverview({ project: "quiet", running: true })).toMatchObject({ runs: [] });
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

    it("runs a human action on a Ticket, acknowledging it with the fields it changed and the new State", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Draft");
      const before = await d.api.getTicket("acme", id);

      const ack = await d.api.act("acme", id, "move", { to_state: "Ready" });
      wantAcknowledgement(ack, id, "Ready", ["state", "moves"]);
      const t = await d.api.getTicket("acme", id);
      expect(t.state).toBe("Ready");
      expect(ack.updated).toBe(t.updated);
      for (const f of ["title", "description", "comments", "labels"] as const) {
        expect(JSON.stringify(t[f])).toBe(JSON.stringify(before[f]));
        expect(ack.changed).not.toContain(f);
      }
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

      wantAcknowledgement(await d.api.act("acme", id, "send_back", { note: "Fixture fixed on main" }), id, "Ready", ["state", "comments", "moves"]);
      const t = await d.api.getTicket("acme", id);
      expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Fixture fixed on main" });
      expect(t.moves.at(-1)).toMatchObject({ from: "Failed", to: "Ready", actor: "human" });
      expect((await d.api.needsMe()).count).toBe(0);
    });

    it("sends a Failed Ticket back with no note and adds no comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Fix login", "Ready");
      await d.reportFailure("acme", id, sampleReport);
      const before = (await d.api.getTicket("acme", id)).comments.length;

      const ack = await d.api.act("acme", id, "send_back", {});
      wantAcknowledgement(ack, id, "Ready", ["state", "moves"]);
      expect(ack.changed).not.toContain("comments");
      expect((await d.api.getTicket("acme", id)).comments).toHaveLength(before);
    });

    it("moves a Failed Ticket to Draft", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Fix login", "Ready");
      await d.reportFailure("acme", id, sampleReport);

      wantAcknowledgement(await d.api.act("acme", id, "to_draft", {}), id, "Draft", ["state", "moves"]);
      expect((await d.api.needsMe()).count).toBe(0);
    });

    it("cancels a Failed Ticket", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Fix login", "Ready");
      await d.reportFailure("acme", id, sampleReport);

      wantAcknowledgement(await d.api.act("acme", id, "cancel", {}), id, "Cancelled", ["state", "moves"]);
      expect(await d.api.getTicket("acme", id)).toMatchObject({ state: "Cancelled", replacements: [] });
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

      wantAcknowledgement(await d.api.act("acme", id, "accept", {}), id, "Done", ["state", "moves"]);
      const t = await d.api.getTicket("acme", id);
      expect(t.moves.at(-1)).toMatchObject({ from: "In Review", to: "Done", actor: "human" });
      expect((await d.api.needsMe()).count).toBe(0);
    });

    // ---- Accepted and land (ticket 71) ----

    it("accepts work on a branch into Accepted, then lands it into Done with the commit", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });

      wantAcknowledgement(await d.api.act("acme", id, "accept", {}), id, "Accepted", ["state", "moves"]);
      const a = await d.api.getTicket("acme", id);
      expect(a).toMatchObject({ state: "Accepted", branch: "impl-7", commit: "abc123" });
      expect(a.moves.at(-1)).toMatchObject({ from: "In Review", to: "Accepted", actor: "human" });

      const blank = await d.api.act("acme", id, "land", { commit: " " }).catch((e) => e);
      expect(blank).toMatchObject({ status: 422, code: "invalid", field: "commit" });

      wantAcknowledgement(await d.api.act("acme", id, "land", { commit: "def456" }), id, "Done", ["state", "moves"]);
      const t = await d.api.getTicket("acme", id);
      expect(t.moves.at(-1)).toMatchObject({ from: "Accepted", to: "Done", actor: "human", via: "merge", commit: "def456" });
    });

    it("cancels an Accepted Ticket", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
      await d.api.act("acme", id, "accept", {});

      wantAcknowledgement(await d.api.act("acme", id, "cancel", {}), id, "Cancelled", ["state", "moves"]);
    });

    it("rejects an In Review Ticket back to Ready only with a comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", id, "Added the login page");

      const blank = await d.api.act("acme", id, "reject", { comment: "  " }).catch((e) => e);
      expect(blank).toBeInstanceOf(ApiError);
      expect(blank).toMatchObject({ status: 422, code: "invalid", field: "comment" });
      expect((await d.api.getTicket("acme", id)).state).toBe("In Review");

      wantAcknowledgement(await d.api.act("acme", id, "reject", { comment: "The redirect loops" }), id, "Ready", ["state", "comments", "moves"]);
      const t = await d.api.getTicket("acme", id);
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

    it("implements a finished spec Stage: the Stage is Done, one Ready implementation Ticket is made, and the tickets Stage is Cancelled with it as Replacement", async () => {
      await d.createProject("acme", "Acme");
      const r = await d.api.submitRequirement("acme", "Offline mode\nWorks on a plane.");
      const [grilling, spec, tickets] = r.stages.map((s) => s.id);
      await d.api.act("acme", grilling, "move", { to_state: "Done" });
      await d.finish("acme", spec, "Wrote the Spec");

      const ack = await d.api.act("acme", spec, "implement", {});
      wantAcknowledgement(ack, spec, "Done", ["state", "moves"]);
      const done = await d.api.getTicket("acme", spec);
      expect(done.moves.at(-1)).toMatchObject({ from: "In Review", to: "Done", actor: "human", via: "implement" });

      const s = await d.api.getSpec("acme", r.spec.id);
      const build = s.tickets.find((t) => t.title === "Implement: Offline mode");
      expect(build).toBeDefined();
      const b = await d.api.getTicket("acme", build!.id);
      expect(b).toMatchObject({ state: "Ready", labels: [], blocked_by: [] });
      expect(b.moves).toMatchObject([{ to: "Ready", actor: "human", via: "implement" }]);
      expect(b.description).toBe(
        `Implement the whole Spec [${r.spec.id}](${r.spec.id}). Its User Stories, Implementation Decisions and Testing Decisions are the acceptance criteria, including writing the glossary terms and ADRs it records. Read them with \`get_spec\` and \`sections\`.\n`,
      );
      const cancelled = await d.api.getTicket("acme", tickets);
      expect(cancelled).toMatchObject({ state: "Cancelled", replacements: [b.id] });
      expect(cancelled.moves.at(-1)).toMatchObject({ to: "Cancelled", actor: "human", via: "implement" });
      expect(cancelled.comments.at(-1)).toMatchObject({ actor: "human", body: expect.stringContaining(b.id) });
    });

    it("refuses implement on a Ticket that isn't a spec Stage or isn't In Review, writing nothing", async () => {
      await d.createProject("acme", "Acme");
      const plain = await d.createTicket("acme", "Login page", "Ready");
      await d.finish("acme", plain, "Done it");
      const e = await d.api.act("acme", plain, "implement", {}).catch((e) => e);
      expect(e).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect(e.message).toContain("not a spec Stage");
      expect((await d.api.getTicket("acme", plain)).state).toBe("In Review");

      const r = await d.api.submitRequirement("acme", "Offline mode");
      const notReview = await d.api.act("acme", r.stages[1].id, "implement", {}).catch((e) => e);
      expect(notReview).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect((await d.api.getSpec("acme", r.spec.id)).tickets).toHaveLength(3);
    });

    // ---- Blockers, move and cancel (ticket 37) ----

    it("removes a Blocker, and rejects one that isn't there", async () => {
      await d.createProject("acme", "Acme");
      const a = await d.createTicket("acme", "A", "Ready");
      const b = await d.createTicket("acme", "B", "Ready");
      await d.addBlocker("acme", b, a);
      expect(await d.api.getTicket("acme", b)).toMatchObject({ blocked_by: [a], blocked: true });

      wantAcknowledgement(await d.api.act("acme", b, "remove_blocker", { blocker: a }), b, "Ready", ["blocked_by", "blockers", "blocked"]);
      expect(await d.api.getTicket("acme", b)).toMatchObject({ blocked_by: [], blockers: [], blocked: false });

      const again = await d.api.act("acme", b, "remove_blocker", { blocker: a }).catch((e) => e);
      expect(again).toMatchObject({ status: 422, code: "invalid", field: "blocker" });
    });

    it("cancels as the human with no Replacement, leaving its dependant blocked by a Cancelled Ticket", async () => {
      await d.createProject("acme", "Acme");
      const a = await d.createTicket("acme", "A", "Ready");
      const b = await d.createTicket("acme", "B", "Ready");
      await d.addBlocker("acme", b, a);

      wantAcknowledgement(await d.api.act("acme", a, "cancel", {}), a, "Cancelled", ["state", "moves"]);
      const t = await d.api.getTicket("acme", a);
      expect(t).toMatchObject({ state: "Cancelled", replacements: [] });
      expect(t.moves.at(-1)).toMatchObject({ from: "Ready", to: "Cancelled", actor: "human" });
      const dep = await d.api.getTicket("acme", b);
      expect(dep).toMatchObject({ blocked: true, blockers: [{ id: a, title: "A", state: "Cancelled", cleared: false }] });
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
      wantAcknowledgement(await d.api.act("acme", id, "move", { to_state: "Done" }), id, "Done", ["state", "moves"]);
      const t = await d.api.getTicket("acme", id);
      expect(t.moves.at(-1)).toMatchObject({ from: "Ready", to: "Done", actor: "human" });
    });

    // ---- release (ticket 35) ----
    it("releases a Claim back to Ready, ending it, with the note as a comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Ready");
      await d.claim("acme", id, "sess-1");

      wantAcknowledgement(await d.api.act("acme", id, "release", { note: "Session died" }), id, "Ready", ["state", "claims", "comments", "moves"]);
      const t = await d.api.getTicket("acme", id);
      expect(t.claims).toHaveLength(1);
      expect(t.claims[0]).toMatchObject({ session_id: "sess-1", ended_by: "release" });
      expect(t.claims[0].ended).toBeTruthy();
      expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Session died" });

      const again = await d.api.act("acme", id, "release", {}).catch((e) => e);
      expect(again).toMatchObject({ status: 409, code: "action_not_allowed" });
    });

    it("releases without a note, adding no comment", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Ready");
      await d.claim("acme", id, "sess-1");

      const ack = await d.api.act("acme", id, "release", {});
      wantAcknowledgement(ack, id, "Ready", ["state", "claims", "moves"]);
      expect(ack.changed).not.toContain("comments");
      expect((await d.api.getTicket("acme", id)).comments).toEqual([]);
    });

    // ---- Requirements (ticket 74) ----
    it("submits a Requirement as its Spec and three chained Stage Tickets, waiting for a Factory", async () => {
      await d.createProject("acme", "Acme");
      const r = await d.api.submitRequirement("acme", "Export a Spec as Markdown\nOne file, Tickets included.");
      expect(Object.keys(r.spec).sort()).toEqual(["changed", "id", "updated"]);
      expect(Number.isNaN(Date.parse(r.spec.updated))).toBe(false);
      expect(r.spec.changed).toEqual(["title", "body"]);
      expect(r.stages).toHaveLength(3);
      const chained = ["title", "state", "spec", "labels", "blocked_by", "blockers", "description", "moves", "blocked"];
      for (const [i, t] of r.stages.entries()) {
        wantAcknowledgement(t, t.id, "Ready");
        expect(t.changed).toEqual(i === 0 ? ["title", "state", "spec", "labels", "description", "moves", "claimable"] : chained);
      }

      const stages = await Promise.all(r.stages.map((t) => d.api.getTicket("acme", t.id)));
      expect(stages.map((t) => [t.labels, t.state])).toEqual([
        [["stage:grilling"], "Ready"],
        [["stage:spec"], "Ready"],
        [["stage:tickets"], "Ready"],
      ]);
      expect(stages[1].blocked_by).toEqual([r.stages[0].id]);
      expect(stages[2].blocked_by).toEqual([r.stages[1].id]);

      const s = await d.api.getSpec("acme", r.spec.id);
      expect(s).toMatchObject({ id: r.spec.id, title: "Export a Spec as Markdown", body: "Export a Spec as Markdown\nOne file, Tickets included.", finished: false });
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

    it("shows a Requirement's Land per Spec status under Pull Request with Land per Spec on", async () => {
      await d.createProject("acme", "Acme");
      await d.reportPushRemote("acme", "git@github.com:acme/web.git");
      await d.letGHWrite("acme/web");
      await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "", land_per_spec: true });
      const r = await d.api.submitRequirement("acme", "Export a Spec as Markdown");
      expect((await d.api.getSpec("acme", r.spec.id)).land_per_spec).toEqual({ waiting: [], ready: false });
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

    // ---- Maps (code-central-218) ----
    it("submits a Map as its Spec, with the Map's sections, and one Ready Charting Ticket", async () => {
      await d.createProject("acme", "Acme");
      const r = await d.api.submitMap("acme", "Offline mode\nWorks on a plane.");
      expect(r.spec.changed).toEqual(["title", "labels", "body"]);
      wantAcknowledgement(r.charting, r.charting.id, "Ready");
      expect(r.charting.changed).toEqual(["title", "state", "spec", "labels", "description", "moves", "claimable"]);

      const charting = await d.api.getTicket("acme", r.charting.id);
      expect(charting).toMatchObject({ title: "Charting: Offline mode", labels: ["wayfinder:chart"], state: "Ready", claimable: true });
      expect(charting.description).toContain(r.spec.id);

      const s = await d.api.getSpec("acme", r.spec.id);
      expect(s).toMatchObject({ title: "Offline mode", labels: ["wayfinder:map"] });
      expect(s.body).toBe(
        "Offline mode\nWorks on a plane.\n\n## Destination\n\n## Notes\n\n## Decisions so far\n\n## Not yet specified\n\n## Out of scope\n",
      );
      expect(s.requirement).toBeUndefined();
      expect(s.map).toEqual({ decided: 0, total: 1, needs_you: 0, guided: false, can_write_spec: false, requirements: [] });
      expect(s.tickets.map((t) => [t.id, t.node_type])).toEqual([[r.charting.id, "chart"]]);

      await d.api.act("acme", r.charting.id, "move", { to_state: "Done" });
      expect((await d.api.getSpec("acme", r.spec.id)).map).toEqual({ decided: 1, total: 1, needs_you: 0, guided: false, can_write_spec: true, requirements: [] });
    });

    it("writes the Spec from a finished Map: a Requirement with only the spec and tickets Stages, tagged with the Map", async () => {
      await d.createProject("acme", "Acme");
      const plain = await d.api.submitRequirement("acme", "Not a Map");
      const m = await d.api.submitMap("acme", "Offline mode");
      const notMap = await d.api.submitRequirement("acme", "Offline mode", plain.spec.id).catch((e) => e);
      expect(notMap).toMatchObject({ status: 422, code: "invalid", field: "map" });
      const open = await d.api.submitRequirement("acme", "Offline mode", m.spec.id).catch((e) => e);
      expect(open).toBeInstanceOf(ApiError);
      expect(open).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect(open.message).toContain(`${m.charting.id} (Ready)`);

      await d.api.act("acme", m.charting.id, "move", { to_state: "Done" });
      const r = await d.api.submitRequirement("acme", "Offline mode\nFrom the Map.", m.spec.id);
      expect(r.stages).toHaveLength(2);
      wantAcknowledgement(r.stages[0], r.stages[0].id, "Ready");
      expect(r.stages[0].changed).toEqual(["title", "state", "spec", "labels", "description", "moves", "claimable"]);
      const [spec, tickets] = await Promise.all(r.stages.map((t) => d.api.getTicket("acme", t.id)));
      expect([spec.labels, tickets.labels, tickets.blocked_by]).toEqual([["stage:spec"], ["stage:tickets"], [spec.id]]);
      expect(spec.description).toContain(m.spec.id);
      expect(spec.description).toContain("Decisions so far");

      const s = await d.api.getSpec("acme", r.spec.id);
      expect(s.tags).toEqual({ map: m.spec.id });
      expect(s.requirement).toEqual({
        map: m.spec.id,
        stages: [
          { name: "spec", ticket: spec.id, state: "Ready" },
          { name: "tickets", ticket: tickets.id, state: "Ready" },
        ],
        build: { done: 0, total: 0 },
        waiting_for_factory: true,
      });
      expect((await d.api.getSpec("acme", m.spec.id)).map?.requirements).toEqual([r.spec.id]);
      expect((await d.api.getSpec("acme", plain.spec.id)).requirement?.map).toBeUndefined();
    });

    it("guides a Map only while its guided Tag is on", async () => {
      await d.createProject("acme", "Acme");
      const r = await d.api.submitMap("acme", "Offline mode");
      const guided = async () => (await d.api.getSpec("acme", r.spec.id)).map?.guided;

      expect(await d.api.setTags("acme", r.spec.id, { guided: "on" })).toEqual({ id: r.spec.id, kind: "spec", labels: ["wayfinder:map"], tags: { guided: "on" } });
      expect(await guided()).toBe(true);
      expect((await d.api.findSpecs("acme", { labels: ["wayfinder:map"] }))[0].map?.guided).toBe(true);
      await d.api.setTags("acme", r.spec.id, { guided: "yes" });
      expect(await guided()).toBe(false);
      await d.api.setTags("acme", r.spec.id, { guided: "on", area: "web" });
      expect(await d.api.removeTags("acme", r.spec.id, ["guided"])).toMatchObject({ tags: { area: "web" } });
      expect(await guided()).toBe(false);
    });

    it("refuses a Map with no text", async () => {
      await d.createProject("acme", "Acme");
      const e = await d.api.submitMap("acme", "  \n ").catch((e) => e);
      expect(e).toBeInstanceOf(ApiError);
      expect(e).toMatchObject({ status: 422, code: "invalid", field: "text" });
    });

    it("finds Maps by their Label, each row with its Map block, and no node type off a Map", async () => {
      await d.createProject("acme", "Acme");
      const plain = await d.createTicket("acme", "Not on a Map", "Ready");
      const r = await d.api.submitMap("acme", "Offline mode");

      const maps = await d.api.findSpecs("acme", { labels: ["wayfinder:map"] });
      expect(maps.map((s) => [s.id, s.map])).toEqual([[r.spec.id, { decided: 0, total: 1, needs_you: 0, guided: false, can_write_spec: false, requirements: [] }]]);
      expect((await d.api.findSpecs("acme")).filter((s) => s.id !== r.spec.id).every((s) => s.map === undefined)).toBe(true);
      const rows = await d.api.findTickets("acme");
      expect(rows.find((t) => t.id === plain)?.node_type).toBeUndefined();
      expect(rows.find((t) => t.id === r.charting.id)?.node_type).toBe("chart");
    });

    it("gives a Map's Ticket rows their Blockers, the Map view's edges, and no other row", async () => {
      await d.createProject("acme", "Acme");
      const a = await d.createTicket("acme", "Not on a Map", "Ready");
      const b = await d.createTicket("acme", "Nor this", "Ready");
      await d.addBlocker("acme", b, a);
      const r = await d.api.submitMap("acme", "Offline mode");
      await d.addBlocker("acme", r.charting.id, a);

      const s = await d.api.getSpec("acme", r.spec.id);
      expect(s.tickets.map((t) => [t.id, t.blocked_by])).toEqual([[r.charting.id, [a]]]);
      const rows = await d.api.findTickets("acme");
      expect(rows.find((t) => t.id === b)?.blocked_by).toBeUndefined();
      expect(rows.find((t) => t.id === r.charting.id)?.blocked_by).toEqual([a]);
    });

    it("approves a Draft Ticket to Ready, and refuses one that isn't Draft", async () => {
      await d.createProject("acme", "Acme");
      const draft = await d.createTicket("acme", "A", "Draft");
      const ready = await d.createTicket("acme", "B", "Ready");

      wantAcknowledgement(await d.api.act("acme", draft, "approve", {}), draft, "Ready", ["state", "moves"]);
      expect((await d.api.getTicket("acme", draft)).moves.at(-1)).toMatchObject({ from: "Draft", to: "Ready", actor: "human" });
      const e = await d.api.act("acme", ready, "approve", {}).catch((e) => e);
      expect(e).toMatchObject({ status: 409, code: "action_not_allowed" });
    });

    // ---- Prototypes (code-central-221, code-central-223) ----
    it("submits a Prototype on a Spec as one Ready prototype Ticket, titled by the text's first line", async () => {
      await d.createProject("acme", "Acme");
      const r = await d.api.submitRequirement("acme", "Dark mode");
      const ack = await d.api.submitPrototype("acme", r.spec.id, "A theme picker\nWould a picker beat a toggle?");
      wantAcknowledgement(ack, ack.id, "Ready", ["title", "state", "spec", "labels", "description", "moves", "claimable"]);

      const t = await d.api.getTicket("acme", ack.id);
      expect(t).toMatchObject({
        title: "Prototype: A theme picker",
        description: "A theme picker\nWould a picker beat a toggle?",
        state: "Ready",
        spec: { id: r.spec.id },
        labels: ["prototype"],
        claimable: true,
      });
      expect(t.moves).toMatchObject([{ to: "Ready", actor: "human", via: "submit_prototype" }]);
      expect((await d.api.getSpec("acme", r.spec.id)).tickets.map((t) => t.id)).toContain(ack.id);
    });

    it("refuses a Prototype with no text, or on no Spec", async () => {
      await d.createProject("acme", "Acme");
      const r = await d.api.submitMap("acme", "Offline mode");
      const blank = await d.api.submitPrototype("acme", r.spec.id, "  \n ").catch((e) => e);
      expect(blank).toBeInstanceOf(ApiError);
      expect(blank).toMatchObject({ status: 422, code: "invalid", field: "text" });
      const nowhere = await d.api.submitPrototype("acme", "acme-99", "A theme picker").catch((e) => e);
      expect(nowhere).toMatchObject({ status: 422, code: "invalid", field: "spec" });
    });

    // ---- the Integration contract (ticket 42) ----
    it("edits a Ticket's title and description, recording one edit by human with only what changed", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Pick a colour", "Draft");

      const one = await d.api.act("acme", id, "edit_ticket", { description: "Blue." });
      wantAcknowledgement(one, id, "Draft", ["description", "edits"]);
      expect(one.changed).not.toContain("title");
      expect(one.edit).toBe(1);
      const both = await d.api.act("acme", id, "edit_ticket", { title: "Pick a shade", description: "Navy." });
      wantAcknowledgement(both, id, "Draft", ["title", "description", "edits"]);

      const t = await d.api.getTicket("acme", id);
      expect(t).toMatchObject({ title: "Pick a shade", description: "Navy." });
      expect(t.edits.map((e) => [e.actor, e.previous])).toEqual([
        ["human", { description: "" }],
        ["human", { title: "Pick a colour", description: "Blue." }],
      ]);
    });

    it("writes no edit for a Ticket change that changes nothing, and refuses an empty title", async () => {
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Pick a colour", "Draft");
      const same = await d.api.act("acme", id, "edit_ticket", { title: "Pick a colour", description: "" });
      expect(same.changed).toEqual([]);
      expect(same.edit).toBeUndefined();
      expect((await d.api.getTicket("acme", id)).edits).toEqual([]);

      const empty = await d.api.act("acme", id, "edit_ticket", { title: "  " }).catch((e) => e);
      expect(empty).toBeInstanceOf(ApiError);
      expect(empty).toMatchObject({ status: 422, code: "invalid", field: "title" });
      expect((await d.api.getTicket("acme", id)).title).toBe("Pick a colour");
    });

    it("still edits a Ready Ticket, and refuses a definition-owned linked Ticket until it is unlinked", async () => {
      await d.createProject("acme", "Acme");
      const ready = await d.createTicket("acme", "Ready one", "Ready");
      wantAcknowledgement(await d.api.act("acme", ready, "edit_ticket", { title: "Ready two" }), ready, "Ready", ["title", "edits"]);

      const gh = { integration: "github", id: "acme/web#9", url: "https://github.com/acme/web/issues/9" };
      const linked = await d.linkTicket("acme", "Issue", gh, "open");
      const refused = await d.api.act("acme", linked, "edit_ticket", { title: "Mine" }).catch((e) => e);
      expect(refused).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect((await d.api.getTicket("acme", linked)).title).toBe("Issue");

      await d.api.unlink("acme", linked);
      wantAcknowledgement(await d.api.act("acme", linked, "edit_ticket", { title: "Mine" }), linked, "Draft", ["title", "edits"]);
    });

    it("edits a Spec's title and body, recording one edit by human with only what changed", async () => {
      await d.createProject("acme", "Acme");
      const { spec } = await d.api.submitRequirement("acme", "Offline mode\nWorks on a plane.");
      const before = await d.api.getSpec("acme", spec.id);

      const one = await d.api.editSpec("acme", spec.id, { body: "New body." });
      expect(one).toMatchObject({ id: spec.id, edit: before.edits.length + 1 });
      expect(one.changed).toEqual(expect.arrayContaining(["body", "edits"]));
      expect(one.changed).not.toContain("title");
      const both = await d.api.editSpec("acme", spec.id, { title: "Flight mode", body: "Newer body." });
      expect(both.changed).toEqual(expect.arrayContaining(["title", "body", "edits"]));

      const s = await d.api.getSpec("acme", spec.id);
      expect(s).toMatchObject({ title: "Flight mode", body: "Newer body." });
      expect(s.edits.slice(before.edits.length).map((e) => [e.actor, Object.keys(e.previous).sort()])).toEqual([
        ["human", ["body"]],
        ["human", ["body", "title"]],
      ]);
      expect(s.edits.at(-1)!.previous.title).toBe("Offline mode");
    });

    it("writes no edit for a Spec change that changes nothing, and refuses an empty title", async () => {
      await d.createProject("acme", "Acme");
      const { spec } = await d.api.submitRequirement("acme", "Offline mode\nWorks on a plane.");
      const s = await d.api.getSpec("acme", spec.id);
      const same = await d.api.editSpec("acme", spec.id, { title: s.title, body: s.body });
      expect(same.changed).toEqual([]);
      expect(same.edit).toBeUndefined();
      expect((await d.api.getSpec("acme", spec.id)).edits).toEqual(s.edits);

      const empty = await d.api.editSpec("acme", spec.id, { title: "" }).catch((e) => e);
      expect(empty).toMatchObject({ status: 422, code: "invalid", field: "title" });
    });

    it("still edits a Finished Spec, and refuses an edit in an Archived Project", async () => {
      await d.createProject("acme", "Acme");
      const { spec, stages } = await d.api.submitRequirement("acme", "Offline mode");
      for (const t of stages) await d.api.act("acme", t.id, "cancel", {});
      expect((await d.api.getSpec("acme", spec.id)).finished).toBe(true);
      const ack = await d.api.editSpec("acme", spec.id, { body: "Still editable." });
      expect(ack.changed).toContain("body");

      const t = await d.createTicket("acme", "Draft one", "Draft");
      await d.api.act("acme", t, "cancel", {});
      await d.api.archiveProject("acme");
      const spec1 = await d.api.editSpec("acme", spec.id, { body: "No." }).catch((e) => e);
      expect(spec1).toMatchObject({ status: 409, code: "action_not_allowed" });
      const ticket1 = await d.api.act("acme", t, "edit_ticket", { title: "No" }).catch((e) => e);
      expect(ticket1).toMatchObject({ status: 409, code: "action_not_allowed" });
    });

    it("reads a linked Ticket closed externally, and unlinks it", async () => {
      await d.createProject("acme", "Acme");
      const gh = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7" };
      const id = await d.linkTicket("acme", "Issue", gh, "closed");
      const link = { ...gh, external_state: "closed", detached: false, outbound: false };

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

    // ---- New issue and Keep as Draft (code-central-287) ----

    // incomingIssue turns GitHub Issues on for a new Project acme, from
    // acme/web, with New issues need me as needMe, and brings acme/web's
    // issue n in as title; it returns its Ticket and its issue's link.
    async function incomingIssue(needMe: boolean, n: number, title: string) {
      await d.createProject("acme", "Acme");
      await d.letGHTriage("acme/web");
      const on = await d.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "", new_issues_need_me: needMe });
      const gh = { integration: "github", id: `acme/web#${n}`, url: `https://github.com/acme/web/issues/${n}` };
      return { id: await d.incomingIssue("acme", on.github_issues.spec!, title, gh), gh };
    }

    it("shows a New issue in Needs Me until the human keeps it as a Draft", async () => {
      const { id, gh } = await incomingIssue(true, 7, "Login is broken");
      const link = { ...gh, external_state: "open", detached: false, outbound: false };

      expect(await d.api.getTicket("acme", id)).toMatchObject({ state: "Draft", link, awaiting_triage: true, triage_actions: TRIAGE_ACTIONS, needs_me_reason: "new_issue" });
      const nm = await d.api.needsMe();
      expect(nm.groups.map((g) => g.reason)).toEqual(["new_issue"]);
      expect(nm.groups[0].tickets[0]).toMatchObject({ id, reason: "new_issue", link });

      wantAcknowledgement(await d.api.act("acme", id, "keep_as_draft", {}), id, "Draft", ["needs_me_reason", "awaiting_triage"]);
      const kept = await d.api.getTicket("acme", id);
      expect(kept).toMatchObject({ state: "Draft", triage_actions: TRIAGE_ACTIONS });
      expect(kept.awaiting_triage).toBeUndefined();
      expect(kept.needs_me_reason).toBeUndefined();
      expect((await d.api.needsMe()).count).toBe(0);

      const plain = await d.createTicket("acme", "Local", "Draft");
      const refused = await d.api.act("acme", plain, "keep_as_draft", {}).catch((e) => e);
      expect(refused).toBeInstanceOf(ApiError);
      expect(refused).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect(refused.message).toContain("not an Incoming Issue");
    });

    it("offers the triage actions on an Incoming Issue that came in with New issues need me off", async () => {
      const { id } = await incomingIssue(false, 8, "Typo");
      const t = await d.api.getTicket("acme", id);
      expect(t).toMatchObject({ triage_actions: TRIAGE_ACTIONS });
      expect(t.awaiting_triage).toBeUndefined();
      expect(t.needs_me_reason).toBeUndefined();
      expect((await d.api.needsMe()).count).toBe(0);

      // Unlinked, it is no longer an Incoming Issue.
      await d.api.unlink("acme", id);
      expect((await d.api.getTicket("acme", id)).triage_actions).toBeUndefined();
    });

    // ---- Won't fix (code-central-289) ----
    it("Cancels an Incoming Issue on Won't fix, with the reason as its comment", async () => {
      const { id } = await incomingIssue(true, 7, "Login is broken");

      wantAcknowledgement(await d.api.act("acme", id, "wont_fix", { reason: "Works as intended." }), id, "Cancelled", ["state", "needs_me_reason", "awaiting_triage", "triage_actions"]);
      const t = await d.api.getTicket("acme", id);
      expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Works as intended." });
      expect(t.moves.at(-1)).toMatchObject({ to: "Cancelled", via: "wont_fix" });
      expect(t.link).toMatchObject({ detached: false });
      expect(t.awaiting_triage).toBeUndefined();
      expect(t.triage_actions).toBeUndefined();
      expect((await d.api.needsMe()).count).toBe(0);

      const plain = await d.createTicket("acme", "Local", "Draft");
      const refused = await d.api.act("acme", plain, "wont_fix", {}).catch((e) => e);
      expect(refused).toBeInstanceOf(ApiError);
      expect(refused).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect(refused.message).toContain("not an Incoming Issue");

      // Unlinked, an Incoming Issue is refused, naming its Link.
      const spec = (await d.api.getTicket("acme", id)).spec.id;
      const other = await d.incomingIssue("acme", spec, "Typo", { integration: "github", id: "acme/web#8", url: "https://github.com/acme/web/issues/8" });
      await d.api.unlink("acme", other);
      const unlinked = await d.api.act("acme", other, "wont_fix", {}).catch((e) => e);
      expect(unlinked).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect(unlinked.message).toContain(`${other} was unlinked from github acme/web#8`);
    });

    // ---- To Requirement (code-central-290) ----
    it("turns an Incoming Issue into a Requirement whose Spec has an outbound Link", async () => {
      const { id, gh } = await incomingIssue(true, 7, "Login is broken");

      const ack = await d.api.act("acme", id, "to_requirement", { context: "  Seen on every login.  " });
      wantAcknowledgement(ack, id, "Cancelled", ["state", "replacements", "comments", "moves", "needs_me_reason", "link", "awaiting_triage", "triage_actions"]);
      const t = await d.api.getTicket("acme", id);
      expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Seen on every login." });
      expect(t.moves.at(-1)).toMatchObject({ to: "Cancelled", via: "to_requirement" });
      expect(t.link).toMatchObject({ ...gh, detached: true, outbound: false });
      expect(t.awaiting_triage).toBeUndefined();
      expect(t.triage_actions).toBeUndefined();
      expect((await d.api.needsMe()).count).toBe(0);

      const grilling = await d.api.getTicket("acme", t.replacements[0]);
      expect(grilling.labels).toEqual(["stage:grilling"]);
      const sp = await d.api.getSpec("acme", grilling.spec.id);
      expect(sp).toMatchObject({ title: "Login is broken", body: "# Login is broken\n\n## Context\n\nSeen on every login.\n" });
      expect(sp.link).toEqual({ ...gh, external_state: "open", detached: false, outbound: true });
      expect(sp.requirement?.stages.map((s) => s.ticket)).toEqual([grilling.id, expect.any(String), expect.any(String)]);

      const plain = await d.createTicket("acme", "Local", "Draft");
      const refused = await d.api.act("acme", plain, "to_requirement", {}).catch((e) => e);
      expect(refused).toBeInstanceOf(ApiError);
      expect(refused).toMatchObject({ status: 409, code: "action_not_allowed" });
      expect(refused.message).toContain("not an Incoming Issue");
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
      const base = Date.parse((await d.api.getTicket("acme", impl)).claims[0].started);
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
      });
      const sessions = (u: typeof all) => u.sessions.map((s) => ({ ...s, started: Date.parse(s.started) }));
      expect(sessions(all)).toEqual([
        session("beta", spec, "sess-spec", "spec", Date.parse(at(5)), 30_002, 1, []),
        session("acme", impl, "sess-impl", "custom", Date.parse(at(4)), 101, 1, [], "toolu_b"),
        session("acme", impl, "sess-impl", "review", Date.parse(at(3)), 9_003, 1, ["claude-haiku-4-5"], "toolu_a"),
        // Posted out of order: the Opus response came first.
        session("acme", impl, "sess-impl", "implementation", Date.parse(at(1)), 61_005, 2, ["claude-opus-4-7", "claude-sonnet-4-6"]),
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

    it("reads one session's responses back in order, with each one's Context Size and token counts", async () => {
      await d.createProject("acme", "Acme");
      const impl = await d.createTicket("acme", "Build it", "Ready");
      await d.claim("acme", impl, "sess-impl");
      const base = Date.parse((await d.api.getTicket("acme", impl)).claims[0].started);
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

    it("refuses Usage for an unknown Project and any range but 7, 30 or 90 days", async () => {
      const unknown = await d.api.usage({ project: "nope" }).catch((e) => e);
      expect(unknown).toBeInstanceOf(ApiError);
      expect(unknown).toMatchObject({ status: 404, code: "not_found" });

      const bad = await d.api.usage({ days: 14 as never }).catch((e) => e);
      expect(bad).toMatchObject({ status: 422, code: "invalid", field: "days" });
    });

    // ---- Options (Spec code-central-160) ----

    it("reads the default Options, and sets focus_clear and reads it back", async () => {
      expect(await d.api.options()).toEqual({ focus_clear: "activity_dashboard", gh_path: "" });

      expect(await d.api.setOptions({ focus_clear: "all_clear" })).toEqual({ focus_clear: "all_clear", gh_path: "" });
      expect(await d.api.options()).toEqual({ focus_clear: "all_clear", gh_path: "" });
      expect(await d.api.setOptions({})).toEqual({ focus_clear: "all_clear", gh_path: "" });
    });

    it("sets the gh path Option to an absolute path, and clears it", async () => {
      // Absolute where the service runs: a Windows path isn't on Unix.
      const gh = process.platform === "win32" ? "C:\\Tools\\gh.exe" : "/opt/tools/gh";
      expect(await d.api.setOptions({ gh_path: ` ${gh} ` })).toEqual({ focus_clear: "activity_dashboard", gh_path: gh });
      expect((await d.api.options()).gh_path).toBe(gh);

      const relative = await d.api.setOptions({ gh_path: "bin/gh" }).catch((e) => e);
      expect(relative).toMatchObject({ status: 422, code: "invalid", field: "gh_path" });
      expect(relative.message).toContain("bin/gh");
      expect((await d.api.options()).gh_path).toBe(gh);

      expect(await d.api.setOptions({ gh_path: "" })).toEqual({ focus_clear: "activity_dashboard", gh_path: "" });
    });

    it("refuses an unknown Option and a bad value, changing nothing", async () => {
      const unknown = await d.api.setOptions({ colour: "red" } as never).catch((e) => e);
      expect(unknown).toBeInstanceOf(ApiError);
      expect(unknown).toMatchObject({ status: 422, code: "invalid", field: "colour" });
      expect(unknown.message).toContain("focus_clear");

      const bad = await d.api.setOptions({ focus_clear: "fireworks" } as never).catch((e) => e);
      expect(bad).toMatchObject({ status: 422, code: "invalid", field: "focus_clear" });
      expect(bad.message).toContain("allowed: activity_dashboard, all_clear");

      expect(await d.api.options()).toEqual({ focus_clear: "activity_dashboard", gh_path: "" });
    });

    it("sends options.changed over the live feed when the Options change", async () => {
      const got: ChangeEvent[] = [];
      let opened!: () => void;
      const open = new Promise<void>((r) => (opened = r));
      const stop = d.api.subscribe({ onEvent: (e) => got.push(e), onOpen: () => opened() });
      try {
        await open;
        await d.api.setOptions({ focus_clear: "all_clear" });
        await expect.poll(() => got, { timeout: 5000 }).toEqual([{ type: "options.changed" }]);
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
