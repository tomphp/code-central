// Fake data for the Options sections' stories: a Project as the service
// reads it, and a FakeApi holding the same Project so Save works in the
// browser. Nothing but the stories imports this.
import { FakeApi } from "../api/fake";
import type { ProjectRow } from "../api/types";

export const storyProject = (patch: Partial<ProjectRow> = {}): ProjectRow => ({
  key: "acme",
  display_name: "Acme",
  created: "2026-11-01T09:00:00Z",
  mode: "committed",
  archived: false,
  landing: {
    method: "merge",
    push: false,
    remote: "origin",
    land_per_spec: true,
    code_host: "",
    repository: "",
    pat_env: "ADO_PAT",
    base: "main",
    watcher: { state: "not_reported" },
    open_pull_requests: [],
  },
  ci: {
    tracking: true,
    provider: "github_actions",
    repository: "acme/web",
    project_url: "",
    pat_env: "ADO_PAT",
    ignored_workflows: ["nightly.yml"],
    push_remote_url: "git@github.com:acme/web.git",
    rules: [
      {
        workflow: "ci.yml",
        branch: "main",
        state: "Draft",
        spec: "acme-12",
        created: "2026-11-02T09:00:00Z",
      },
    ],
    watcher: { state: "watching" },
  },
  github_issues: {
    on: true,
    repository: "acme/web",
    label: "bug",
    new_issues_need_me: true,
    spec: "acme-3",
    watcher: { state: "watching" },
  },
  models: {
    default: { model: "sonnet", effort: "" },
    kinds: [
      { kind: "implementation", model: "", effort: "high" },
      { kind: "review", model: "", effort: "" },
    ],
  },
  reported_models: {
    effort_levels: ["low", "medium", "high"],
    aliases: ["sonnet", "opus"],
    model_ids: ["claude-sonnet-5-5"],
    model_flag: "",
    effort_flag: "",
  },
  stage_guides: { subagent_exploration: true, model_assignment: false },
  focus_kinds: { off: [] },
  factory_status: {
    status: "online",
    factories: [
      {
        name: "factory-1",
        host: "dev-box",
        status: "online",
        note: "",
        last_heartbeat: "2026-11-13T12:00:00Z",
        interval_seconds: 30,
      },
    ],
  },
  needs_me: 0,
  unfinished: 0,
  ...patch,
});

// storyApi is a FakeApi that holds the story Project, for the sections to
// save to.
export function storyApi(): FakeApi {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  return fake;
}
