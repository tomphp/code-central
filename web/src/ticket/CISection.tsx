// A Ticket's CI section (code-central-256): its CI result and the CI Runs
// tracked against it, each workflow with its status, jobs and GitHub
// link. The latest pushed commit's runs come first; an earlier push's
// stay below, folded away. Ignored runs are greyed and marked ignored.
// While the Ticket is CI failed it ends with the failure options
// (code-central-261), when given the Api to act with.
import "../api/actions/land_anyway";
import type { Api } from "../api/api";
import type { CIConclusion, CIJobView, CIRunView, TicketView } from "../api/types";
import { useAct } from "./useAct";

// CONCLUSION_LABEL says how a finished run or job ended.
const CONCLUSION_LABEL: Record<CIConclusion, string> = {
  success: "passed",
  failure: "failed",
  cancelled: "cancelled",
  timed_out: "timed out",
  skipped: "skipped",
  neutral: "neutral",
};

// runStatus is a run's status as the CI section says it.
export function runStatus(run: CIRunView): string {
  if (run.status === "queued") return "queued";
  if (run.status === "in_progress" || !run.conclusion) return "running";
  return CONCLUSION_LABEL[run.conclusion];
}

const runName = (run: CIRunView) => run.name || run.workflow;
const shortCommit = (commit: string) => commit.slice(0, 7);

const jobStatus = (job: CIJobView) => (job.conclusion ? CONCLUSION_LABEL[job.conclusion] : "running");

export interface CISectionProps {
  ticket: TicketView;
  // api and project, when given, let the human choose a failure option.
  api?: Api;
  project?: string;
}

// CISection is the Ticket's CI section, or nothing while it has no CI
// Runs.
export function CISection({ ticket: t, api, project }: CISectionProps) {
  const runs = t.ci_runs ?? [];
  if (runs.length === 0) return null;
  const latest = t.pushed_commit?.commit;
  const current = runs.filter((r) => r.commit === latest);
  const earlier = runs.filter((r) => r.commit !== latest);
  return (
    <section aria-label="CI" className="ci-section">
      <h3>
        CI{" "}
        {t.ci_result && (
          <span className={`ci-result ci-${t.ci_result}`}>{t.ci_result}</span>
        )}{" "}
        {latest && <code>{shortCommit(latest)}</code>}
      </h3>
      <CIRuns runs={current} />
      {earlier.length > 0 && (
        <details>
          <summary>Earlier pushes ({earlier.length === 1 ? "1 run" : `${earlier.length} runs`})</summary>
          <CIRuns runs={earlier} />
        </details>
      )}
      {api && project && <CIFailureOptions api={api} project={project} ticket={t} />}
    </section>
  );
}

function CIRuns({ runs }: { runs: CIRunView[] }) {
  return (
    <ul className="ci-runs">
      {runs.map((run) => {
        const status = runStatus(run);
        return (
          <li key={`${run.id}/${run.attempt}`} className={run.ignored ? "ci-run ignored" : "ci-run"} aria-label={runName(run)}>
            <div>
              <a href={run.url} target="_blank" rel="noreferrer">
                {runName(run)}
              </a>{" "}
              <span className="muted small">{run.workflow}</span> <span className={`ci-status ci-${status.replace(" ", "-")}`}>{status}</span>
              {run.attempt > 1 && <span className="muted small"> · attempt {run.attempt}</span>}
              {run.commit && <span className="muted small"> · {shortCommit(run.commit)}</span>}
              {run.ignored && <span className="label">ignored</span>}
            </div>
            {run.jobs.length > 0 && (
              <ul className="ci-jobs small">
                {run.jobs.map((job, i) => (
                  <li key={i}>
                    <a href={job.url} target="_blank" rel="noreferrer">
                      {job.name}
                    </a>{" "}
                    <span className="muted">{jobStatus(job)}</span>
                  </li>
                ))}
              </ul>
            )}
          </li>
        );
      })}
    </ul>
  );
}

// CIFailureOptions are the failure options of a CI failed Ticket: Land
// anyway under Merge; under Branch the human's merge decides. Once the
// human chose Land anyway it says the Factory lands it.
function CIFailureOptions({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const { busy, error, run } = useAct();
  if (t.land_anyway && t.ci_result === "failed") {
    return <p className="small ci-options">You chose Land anyway: the Factory lands it at its pushed commit on its next poll.</p>;
  }
  const options = t.ci_failure_options;
  if (!options) return null;
  return (
    <div role="group" aria-label="Failure options" className="ci-options">
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
      {options.includes("land_anyway") ? (
        <>
          <p className="small">If the failure is unrelated to this work, say a flake, land it anyway: the Factory lands it on its next poll.</p>
          <p>
            <button type="button" className="btn primary" disabled={busy} onClick={() => run(() => api.act(project, t.id, "land_anyway", {}))}>
              Land anyway
            </button>
          </p>
        </>
      ) : (
        <p className="small">This Project lands by Branch, so your merge decides: merge the branch to land it whatever its CI result.</p>
      )}
    </div>
  );
}
