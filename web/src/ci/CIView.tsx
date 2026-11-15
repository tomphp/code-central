// The CI view (Spec code-central-247, code-central-258): the CI Runs of
// one Project, or of every active Project with CI Tracking on. The CI
// Watchers' statuses head it; then the runs, in-progress ones first, then
// those finished in the last 7 days, each with its workflow, branch,
// short commit, status, duration, the Ticket or CI Rule it is tracked
// against and its link to the CI system. Ignored runs are greyed and marked
// ignored. It follows the live feed, and a run that changes pulses. Back
// returns to the view it was opened from.
import { useRef } from "react";
import type { Api } from "../api/api";
import { backLabel } from "../controls/backLabel";
import { Empty, Loading } from "../controls/Empty";
import { ErrorBanner } from "../controls/ErrorBanner";
import { projectName } from "../controls/ProjectName";
import { ProjectPicker } from "../controls/ProjectPicker";
import { useProjectScope } from "../projectScope";
import { ViewHeader } from "../controls/ViewHeader";
import { CI_VIEW_DAYS, type CIOverview, type CIProjectStatus, type ProjectRow } from "../api/types";
import { useFetch } from "../live";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { Button } from "../controls/Button";
import { CIRunEntry, runKey } from "./CIRunEntry";
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

export function CIView({ api, now, version, projects = [], loc, navigate }: CIViewProps) {
  const { project, sole, resolved } = useProjectScope();
  const nameOf = (key: string) => projectName(projects, key);

  return (
    <main aria-label="CI" className="ci view-shell">
      <ViewHeader
        title="CI"
        left={<ProjectPicker projects={projects} value={project} withoutAll={sole} onChange={(project) => navigate({ project })} />}
        right={
          <Button  onClick={() => navigate(loc.back ?? { view: "inbox" })}>
            {backLabel(loc)}
          </Button>
        }
      />
      {/* Keyed by scope, so another scope starts from nothing and its first read doesn't pulse. */}
      <ScopedBody key={project ?? ""} api={api} now={now} version={version} resolved={resolved} project={project} projectName={nameOf} navigate={navigate} />
    </main>
  );
}

interface ScopedBodyProps {
  api: Api;
  now: Date;
  version: number;
  // resolved says the scope is known; the first read waits for it.
  resolved: boolean;
  project?: string;
  projectName(key: string): string;
  navigate(patch: Partial<UiLocation>): void;
}

// ScopedBody reads one scope's CI overview and hands it to CIBody.
function ScopedBody({ api, now, version, resolved, project, projectName, navigate }: ScopedBodyProps) {
  const ci = useFetch(resolved ? () => api.ciOverview({ project }) : null, [api, resolved, project, version]);
  return <CIBody overview={ci.data} error={ci.error?.message} now={now} project={project} projectName={projectName} navigate={navigate} />;
}

export interface CIBodyProps {
  // overview is the read, or undefined while it loads.
  overview?: CIOverview;
  error?: string;
  now: Date;
  // project is the scope's Project; left out, the scope is All Projects.
  project?: string;
  projectName(key: string): string;
  navigate(patch: Partial<UiLocation>): void;
}

// CIBody is one scope's CI Watchers and runs. The rows of its first read
// don't pulse; a run that joins later pulses once.
export function CIBody({ overview: data, error, now, project, projectName, navigate }: CIBodyProps) {
  const first = useRef<Set<string>>(undefined);
  if (data && !first.current) first.current = new Set(data.runs.map(runKey));

  return (
    <div className="view-body">
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {!data && !error && <Loading />}
      {data && (
        <>
          <section aria-label="CI Watchers" className="ci-watchers">
            {data.projects.length === 0 && <p className="muted">No Project has CI Tracking on. Turn it on in a Project&apos;s Project Options.</p>}
            {data.projects.map((p) => (
              <WatcherStatus key={p.project} status={p} name={projectName(p.project)} navigate={navigate} />
            ))}
          </section>
          {data.runs.length ? (
            <table aria-label="CI Runs" className="data-table">
              <thead>
                <tr>
                  <th scope="col">Workflow</th>
                  <th scope="col">Branch</th>
                  <th scope="col">Commit</th>
                  <th scope="col">Status</th>
                  <th scope="col" className="num">Duration</th>
                  <th scope="col">Tracked against</th>
                  {!project && <th scope="col">Project</th>}
                  <th scope="col">Link</th>
                </tr>
              </thead>
              <tbody>
                {data.runs.map((r) => (
                  <CIRunEntry
                    key={runKey(r)}
                    layout="row"
                    run={r}
                    provider={data.projects.find((p) => p.project === r.project)?.provider}
                    now={now}
                    joined={!first.current?.has(runKey(r))}
                    projectName={project ? undefined : projectName(r.project)}
                    navigate={navigate}
                  />
                ))}
              </tbody>
            </table>
          ) : (
            <Empty>No CI Runs in progress or finished in the last {CI_VIEW_DAYS} days.</Empty>
          )}
        </>
      )}
    </div>
  );
}

interface WatcherStatusProps {
  status: CIProjectStatus;
  name: string;
  navigate(patch: Partial<UiLocation>): void;
}

// WatcherStatus is one Project's CI Watcher's status, as it last
// reported it, or that the Project has CI Tracking off.
function WatcherStatus({ status: { project, tracking, provider, repository, project_url, watcher }, name, navigate }: WatcherStatusProps) {
  const options: Partial<UiLocation> = { view: "project-options", project };
  if (!tracking) {
    return (
      <p className="muted">
        CI Tracking is off in {name}. Turn it on in its{" "}
        <a href={hrefFor({ view: "project-options", project })} onClick={clickTo(navigate, options)}>
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
          {name}: Watching {provider === "azure_pipelines" ? project_url : repository}.
        </p>
      );
    case "not_reported":
      return <p className="muted">{name}: Starting; the CI Watcher hasn&apos;t reported yet.</p>;
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
