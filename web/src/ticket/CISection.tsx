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
import type { CIConclusion, CIRunView, TicketView } from "../api/types";
import { CIRunEntry } from "../ci/CIRunEntry";
import { runName } from "../ci/RunParts";
import { shortCommit } from "../format";
import { useAct } from "./useAct";
import { Button } from "../controls/Button";

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
      {runs.map((run) => (
        <CIRunEntry key={`${run.id}/${run.attempt}`} layout="item" run={run} />
      ))}
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

// CIFailureOptions are the failure options of a Ticket's CI failed or
// CI cancelled item, which act on every Ticket it lists:
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
            <Button type="button"  disabled={busy} onClick={() => run(() => api.act(project, t.id, "rerun_ci", {}))}>
              Re-run failed jobs
            </Button>
          </p>
        </>
      )}
      {options.includes("land_anyway") ? (
        <>
          <p className="small">If the failure is unrelated to this work, land it anyway: the Factory lands it on its next poll.</p>
          <p>
            <Button type="button" variant="primary" disabled={busy} onClick={() => run(() => api.act(project, t.id, "land_anyway", {}))}>
              Land anyway
            </Button>
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
            To fix it, send it back for the next agent to fix on its branch, or open a new fix Ticket on the CI Fixes Spec, which this one waits for. Either
            way the fix goes on top: nothing is reverted.
          </p>
          <label className="ci-fix-note">
            <span className="small muted">Note for the fix (required)</span>
            <textarea rows={5} value={note} onChange={(e) => setNote(e.target.value)} />
          </label>
          <p className="ci-fixes">
            {options.includes("send_back") && (
              <Button
                type="button"

                disabled={busy || noNote}
                onClick={() => run(() => api.act(project, t.id, "send_back", { note: note.trim() }))}
              >
                Send back to fix
              </Button>
            )}
            {options.includes("new_fix_ticket") && (
              <Button
                type="button"

                disabled={busy || noNote}
                onClick={() => run(() => api.act(project, t.id, "new_fix_ticket", { note: note.trim() }))}
              >
                New fix Ticket
              </Button>
            )}
          </p>
        </>
      )}
    </div>
  );
}
