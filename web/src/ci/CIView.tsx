// The CI view (Spec code-central-247, code-central-258): the CI Runs of
// one Project, or of every active Project with CI Tracking on. The CI
// Watchers' statuses head it; then the runs, in-progress ones first, then
// those finished in the last 7 days, each with its workflow, branch,
// short commit, status, duration, the Ticket or CI Rule it is tracked
// against and its GitHub link. Ignored runs are greyed and marked
// ignored. It follows the live feed, and a run that changes pulses. Back
// returns to the view it was opened from.
import { useRef } from "react";
import type { Api } from "../api/api";
import {
  CI_VIEW_DAYS,
  type CIProjectStatus,
  type CIRunRow,
  type CIRunView,
  type ProjectRow,
} from "../api/types";
import { usePulse } from "../focus/pulse";
import { useFetch } from "../live";
import { runStatus, shortCommit } from "../ticket/CISection";
import { clickTo, hrefFor, type UiLocation } from "../url";
import "./ci.css";

export interface CIViewProps {
  api: Api;
  now: Date;
  // version is the live feed's, so the runs follow every change.
  version: number;
  // projects are every Project, Archived ones too, for the scope picker.
  projects?: ProjectRow[];
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function CIView({
  api,
  now,
  version,
  projects = [],
  loc,
  navigate,
}: CIViewProps) {
  const back = loc.back?.view === "focus" ? "focus" : "inbox";
  const projectName = (key: string) =>
    projects.find((p) => p.key === key)?.display_name ?? key;

  return (
    <main aria-label="CI" className="ci">
      <header className="ci-head">
        <h1>CI</h1>
        <label>
          <span className="ci-picker-label">Projects</span>
          <select
            value={loc.project ?? ""}
            onChange={(e) => navigate({ project: e.target.value || undefined })}
          >
            <option value="">All Projects</option>
            {projects.map((p) => (
              <option key={p.key} value={p.key}>
                {p.archived ? `${p.display_name} (Archived)` : p.display_name}
              </option>
            ))}
            {loc.project && !projects.some((p) => p.key === loc.project) && (
              <option value={loc.project}>{loc.project}</option>
            )}
          </select>
        </label>
        <button
          className="btn ci-back"
          onClick={() => navigate(loc.back ?? { view: "inbox" })}
        >
          {back === "focus" ? "Back to Focus Mode" : "Back to the Inbox"}
        </button>
      </header>
      {/* Keyed by scope, so another scope starts from nothing and its first read doesn't pulse. */}
      <CIBody
        key={loc.project ?? ""}
        api={api}
        now={now}
        version={version}
        project={loc.project}
        projectName={projectName}
        navigate={navigate}
      />
    </main>
  );
}

interface CIBodyProps {
  api: Api;
  now: Date;
  version: number;
  project?: string;
  projectName(key: string): string;
  navigate(patch: Partial<UiLocation>): void;
}

// CIBody is one scope's CI Watchers and runs. The rows of its first read
// don't pulse; a run that joins later pulses once.
function CIBody({
  api,
  now,
  version,
  project,
  projectName,
  navigate,
}: CIBodyProps) {
  const ci = useFetch(
    () => api.ciOverview({ project }),
    [api, project, version],
  );
  const data = ci.data;
  const first = useRef<Set<string>>(undefined);
  if (data && !first.current) first.current = new Set(data.runs.map(runKey));

  return (
    <div className="ci-body">
      {ci.error && (
        <p role="alert" className="warning">
          {ci.error.message}
        </p>
      )}
      {data && (
        <>
          <section aria-label="CI Watchers" className="ci-watchers">
            {data.projects.length === 0 && (
              <p className="muted">
                No Project has CI Tracking on. Turn it on in a Project&apos;s
                Project Options.
              </p>
            )}
            {data.projects.map((p) => (
              <WatcherStatus
                key={p.project}
                status={p}
                name={projectName(p.project)}
                navigate={navigate}
              />
            ))}
          </section>
          {data.runs.length ? (
            <table aria-label="CI Runs" className="ci-table">
              <thead>
                <tr>
                  <th scope="col">Workflow</th>
                  <th scope="col">Branch</th>
                  <th scope="col">Commit</th>
                  <th scope="col">Status</th>
                  <th scope="col" className="num">
                    Duration
                  </th>
                  <th scope="col">Tracked against</th>
                  {!project && <th scope="col">Project</th>}
                  <th scope="col">GitHub</th>
                </tr>
              </thead>
              <tbody>
                {data.runs.map((r) => (
                  <RunRow
                    key={runKey(r)}
                    run={r}
                    now={now}
                    joined={!first.current?.has(runKey(r))}
                    projectName={project ? undefined : projectName(r.project)}
                    navigate={navigate}
                  />
                ))}
              </tbody>
            </table>
          ) : (
            <p className="muted">
              No CI Runs in progress or finished in the last {CI_VIEW_DAYS}{" "}
              days.
            </p>
          )}
        </>
      )}
    </div>
  );
}

// runKey names a run across Projects; a re-run's next attempt is the same run.
const runKey = (r: CIRunRow) => `${r.project}/${r.id}`;

interface WatcherStatusProps {
  status: CIProjectStatus;
  name: string;
  navigate(patch: Partial<UiLocation>): void;
}

// WatcherStatus is one Project's CI Watcher's status, as it last
// reported it, or that the Project has CI Tracking off.
function WatcherStatus({
  status: { project, tracking, provider, repository, project_url, watcher },
  name,
  navigate,
}: WatcherStatusProps) {
  const options: Partial<UiLocation> = { view: "project-options", project };
  if (!tracking) {
    return (
      <p className="muted">
        CI Tracking is off in {name}. Turn it on in its{" "}
        <a
          href={hrefFor({ view: "project-options", project })}
          onClick={clickTo(navigate, options)}
        >
          Project Options
        </a>
        .
      </p>
    );
  }
  switch (watcher.state) {
    case "watching":
      return (
        <p>
          {name}: Watching{" "}
          {provider === "azure_pipelines" ? project_url : repository}.
        </p>
      );
    case "not_reported":
      return (
        <p className="muted">
          {name}: Starting; the CI Watcher hasn&apos;t reported yet.
        </p>
      );
    case "error":
      return (
        <p className="warning">
          {name}: Error. {watcher.message}
          {watcher.fix && (
            <>
              <br />
              {watcher.fix}
            </>
          )}
        </p>
      );
  }
}

interface RunRowProps {
  run: CIRunRow;
  now: Date;
  // joined is a run that wasn't in the view's first read: it pulses once.
  joined: boolean;
  // projectName is the run's Project's name, in All Projects only.
  projectName?: string;
  navigate(patch: Partial<UiLocation>): void;
}

// RunRow is one run's row. It pulses when the run's status, conclusion
// or attempt changes.
function RunRow({ run, now, joined, projectName, navigate }: RunRowProps) {
  const status = runStatus(run);
  const pulse = usePulse(
    run.ignored ? "ci-row ignored" : "ci-row",
    `${run.status} ${run.conclusion ?? ""} ${run.attempt}`,
    joined,
  );
  const tracked = run.ticket ?? run.rule;
  const to: Partial<UiLocation> = run.ticket
    ? { view: "inbox", project: run.project, ticket: run.ticket }
    : { view: "inbox", project: run.project, requirement: run.rule };
  return (
    <tr {...pulse} aria-label={run.name || run.workflow}>
      <th scope="row">
        {run.name || run.workflow}{" "}
        <span className="muted small">{run.workflow}</span>
      </th>
      <td>{run.branch}</td>
      <td>
        <code>{shortCommit(run.commit)}</code>
      </td>
      <td>
        <span className={`ci-status ci-${status.replace(" ", "-")}`}>
          {status}
        </span>
        {run.attempt > 1 && (
          <span className="muted small"> · attempt {run.attempt}</span>
        )}
        {run.ignored && <span className="label">ignored</span>}
      </td>
      <td className="num">{runDuration(run, now)}</td>
      <td>
        {tracked && (
          <>
            {run.rule && "CI Rule "}
            <a
              href={hrefFor({ view: "inbox", ...to })}
              onClick={clickTo(navigate, to)}
            >
              {tracked}
            </a>
          </>
        )}
      </td>
      {projectName !== undefined && <td>{projectName}</td>}
      <td>
        <RunGitHubLink run={run} />
      </td>
    </tr>
  );
}

// RunGitHubLink opens a run on GitHub, in a new tab: in the CI view's
// rows and the Activity Dashboard's CI Running entries.
export function RunGitHubLink({
  run,
  className,
}: {
  run: CIRunView;
  className?: string;
}) {
  return (
    <a href={run.url} target="_blank" rel="noreferrer" className={className}>
      View on GitHub
    </a>
  );
}

// runDuration is how long a run took, or has taken so far: "45s",
// "3m 12s", "1h 5m"; "–" before it starts.
export function runDuration(run: CIRunRow, now: Date): string {
  if (!run.started) return "–";
  const end =
    run.status === "completed" && run.finished
      ? Date.parse(run.finished)
      : now.getTime();
  const s = Math.max(0, Math.round((end - Date.parse(run.started)) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}
