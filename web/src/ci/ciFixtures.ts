// Plain CI overview data for the CI stories and prop tests: CIBody's,
// CIRunEntry's and the CI Run parts'.
import type { CIProjectStatus, CIRunRow } from "../api/types";

export const NOW = new Date("2026-03-01T12:00:00Z");
const NAMES: Record<string, string> = { acme: "Acme", beta: "Beta" };
export const projectName = (key: string) => NAMES[key] ?? key;

export const status = (project: string, over: Partial<CIProjectStatus> = {}): CIProjectStatus => ({
  project,
  tracking: true,
  provider: "github_actions",
  repository: `${project}/web`,
  project_url: "",
  watcher: { state: "watching" },
  ...over,
});

export const finished = (project: string, id: number, over: Partial<CIRunRow> = {}): CIRunRow => ({
  project,
  id,
  attempt: 1,
  workflow: "ci.yml",
  name: "CI",
  branch: "main",
  commit: "1abcdef0123456",
  status: "completed",
  conclusion: "success",
  started: "2026-03-01T11:50:00Z",
  finished: "2026-03-01T11:53:12Z",
  url: "https://example.com/run",
  jobs: [],
  ignored: false,
  ...over,
});

export const running = (project: string, id: number): CIRunRow =>
  finished(project, id, { status: "in_progress", conclusion: undefined, finished: undefined, started: "2026-03-01T11:57:00Z" });
