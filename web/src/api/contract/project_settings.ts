import { beforeEach, expect, it } from "vitest";
import type { CIProvider, LandingMethod, NeedsMeReason } from "../types";
import { ApiError, NEEDS_ME_REASONS } from "../types";
import type { Driver } from "../driver";
import { defaultCI, defaultLanding } from "../scenario";
import { wantAcknowledgement, withoutHost } from "./support";

export function projectSettingsContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
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
    expect((await d.api.listProjects())[0].landing).toEqual(
      defaultLanding({ method: "branch", push: true, remote: "upstream", repository_landing: "branch_push" }),
    );

    // Without Push, an empty remote reads as origin.
    expect((await d.api.setLandingMethod("acme", { method: "merge", push: false, remote: "" })).landing).toEqual(defaultLanding());

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

  it("keeps Rebase off by default, sets it, and keeps it when a later set leaves it out", async () => {
    await d.createProject("acme", "Acme");
    expect((await d.api.listProjects())[0].landing.rebase).toBe(false);
    const on = await d.api.setLandingMethod("acme", { method: "merge", push: false, remote: "", rebase: true });
    expect(on.landing.rebase).toBe(true);
    // Left out, the stored value stays, and it is shown whatever the method.
    const kept = await d.api.setLandingMethod("acme", { method: "branch", push: false, remote: "" });
    expect(kept.landing).toMatchObject({ method: "branch", rebase: true });
    expect(kept.landing.methods.branch.offers_rebase).toBe(false);
    expect(kept.landing.methods.merge.offers_rebase).toBe(true);
    const off = await d.api.setLandingMethod("acme", { method: "merge", push: false, remote: "", rebase: false });
    expect(off.landing.rebase).toBe(false);
  });

  it("refuses turning CI Tracking on when the CI check fails, with its reason and fix", async () => {
    await d.createProject("acme", "Acme");
    await d.logOutOfGH();
    const refused = await d.api
      .setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] })
      .catch((e) => e);
    expect(refused).toBeInstanceOf(ApiError);
    expect(refused).toMatchObject({ status: 422, code: "invalid", field: "tracking" });
    expect(refused.message).toContain("isn't logged in");
    expect(refused.fix).toContain("gh auth login");
    expect((await d.api.listProjects())[0].ci.tracking).toBe(false);
  });

  it("sets a Project's GitHub Issues settings, off by default and checked when turned on", async () => {
    await d.createProject("acme", "Acme");
    const off = { on: false, repositories: [], label: "", new_issues_need_me: false };
    const notReported = { state: "not_reported" };
    expect((await d.api.listProjects())[0].github_issues).toEqual({ ...off, watcher: notReported });

    const none = await d.api.setGitHubIssues("acme", { ...off, on: true, repositories: [" "] }).catch((e) => e);
    expect(none).toMatchObject({ status: 422, code: "invalid", field: "repositories" });
    expect(none.message).toContain("owner/name");
    const url = await d.api.setGitHubIssues("acme", { ...off, repositories: ["https://github.com/acme/web"] }).catch((e) => e);
    expect(url).toMatchObject({ status: 422, code: "invalid", field: "repositories" });
    const twice = await d.api.setGitHubIssues("acme", { ...off, repositories: ["acme/web", "Acme/Web"] }).catch((e) => e);
    expect(twice).toMatchObject({ status: 422, code: "invalid", field: "repositories" });

    // Off, nothing is checked and no Issues Spec is made.
    await d.logOutOfGH();
    const kept = await d.api.setGitHubIssues("acme", { on: false, repositories: [" acme/web "], label: " bug ", new_issues_need_me: true });
    expect(kept.github_issues).toEqual({ on: false, repositories: ["acme/web"], label: "bug", new_issues_need_me: true, watcher: notReported });

    const refused = await d.api.setGitHubIssues("acme", { ...kept.github_issues, on: true }).catch((e) => e);
    expect(refused).toBeInstanceOf(ApiError);
    expect(refused).toMatchObject({ status: 422, code: "invalid", field: "on" });
    expect(refused.message).toContain("isn't logged in");
    expect(refused.fix).toContain("gh auth login");
    expect((await d.api.listProjects())[0].github_issues).toEqual(kept.github_issues);
  });

  it("sets a Project's Azure Boards settings, off by default, keeping each mode's, with the Azure Boards Spec made once", async () => {
    await d.createProject("acme", "Acme");
    const off = {
      on: false, mode: "saved_query" as const, query_url: "", project_url: "", where: "", pat_env: "ADO_PAT", new_issues_need_me: false, hide_code_central: false, branch_format: "",
      work_item_states: { in_progress: "", accepted: "", done: "" },
      watcher: { state: "not_reported" as const },
    };
    expect((await d.api.listProjects())[0].azure_boards).toEqual(off);

    const none = await d.api.setAzureBoards("acme", { ...off, on: true }).catch((e) => e);
    expect(none).toMatchObject({ status: 422, code: "invalid", field: "query_url" });
    const select = await d.api
      .setAzureBoards("acme", { ...off, mode: "wiql", project_url: "https://dev.azure.com/contoso/web", where: "SELECT [System.Id] FROM workitems" })
      .catch((e) => e);
    expect(select).toMatchObject({ status: 422, code: "invalid", field: "where" });

    const query = "https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10";
    if (!d.canReachAzureDevOps) {
      // The check refuses turning it on without a PAT, and the Project is left as it was.
      const refused = await d.api.setAzureBoards("acme", { ...off, on: true, query_url: query }).catch((e) => e);
      expect(refused).toMatchObject({ status: 422, code: "invalid", field: "on" });
      expect(refused.message).toContain("ADO_PAT");
      expect((await d.api.listProjects())[0].azure_boards).toEqual(off);
      return;
    }
    const on =await d.api.setAzureBoards("acme", {
      ...off, on: true, query_url: ` ${query}/ `, project_url: "https://dev.azure.com/contoso/api/_boards", where: " [System.State] = 'Approved' ",
    });
    expect(on.azure_boards).toEqual({
      ...off, on: true, query_url: query, project_url: "https://dev.azure.com/contoso/api", where: "[System.State] = 'Approved'", spec: expect.stringMatching(/^acme-\d+$/),
    });
    const spec = on.azure_boards.spec!;
    const s = await d.api.getSpec("acme", spec);
    expect(s.title).toBe("Azure Boards: contoso/web");
    expect(s.labels).toEqual(["long-lived"]);
    expect(s.link).toBeUndefined();

    // Switching mode keeps the Saved query's URL, and the title follows
    // the WIQL filter's project; off and on again reuses the Spec.
    const wiql = await d.api.setAzureBoards("acme", { ...on.azure_boards, mode: "wiql" });
    expect(wiql.azure_boards).toEqual({ ...on.azure_boards, mode: "wiql" });
    expect((await d.api.getSpec("acme", spec)).title).toBe("Azure Boards: contoso/api");
    await d.api.setAzureBoards("acme", { ...wiql.azure_boards, on: false });
    expect((await d.api.setAzureBoards("acme", wiql.azure_boards)).azure_boards.spec).toBe(spec);
  });

  it("sets a Project's Subagent Exploration, on to begin with", async () => {
    await d.createProject("acme", "Acme");
    expect((await d.api.listProjects())[0].stage_guides).toEqual({ subagent_exploration: true, model_assignment: false });
    expect((await d.api.setSubagentExploration("acme", false)).stage_guides).toEqual({ subagent_exploration: false, model_assignment: false });
    expect((await d.api.listProjects())[0].stage_guides).toEqual({ subagent_exploration: false, model_assignment: false });
    expect((await d.api.setSubagentExploration("acme", true)).stage_guides).toEqual({ subagent_exploration: true, model_assignment: false });
  });

  it("sets a Project's Model Assignment, off to begin with, apart from Subagent Exploration", async () => {
    await d.createProject("acme", "Acme");
    expect((await d.api.listProjects())[0].stage_guides.model_assignment).toBe(false);
    expect((await d.api.setModelAssignment("acme", true)).stage_guides).toEqual({ subagent_exploration: true, model_assignment: true });
    expect((await d.api.listProjects())[0].stage_guides.model_assignment).toBe(true);
    expect((await d.api.setSubagentExploration("acme", false)).stage_guides).toEqual({ subagent_exploration: false, model_assignment: true });
    expect((await d.api.setModelAssignment("acme", false)).stage_guides).toEqual({ subagent_exploration: false, model_assignment: false });
  });

  it("sets a Project's Auto-accept per kind, all off to begin with, keeping the kinds left out", async () => {
    await d.createProject("acme", "Acme");
    const off = { grilling: false, spec: false, tickets: false, implementation: false, merge: false };
    expect((await d.api.listProjects())[0].auto_accept).toEqual(off);
    expect((await d.api.setAutoAccept("acme", { implementation: true, merge: true })).auto_accept).toEqual({ ...off, implementation: true, merge: true });
    expect((await d.api.setAutoAccept("acme", { merge: false, grilling: true })).auto_accept).toEqual({ ...off, implementation: true, grilling: true });
    expect((await d.api.listProjects())[0].auto_accept).toEqual({ ...off, implementation: true, grilling: true });
  });

  it("sets a Project's Focus Kinds, every reason on to begin with, replacing the off list whole", async () => {
    await d.createProject("acme", "Acme");
    expect((await d.api.listProjects())[0].focus_kinds).toEqual({ off: [] });

    // Kept in Needs Me's group order, each once.
    const set = await d.api.setFocusKinds("acme", ["in_review", "answered", "in_review"]);
    expect(set.focus_kinds).toEqual({ off: ["answered", "in_review"] });
    expect((await d.api.listProjects())[0].focus_kinds).toEqual(set.focus_kinds);
    expect((await d.api.setFocusKinds("acme", ["failed"])).focus_kinds).toEqual({ off: ["failed"] });

    const bad = await d.api.setFocusKinds("acme", ["answered", "lunch" as NeedsMeReason]).catch((e) => e);
    expect(bad).toBeInstanceOf(ApiError);
    expect(bad).toMatchObject({ status: 422, code: "invalid", field: "off" });
    expect(bad.message).toContain("lunch");
    expect((await d.api.listProjects())[0].focus_kinds).toEqual({ off: ["failed"] });

    // NEEDS_ME_REASONS are the service's reasons, in its group order.
    expect((await d.api.setFocusKinds("acme", [...NEEDS_ME_REASONS])).focus_kinds).toEqual({ off: NEEDS_ME_REASONS });
    expect((await d.api.setFocusKinds("acme", [])).focus_kinds).toEqual({ off: [] });

    // An Archived Project refuses before the reasons are checked.
    await d.api.archiveProject("acme");
    for (const off of [["answered"], ["lunch"]] as NeedsMeReason[][]) {
      const refused = await d.api.setFocusKinds("acme", off).catch((e) => e);
      expect(refused).toMatchObject({ status: 409, code: "action_not_allowed", allowed_actions: ["unarchive_project"] });
    }
  });

  it("sets a Project's Models, a row per kind of work, all at Default to begin with", async () => {
    await d.createProject("acme", "Acme");
    const kinds = ["grilling", "spec", "tickets", "map-chart", "map-grilling", "map-research", "map-task", "prototype", "retro", "implementation"];
    const unset = { model: "", effort: "" };
    expect((await d.api.listProjects())[0].models).toEqual({ default: unset, chat: unset, kinds: kinds.map((kind) => ({ kind, ...unset })) });

    const set = await d.api.setModels("acme", {
      default: { model: " opus ", effort: "high" },
      chat: { model: " sonnet ", effort: "low" },
      kinds: [{ kind: "implementation", model: "", effort: "xhigh" }],
    });
    expect(set.models.default).toEqual({ model: "opus", effort: "high" });
    // The Chat row is its own row, not one of the kinds of work.
    expect(set.models.chat).toEqual({ model: "sonnet", effort: "low" });
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
    const chatEffort = await d.api.setModels("acme", { default: unset, chat: { model: "", effort: "extreme" }, kinds: [] }).catch((e) => e);
    expect(chatEffort).toMatchObject({ status: 422, code: "invalid", field: "chat.effort" });
    const chatBlank = await d.api.setModels("acme", { default: unset, chat: { model: " ", effort: "" }, kinds: [] }).catch((e) => e);
    expect(chatBlank).toMatchObject({ status: 422, code: "invalid", field: "chat.model" });
    expect((await d.api.listProjects())[0].models).toEqual(set.models);

    // The rows are replaced whole, the Chat row too.
    const cleared = await d.api.setModels("acme", { default: unset, kinds: [] });
    expect(cleared.models).toEqual({ default: unset, chat: unset, kinds: kinds.map((kind) => ({ kind, ...unset })) });
  });

  it("serves factory_status: offline with no Factories, then each Factory from its Heartbeat", async () => {
    await d.createProject("acme", "Acme");
    expect((await d.api.listProjects())[0].factory_status).toEqual({ status: "offline", factories: [] });

    await d.reportHeartbeat("acme", { factory: "code-central-factory", host: "laptop", state: "online", note: "1 of 3 turns running", interval_seconds: 30 });
    const online = (await d.api.listProjects())[0].factory_status;
    expect(online.status).toBe("online");
    expect(online.factories).toEqual([
      { name: "code-central-factory", host: "laptop", status: "online", note: "1 of 3 turns running", last_heartbeat: expect.any(String), interval_seconds: 30, chat: false },
    ]);

    await d.reportHeartbeat("acme", { factory: "code-central-factory", host: "laptop", state: "online", interval_seconds: 30, chat: true });
    expect((await d.api.listProjects())[0].factory_status.factories).toMatchObject([{ chat: true }]);

    await d.reportHeartbeat("acme", { factory: "code-central-factory", host: "laptop", state: "stopping", note: "stopped: drained", interval_seconds: 30 });
    const stopped = (await d.api.listProjects())[0].factory_status;
    expect(stopped.status).toBe("offline");
    expect(stopped.factories).toMatchObject([{ status: "offline", note: "stopped: drained" }]);
  });

  it("serves the Plan Limits a Heartbeat carries, replaced whole by the next, and refuses bad figures", async () => {
    await d.createProject("acme", "Acme");
    const beat = { factory: "code-central-factory", host: "laptop", state: "online" as const, interval_seconds: 30 };
    const plan_limits = {
      observed_at: "2026-01-01T09:00:00Z",
      five_hour: { used_percent: 42.5, resets_at: "2026-01-01T12:00:00Z" },
      weekly: { used_percent: 7, resets_at: "2026-01-05T09:00:00Z" },
    };
    await d.reportHeartbeat("acme", beat);
    expect((await d.api.listProjects())[0].factory_status.plan_limits).toBeUndefined();

    await d.reportHeartbeat("acme", { ...beat, plan_limits });
    expect((await d.api.listProjects())[0].factory_status.plan_limits).toEqual(plan_limits);

    const over = await d.reportHeartbeat("acme", { ...beat, plan_limits: { ...plan_limits, weekly: { used_percent: 101, resets_at: "2026-01-05T09:00:00Z" } } }).catch((e) => e);
    expect(over).toMatchObject({ status: 422, code: "invalid", field: "plan_limits.weekly.used_percent" });

    await d.reportHeartbeat("acme", beat);
    expect((await d.api.listProjects())[0].factory_status.plan_limits).toBeUndefined();
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

  it("creates the Issues Spec the first time GitHub Issues is turned on, retitled with the repositories", async () => {
    await d.createProject("acme", "Acme");
    await d.letGHTriage("acme/web");
    const on = await d.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web"], label: "", new_issues_need_me: false });
    // The service's own Issue Watcher may report by the time it reads.
    expect(on.github_issues).toEqual({
      on: true,
      repositories: ["acme/web"],
      label: "",
      new_issues_need_me: false,
      spec: expect.stringMatching(/^acme-\d+$/),
      watcher: expect.anything(),
    });
    const spec = on.github_issues.spec!;
    const s = await d.api.getSpec("acme", spec);
    expect(s.title).toBe("GitHub Issues: acme/web");
    expect(s.link).toBeUndefined();

    await d.api.setGitHubIssues("acme", { on: false, repositories: ["acme/web"], label: "", new_issues_need_me: false });
    expect((await d.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web"], label: "", new_issues_need_me: false })).github_issues.spec).toBe(spec);

    await d.letGHTriage("acme/site");
    await d.api.setGitHubIssues("acme", { on: true, repositories: ["acme/site"], label: "", new_issues_need_me: false });
    expect((await d.api.getSpec("acme", spec)).title).toBe("GitHub Issues: acme/site");

    // Listing more than one repository titles it "GitHub Issues", and each
    // one added passes the check.
    await d.letGHTriage("acme/api");
    await d.api.setGitHubIssues("acme", { on: true, repositories: ["acme/site", "acme/api"], label: "", new_issues_need_me: false });
    expect((await d.api.getSpec("acme", spec)).title).toBe("GitHub Issues");
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

  it("lists the Home Repository alone, then adds, edits, reorders and removes Repositories", async () => {
    await d.createProject("acme", "Acme");
    const names = (p: { repositories: { name: string }[] }) => p.repositories.map((r) => r.name);
    // A local path is absolute where Core runs, as d.absolutePath is.
    const local = (name: string) => d.absolutePath.replace(/[^\\/]+$/, name);
    // A Repository follows the Default Landing with the Project's Push
    // remote until it has its own.
    const following = { landing: "merge", own_landing: false, push_remote: "origin", own_push_remote: false, local_only_under_pull_request: false };
    // The Home isn't local-only until the Factory reports its clone has no
    // Push remote; its CI, filled in from no Clone URL, is Not watched.
    expect((await d.api.listProjects())[0].repositories).toEqual([
      { name: "acme", home: true, clone_url: "", local_only: false, base: "main", local_path: "", code_host: "", changed_by: [], ...following,
        ci: { name: "acme", not_watched: true } },
    ]);

    const added = await d.api.addRepository("acme", { name: "web", clone_url: " git@github.com:acme/web.git " });
    expect(added.repositories[1]).toEqual({
      name: "web", home: false, clone_url: "git@github.com:acme/web.git", local_only: false, base: "main", local_path: "", code_host: "github", changed_by: [], ...following,
      ci: { name: "web", not_watched: false, provider: "github_actions", repository: "acme/web" },
    });
    const own = await d.api.editRepository("acme", "web", { landing: "branch_push", push_remote: "upstream" });
    expect(own.repositories[1]).toMatchObject({ landing: "branch_push", own_landing: true, push_remote: "upstream", own_push_remote: true });
    await d.api.editRepository("acme", "web", { landing: "default", push_remote: "" });
    await d.api.addRepository("acme", { name: "api", clone_url: "https://github.com/acme/api", base: "develop", local_path: local("api") });
    expect(names((await d.api.listProjects())[0])).toEqual(["acme", "web", "api"]);

    const edited = await d.api.editRepository("acme", "web", { base: "trunk", local_path: local("web") });
    expect(edited.repositories[1]).toMatchObject({ name: "web", clone_url: "git@github.com:acme/web.git", base: "trunk", local_path: local("web") });
    expect(names(await d.api.setLandingOrder("acme", ["api", "acme", "web"]))).toEqual(["api", "acme", "web"]);
    expect(names(await d.api.removeRepository("acme", "web"))).toEqual(["api", "acme"]);

    // Each refusal names its field and says how to put it right.
    const twice = await d.api.addRepository("acme", { name: "shop", clone_url: "git@github.com:Acme/API.git" }).catch((e) => e);
    expect(twice).toMatchObject({ status: 422, code: "invalid", field: "clone_url" });
    expect(twice.fix).toContain("edit api");
    const home = await d.api.removeRepository("acme", "acme").catch((e) => e);
    expect(home).toMatchObject({ status: 422, code: "invalid", field: "name" });
    expect(home.fix).toBeTruthy();
    const homeBase = await d.api.editRepository("acme", "acme", { base: "trunk" }).catch((e) => e);
    expect(homeBase).toMatchObject({ status: 422, code: "invalid", field: "base" });
    expect(homeBase.fix).toContain("Landing");
    const order = await d.api.setLandingOrder("acme", ["api"]).catch((e) => e);
    expect(order).toMatchObject({ status: 422, code: "invalid", field: "names" });
    expect(await d.api.editRepository("acme", "web", { base: "main" }).catch((e) => e)).toMatchObject({ status: 404, code: "not_found" });
  });

  it("sets a Project's Azure Pipelines settings, validating them by provider", async () => {
    await d.createProject("acme", "Acme");
    const azure = { provider: "azure_pipelines" as CIProvider, repository: "", ignored_workflows: [] };

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
    });
    const on = defaultCI({
      tracking: true,
      repository: "acme/web",
      ignored_workflows: ["nightly.yml", "deploy.yaml"],
      push_remote_url: "git@github.com:acme/web.git",
    });
    expect(v).toMatchObject({ key: "acme", ci: on });
    const { ci } = (await d.api.listProjects())[0];
    expect({ ...ci, watcher: withoutHost(ci.watcher) }).toEqual(on);

    const none = await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: " ", ignored_workflows: [] }).catch((e) => e);
    expect(none).toMatchObject({ status: 422, code: "invalid", field: "repository" });
    expect(none.message).toContain("owner/name");
    const url = await d.api
      .setCITracking("acme", { tracking: true, provider: "github_actions", repository: "https://github.com/acme/web", ignored_workflows: [] })
      .catch((e) => e);
    expect(url).toMatchObject({ status: 422, code: "invalid", field: "repository" });
    const path = await d.api
      .setCITracking("acme", { tracking: false, provider: "github_actions", repository: "", ignored_workflows: [".github/workflows/ci.yml"] })
      .catch((e) => e);
    expect(path).toMatchObject({ status: 422, code: "invalid", field: "ignored_workflows" });
    const provider = await d.api
      .setCITracking("acme", { tracking: false, provider: "circleci" as CIProvider, repository: "", ignored_workflows: [] })
      .catch((e) => e);
    expect(provider).toMatchObject({ status: 422, code: "invalid", field: "provider" });

    // Turning off runs no check, so a logged-out gh doesn't stop it.
    await d.logOutOfGH();
    const off = await d.api.setCITracking("acme", { tracking: false, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    expect(off.ci).toMatchObject({ tracking: false });

    // Off, the repository may be empty.
    const empty = (await d.api.setCITracking("acme", { tracking: false, provider: "github_actions", repository: "", ignored_workflows: [] })).ci;
    expect({ ...empty, watcher: withoutHost(empty.watcher) }).toEqual(defaultCI({ push_remote_url: "git@github.com:acme/web.git" }));
  });
}
