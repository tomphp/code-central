import { beforeEach, expect, it } from "vitest";
import type { CIRunView } from "../types";
import type { Driver } from "../driver";
import { wantAcknowledgement } from "./support";

export function ciContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  it("reads the CI Watcher's status, forgotten when CI Tracking is turned off", async () => {
    await d.createProject("acme", "Acme");
    const refused = await d.reportCIWatcherStatus("acme", { state: "watching" }).catch((e) => e);
    expect(refused).toMatchObject({ code: "action_not_allowed" });
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
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
    expect(limitedError).toMatchObject({ code: "invalid" });
    await d.reportCIWatcherStatus("acme", { state: "watching" });
    expect((await d.api.listProjects())[0].ci.watcher).toEqual({ state: "watching", since: expect.any(String) });
    // Only the service reports a crash.
    const crashed = await d.reportCIWatcherStatus("acme", { state: "error", problem: "crashed", message: "crashed" }).catch((e) => e);
    expect(crashed).toMatchObject({ code: "invalid" });

    await d.api.setCITracking("acme", { tracking: false, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
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
    expect(early).toMatchObject({ code: "not_found" });

    // Under Pull Request a push is recorded without CI Tracking.
    await d.reportPushedCommit("acme", id, "abc123", "impl-7");
    await d.reportPullRequest("acme", { ...report, state: "open" });
    expect(await d.api.getTicket("acme", id)).toMatchObject({ pull_request: { number: 7, state: "open" } });
    expect(await d.api.getTicket("acme", id)).not.toHaveProperty("ci_result");

    // A closed pull request reopened is accepted, and a merged one needs its merge commit.
    await d.reportPullRequest("acme", { ...report, state: "closed" });
    await d.reportPullRequest("acme", { ...report, state: "open" });
    const bare = await d.reportPullRequest("acme", { ...report, state: "merged" }).catch((e) => e);
    expect(bare).toMatchObject({ code: "invalid" });
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
    expect(early).toMatchObject({ code: "action_not_allowed" });

    // With CI Tracking off, a push isn't tracked, and no runs are taken.
    await d.reportPushedCommit("acme", id, "abc123");
    expect(await d.api.getTicket("acme", id)).not.toHaveProperty("pushed_commit");

    const off = await d.reportCIRuns("acme", []).catch((e) => e);
    expect(off).toMatchObject({ code: "action_not_allowed" });

    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"] });
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
    expect(concluded).toMatchObject({ code: "invalid" });
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
    expect(spec.tickets!.find((r) => r.id === id)?.ci_result).toBe("passed");
  });

  it("offers a CI failed Ticket Land anyway under Merge, and records the human's choice", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
    await d.api.act("acme", id, "accept", {});
    await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin", land_per_spec: false });
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
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
    // The Ticket links its commit's CI item, one per pushed commit,
    // listing the Tickets waiting on it (code-central-432).
    expect(failed.ci_item).toEqual({
      reason: "ci_failed",
      commit: "abc123",
      tickets: [{ id, title: "Login page", spec: failed.spec.id }],
      options: ["rerun_ci", "land_anyway", "send_back", "new_fix_ticket"],
    });

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
    expect(chosen).not.toHaveProperty("ci_item");
    expect(chosen.needs_me_reason).not.toBe("ci_failed");
  });

  it("passes a Ticket under the CI gate once any of its pushed commits passed (code-central-430)", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
    await d.api.act("acme", id, "accept", {});
    await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin", land_per_spec: false });
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    const run = (n: number, commit: string, status: "in_progress" | "completed", conclusion?: "success" | "failure") => ({
      id: n,
      attempt: 1,
      workflow: "ci.yml",
      name: "CI",
      branch: "main",
      commit,
      status,
      ...(conclusion && { conclusion }),
      url: `https://github.com/acme/web/actions/runs/${n}`,
      jobs: [],
      ticket: id,
    });
    await d.reportPushedCommit("acme", id, "abc123");
    await d.reportCIRuns("acme", [run(1, "abc123", "in_progress")]);
    await d.reportPushedCommit("acme", id, "def456");
    await d.reportCIRuns("acme", [run(2, "def456", "completed", "failure")]);
    expect(await d.api.getTicket("acme", id)).toMatchObject({ ci_result: "failed", ci_commit: "def456", pushed_commit: { commit: "def456" } });

    await d.reportCIRuns("acme", [run(1, "abc123", "completed", "success")]);
    const passed = await d.api.getTicket("acme", id);
    expect(passed).toMatchObject({ ci_result: "passed", ci_commit: "abc123", pushed_commit: { commit: "def456" } });
    expect(passed).not.toHaveProperty("ci_failure_options");
  });

  it("re-runs a CI failed Ticket's failed jobs: it waits on CI until the run's new attempt passes (code-central-265)", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
    await d.api.act("acme", id, "accept", {});
    await d.api.setLandingMethod("acme", { method: "branch", push: true, remote: "origin" });
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
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

  it("sends a CI failed Ticket back to fix with a note, or opens a New fix Ticket on the CI Fixes Spec that leaves it waiting (code-central-264, code-central-433)", async () => {
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
    await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin", land_per_spec: false });
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });

    const back = await failedTicket("Login page", 1);
    const noNote = await d.api.act("acme", back, "send_back", { note: " " }).catch((e) => e);
    expect(noNote).toMatchObject({ status: 422, code: "invalid" });
    const sent = await d.api.act("acme", back, "send_back", { note: "CI failed: runs/1" });
    expect(sent).toMatchObject({ id: back, state: "Ready" });
    const ready = await d.api.getTicket("acme", back);
    expect(ready.comments!.at(-1)).toMatchObject({ actor: "human", body: "CI failed: runs/1" });
    expect(ready).not.toHaveProperty("ci_failure_options");

    const fixed = await failedTicket("Sign-up page", 2);
    const ack = await d.api.act("acme", fixed, "new_fix_ticket", { note: "CI failed: runs/2" });
    expect(ack).toMatchObject({ state: "Ready" });
    expect(ack.id).not.toBe(fixed);
    const fix = await d.api.getTicket("acme", ack.id);
    expect(fix).toMatchObject({ state: "Ready", spec: { title: "CI Fixes" }, title: `Fix the CI failure of ${fixed}: Sign-up page` });
    expect(fix.description).toContain("CI failed: runs/2");
    const landing = await d.api.getTicket("acme", fixed);
    expect(landing).toMatchObject({ state: "Accepted" });
    expect(landing.land_anyway).toBeFalsy();
    expect(landing).not.toHaveProperty("ci_failure_options");
    expect(landing.comments!.at(-1)?.body).toContain(ack.id);
  });

  it("reads the CI view: each run's latest attempt, unfinished first, for one Project or All Projects (code-central-258)", async () => {
    await d.createProject("acme", "Acme");
    await d.createProject("quiet", "Quiet");
    const id = await d.createTicket("acme", "Pick a colour", "Ready");
    await d.finish("acme", id, "Done.", { branch: id, commit: "abc123" });
    await d.api.act("acme", id, "accept", {});
    await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"] });
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
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"] });
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
    // Under the CI gate old111, pushed before abc123, still counts
    // (code-central-430).
    expect(acme.runs.map((r) => [r.project, r.workflow, r.attempt])).toEqual([
      ["acme", "old.yml", 1],
      ["acme", "queued.yml", 1],
      ["acme", "ci.yml", 1],
    ]);
    expect((await d.api.ciOverview({ running: true })).runs).toEqual(acme.runs);
    expect((await d.api.ciOverview({ project: "acme" })).runs).toHaveLength(7);
    expect(await d.api.ciOverview({ project: "quiet", running: true })).toMatchObject({ runs: [] });
  });

  it("tracks CI per Repository: each one's entry, result and Name@sha item, whose options act on it alone (Spec code-central-848)", async () => {
    await d.createProject("acme", "Acme");
    await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin", land_per_spec: false });
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    const added = await d.api.addRepository("acme", { name: "api", clone_url: "git@github.com:acme/api.git" });
    expect(added.ci.repositories).toEqual([{ name: "api", not_watched: false, provider: "github_actions", repository: "acme/api" }]);
    const refused = await d.api.setRepositoryCI("acme", "acme", { not_watched: true }).catch((e) => e);
    expect(refused).toMatchObject({ code: "invalid", field: "name" });

    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123", repositories: [{ name: "api", branch: "impl-7", commit: "def456" }] });
    await d.api.act("acme", id, "accept", {});
    await d.reportPushedCommit("acme", id, "abc123");
    await d.reportPushedCommit("acme", id, "def456", undefined, "api");
    const run = (repository: string, commit: string, conclusion: "success" | "failure") => ({
      ...(repository && { repository }),
      id: 1,
      attempt: 1,
      workflow: "ci.yml",
      name: "CI",
      branch: "main",
      commit,
      status: "completed" as const,
      conclusion,
      url: "https://github.com/acme/web/actions/runs/1",
      jobs: [],
      ticket: id,
    });
    await d.reportCIRuns("acme", [run("", "abc123", "success"), run("api", "def456", "failure")]);

    const t = await d.api.getTicket("acme", id);
    expect(t.ci_result).toBe("failed");
    expect(t.repositories?.map((r) => [r.name, r.ci_result])).toEqual([
      ["acme", "passed"],
      ["api", "failed"],
    ]);
    expect(t.ci_item).toMatchObject({ reason: "ci_failed", repository: "api", commit: "def456" });
    expect((await d.api.needsMe()).groups.flatMap((g) => g.tickets).map((r) => r.title)).toEqual(["CI failed at api@def456"]);

    await d.api.act("acme", id, "land_anyway", { repository: "api" });
    const chosen = await d.api.getTicket("acme", id);
    expect(chosen.repositories?.map((r) => [r.name, !!r.land_anyway])).toEqual([
      ["acme", false],
      ["api", true],
    ]);

    // Not watched, api never counts.
    const off = await d.api.setRepositoryCI("acme", "api", { not_watched: true });
    expect(off.ci.repositories).toEqual([{ name: "api", not_watched: true }]);
    expect((await d.api.getTicket("acme", id)).ci_result).toBe("passed");
  });
}
