// A Ticket's CI section (code-central-256): its CI result and the CI Runs
// tracked against it, each workflow with its status, jobs and GitHub
// link. The latest pushed commit's runs come first; an earlier push's
// stay below, folded away. Ignored runs are greyed and marked ignored.
// While the Ticket is CI failed it ends with the failure options
// (code-central-261, code-central-264, code-central-265), when given the
// Api to act with.
import { useState } from "react";
import "../api/actions/land_anyway";
import "../api/actions/new_fix_ticket";
import "../api/actions/rerun_ci";
import "../api/actions/send_back";
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
  partially_succeeded: "Partially succeeded",
};

// runStatus is a run's status as the CI section says it.
export function runStatus(run: CIRunView): string {
  if (run.status === "queued") return "queued";
  if (run.status === "in_progress" || !run.conclusion) return "running";
  return CONCLUSION_LABEL[run.conclusion];
}

// statusClass is the CSS class of a status label, such as "ci-timed-out".
const statusClass = (status: string) => `ci-${status.toLowerCase().replace(" ", "-")}`;

// RunStatusBadge is a status as a badge. A partially succeeded one has the
// passed styling plus a warning mark.
export function RunStatusBadge({ status }: { status: string }) {
  return (
    <span className={`ci-status ${statusClass(status)}`}>
      {status === CONCLUSION_LABEL.partially_succeeded && <span aria-hidden="true">⚠ </span>}
      {status}
    </span>
  );
}

// runName is the name a run shows: its workflow's display name, or its file.
export const runName = (run: CIRunView) => run.name || run.workflow;
// shortCommit is a commit as runs show it: its first 7 characters.
export const shortCommit = (commit: string) => commit.slice(0, 7);

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
              <span className="muted small">{run.workflow}</span> <RunStatusBadge status={status} />
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

// FAILED is the conclusions that fail a run or job, as core counts them;
// a cancelled one is no failure.
const FAILED: CIConclusion[] = ["failure", "timed_out"];
const failed = (c?: CIConclusion) => c !== undefined && FAILED.includes(c);

// fixNote is the note Send back to fix and New fix Ticket start from: each
// failed run of the latest pushed commit, linked, with its failed jobs'
// names. No logs: the next agent reads them with gh.
function fixNote(t: TicketView): string {
  const latest = t.pushed_commit?.commit ?? "";
  const runs = new Map<number, CIRunView>();
  for (const r of t.ci_runs ?? []) {
    if (r.commit !== latest || r.ignored) continue;
    if ((runs.get(r.id)?.attempt ?? 0) < r.attempt) runs.set(r.id, r);
  }
  // A CI cancelled Ticket's note names its cancelled runs instead.
  const bad = t.ci_result === "cancelled" ? (c?: CIConclusion) => c === "cancelled" : failed;
  const failedRuns = [...runs.values()].filter((r) => bad(r.conclusion));
  const lines = failedRuns.map((r) => {
    const jobs = r.jobs.filter((j) => bad(j.conclusion)).map((j) => j.name).sort();
    return `- [${runName(r)}](${r.url}) (${r.workflow})${jobs.length > 0 ? `: failed jobs ${jobs.join(", ")}` : ""}`;
  });
  const logs = failedRuns.map((r) => `\`gh run view ${r.id} --log-failed\``).join(", ");
  return [`CI failed at ${shortCommit(latest)}:`, lines.join("\n"), ...(logs ? [`Read the failed jobs' logs with ${logs}.`] : [])].join("\n\n");
}

// CIFailureOptions are the failure options of a CI failed Ticket:
// Re-run failed jobs under either Landing Method; Land anyway under
// Merge, where under Branch the human's merge decides; then under both,
// Send back to fix and New fix Ticket, which fix forward with
// the note. Once the human accepted the failed result (Land anyway or
// New fix Ticket, whose comment names the fix Ticket) it says so.
function CIFailureOptions({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const { busy, error, run } = useAct();
  const [note, setNote] = useState(() => fixNote(t));
  if (t.land_anyway && t.ci_result === "failed") {
    return <p className="small ci-options">You accepted its failed CI result, so CI no longer holds it back.</p>;
  }
  const options = t.ci_failure_options;
  if (!options) return null;
  const fixes = options.includes("send_back") || options.includes("new_fix_ticket");
  const noNote = !note.trim();
  return (
    <div role="group" aria-label="Failure options" className="ci-options">
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
      {options.includes("rerun_ci") && (
        <>
          <p className="small">If it may be a flake, re-run the failed jobs: the CI Watcher asks GitHub to, and the Ticket waits on CI again.</p>
          <p>
            <button type="button" className="btn" disabled={busy} onClick={() => run(() => api.act(project, t.id, "rerun_ci", {}))}>
              Re-run failed jobs
            </button>
          </p>
        </>
      )}
      {options.includes("land_anyway") ? (
        <>
          <p className="small">If the failure is unrelated to this work, land it anyway: the Factory lands it on its next poll.</p>
          <p>
            <button type="button" className="btn primary" disabled={busy} onClick={() => run(() => api.act(project, t.id, "land_anyway", {}))}>
              Land anyway
            </button>
          </p>
        </>
      ) : (
        <p className="small">
          {t.pull_request ? (
            <>This Project lands by Pull Request, so your merge decides: merge pull request #{t.pull_request.number} to land it whatever its CI result.</>
          ) : (
            <>
              This Project lands by Branch, so your merge decides: merge {t.branch ? <code>{t.branch}</code> : "the branch"} into the base to land
              it whatever its CI result.
            </>
          )}
        </p>
      )}
      {fixes && (
        <>
          <p className="small">
            To fix it, send it back for the next agent to fix on its branch, or open a new fix Ticket on its Spec and let this one land. Either
            way the fix goes on top: nothing is reverted.
          </p>
          <label className="ci-fix-note">
            <span className="small muted">Note for the fix (required)</span>
            <textarea rows={5} value={note} onChange={(e) => setNote(e.target.value)} />
          </label>
          <p className="ci-fixes">
            {options.includes("send_back") && (
              <button
                type="button"
                className="btn"
                disabled={busy || noNote}
                onClick={() => run(() => api.act(project, t.id, "send_back", { note: note.trim() }))}
              >
                Send back to fix
              </button>
            )}
            {options.includes("new_fix_ticket") && (
              <button
                type="button"
                className="btn"
                disabled={busy || noNote}
                onClick={() => run(() => api.act(project, t.id, "new_fix_ticket", { note: note.trim() }))}
              >
                New fix Ticket
              </button>
            )}
          </p>
        </>
      )}
    </div>
  );
}
