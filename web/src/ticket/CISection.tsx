// A Ticket's CI section (code-central-256): its CI result and the CI Runs
// tracked against it, each workflow with its status, jobs and GitHub
// link. The latest pushed commit's runs come first; an earlier push's
// stay below, folded away. Ignored runs are greyed and marked ignored.
import type { CIConclusion, CIJobView, CIRunView, TicketView } from "../api/types";

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

// CISection is the Ticket's CI section, or nothing while it has no CI
// Runs.
export function CISection({ ticket: t }: { ticket: TicketView }) {
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
