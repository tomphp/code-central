// A Ticket's CI section (code-central-256): its CI result and the CI Runs
// tracked against it, each workflow with its status, jobs and GitHub
// link. The latest pushed commit's runs come first; an earlier push's
// stay below, folded away. Ignored runs are greyed and marked ignored.
// While the Ticket is CI failed it ends with the failure options
// (code-central-261, code-central-264, code-central-265), when given the
// Project to act in. With several Repositories (Spec code-central-848) it
// shows the runs per Repository, each with its own CI result, pushed
// commit and failure options, which act on that Repository only.
import { useState } from "react";
import "../api/actions/land_anyway";
import "../api/actions/new_fix_ticket";
import "../api/actions/rerun_ci";
import "../api/actions/send_back";
import { projectOf } from "../api/api";
import type { CIFailureOption, CIItemView, CIResult, CIRunView, TicketView } from "../api/types";
import { CIRunEntry } from "../ci/CIRunEntry";
import { shortCommit } from "../format";
import { hrefFor } from "../url";
import { useAct } from "./useAct";
import { Button } from "../controls/Button";
import { useApi } from "../live";

export interface CISectionProps {
  ticket: TicketView;
  // project, when given, lets the human choose a failure option.
  project?: string;
}

type CIAutoFixView = NonNullable<TicketView["ci_auto_fix"]>;

// RepositoryCI is the CI of a Ticket in one Repository, or with the Home
// Repository alone, the Ticket's: its result, latest pushed commit, runs,
// and the failure options of its CI item there, with whether the human
// chose Land anyway. repository names it, left out with the Home
// Repository alone.
interface RepositoryCI {
  repository?: string;
  result?: CIResult;
  latest?: string;
  runs: CIRunView[];
  options?: CIFailureOption[];
  // item is its CI item, which carries the prefilled fix notes.
  item?: CIItemView;
  landAnyway?: boolean;
  // autoFix is what Auto-fix does about its failure, which Go derives.
  autoFix?: CIAutoFixView;
}

// repositoriesCI is t's CI per Repository, in landing order, those with
// runs only; with the Home Repository alone, the Ticket's own, whole.
function repositoriesCI(t: TicketView): RepositoryCI[] {
  const runs = t.ci_runs ?? [];
  if (!t.repositories?.length) {
    return [{ result: t.ci_result, latest: t.pushed_commit?.commit, runs, options: t.ci_failure_options, item: t.ci_item, landAnyway: t.land_anyway, autoFix: t.ci_auto_fix }];
  }
  // A run names its Repository, and leaves the Home Repository's out.
  const home = projectOf(t.id);
  const of = (name: string) => runs.filter((r) => (r.repository || home) === name);
  return t.repositories
    .filter((r) => of(r.name).length > 0)
    .map((r) => {
      const item = t.ci_items?.find((i) => i.repository === r.name);
      return {
        repository: r.name,
        result: r.ci_result,
        latest: r.pushed_commit?.commit,
        runs: of(r.name),
        options: item && item.reason !== "ci_unavailable" ? item.options : undefined,
        item,
        landAnyway: r.land_anyway,
        autoFix: r.ci_auto_fix,
      };
    });
}

// CISection is the Ticket's CI section, or nothing while it has no CI
// Runs.
export function CISection({ ticket: t, project }: CISectionProps) {
  if ((t.ci_runs ?? []).length === 0) return null;
  const repos = repositoriesCI(t);
  if (repos.length === 1 && !repos[0].repository) {
    const ci = repos[0];
    return (
      <section aria-label="CI" className="ci-section">
        <h3>
          CI <Result ci={ci} />
        </h3>
        <CIRepositoryRuns ci={ci} />
        <AutoFixLine autoFix={ci.autoFix} />
        {project && <CIFailureOptions project={project} ticket={t} ci={ci} />}
      </section>
    );
  }
  return (
    <section aria-label="CI" className="ci-section">
      <h3>
        CI {t.ci_result && <span className={`ci-result ci-${t.ci_result}`}>{t.ci_result}</span>}
      </h3>
      {repos.map((ci) => (
        <section key={ci.repository} aria-label={`CI in ${ci.repository}`} className="ci-repository">
          <h4>
            {ci.repository} <Result ci={ci} />
          </h4>
          <CIRepositoryRuns ci={ci} />
          <AutoFixLine autoFix={ci.autoFix} />
          {project && <CIFailureOptions project={project} ticket={t} ci={ci} />}
        </section>
      ))}
    </section>
  );
}

// Result is a CI result with the pushed commit it is about.
function Result({ ci }: { ci: RepositoryCI }) {
  return (
    <>
      {ci.result && <span className={`ci-result ci-${ci.result}`}>{ci.result}</span>} {ci.latest && <code>{shortCommit(ci.latest)}</code>}
    </>
  );
}

// FixTicketLink links an auto fix Ticket.
function FixTicketLink({ id }: { id: string }) {
  return <a href={hrefFor({ view: "inbox", ticket: id })}>{id}</a>;
}

// AutoFixLine is what Auto-fix does about a CI failure, as Go derived it
// (ADR 0063): the auto fix Ticket fixing it, the one covering it with the
// commit it was opened for, or the attempts it stopped after.
function AutoFixLine({ autoFix }: { autoFix?: CIAutoFixView }) {
  if (!autoFix) return null;
  const { fixing, covering, stopped } = autoFix;
  if (fixing) {
    return (
      <p className="small ci-auto-fix">
        Auto-fix is fixing it: <FixTicketLink id={fixing.fix_ticket} /> (attempt {fixing.attempt} of {fixing.limit}).
      </p>
    );
  }
  if (covering) {
    return (
      <p className="small ci-auto-fix">
        Auto-fix covers it: <FixTicketLink id={covering.fix_ticket} />, opened for <code>{shortCommit(covering.commit)}</code>.
      </p>
    );
  }
  if (stopped) {
    const n = stopped.attempts.length;
    return (
      <p className="small ci-auto-fix">
        Auto-fix stopped after {n === 1 ? "1 attempt" : `${n} attempts`}:{" "}
        {stopped.attempts.map((id, i) => (
          <span key={id}>
            {i > 0 && ", "}
            <FixTicketLink id={id} />
          </span>
        ))}
        .
      </p>
    );
  }
  return null;
}

// CIRepositoryRuns are ci's runs: its latest pushed commit's, then the
// earlier pushes', folded away.
function CIRepositoryRuns({ ci }: { ci: RepositoryCI }) {
  const current = ci.runs.filter((r) => r.commit === ci.latest);
  const earlier = ci.runs.filter((r) => r.commit !== ci.latest);
  return (
    <>
      <CIRuns runs={current} />
      {earlier.length > 0 && (
        <details>
          <summary>Earlier pushes ({earlier.length === 1 ? "1 run" : `${earlier.length} runs`})</summary>
          <CIRuns runs={earlier} />
        </details>
      )}
    </>
  );
}

function CIRuns({ runs }: { runs: CIRunView[] }) {
  return (
    <ul className="ci-runs">
      {runs.map((run) => (
        <CIRunEntry key={`${run.repository ?? ""}/${run.id}/${run.attempt}`} layout="item" run={run} />
      ))}
    </ul>
  );
}

// fitToContent grows a textarea to hold all its lines: on mount, and on
// each edit.
function fitToContent(el: HTMLTextAreaElement | null) {
  if (!el) return;
  el.style.height = "auto";
  el.style.height = `${el.scrollHeight}px`;
}

// FixNote is the editable note one fix option starts from.
function FixNote({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <label className="ci-fix-note">
      <span className="small muted">{label}</span>
      <textarea
        rows={2}
        value={value}
        onChange={(e) => {
          onChange(e.target.value);
          fitToContent(e.target);
        }}
        ref={fitToContent}
      />
    </label>
  );
}

// CIFailureOptions are the failure options of a Ticket's CI failed or
// CI cancelled item, which act on every Ticket it lists, in its
// Repository only:
// Re-run failed jobs under either Landing Method; Land anyway under
// Merge, where under Branch the human's merge decides; then under both,
// Send back to fix and New fix Ticket, which fix forward with
// a note each, which Go prefills (the item's send_back_note and
// new_fix_note) and the human edits. Once the human accepted the failed
// result (Land anyway or New fix Ticket, whose comment names the fix
// Ticket) it says so.
function CIFailureOptions({ project, ticket: t, ci }: { project: string; ticket: TicketView; ci: RepositoryCI }) {
  const api = useApi();
  const { busy, error, run } = useAct();
  const [sendBackNote, setSendBackNote] = useState(ci.item?.send_back_note ?? "");
  const [newFixNote, setNewFixNote] = useState(ci.item?.new_fix_note ?? "");
  if (ci.landAnyway && ci.result === "failed") {
    return <p className="small ci-options">You accepted its failed CI result, so CI no longer holds it back.</p>;
  }
  const options = ci.options;
  if (!options) return null;
  const fixes = options.includes("send_back") || options.includes("new_fix_ticket");
  // The options act on the item's Repository, which they name with
  // several.
  const where = ci.repository ? { repository: ci.repository } : {};
  return (
    <div role="group" aria-label={ci.repository ? `Failure options in ${ci.repository}` : "Failure options"} className="ci-options">
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
      {options.includes("rerun_ci") && (
        <>
          <p className="small">If it may be a flake, re-run the failed jobs: the CI Watcher asks GitHub to, and the Ticket waits on CI again.</p>
          <p>
            <Button type="button" disabled={busy} onClick={() => run(() => api.act(project, t.id, "rerun_ci", where))}>
              Re-run failed jobs
            </Button>
          </p>
        </>
      )}
      {options.includes("land_anyway") ? (
        <>
          <p className="small">
            If the failure is unrelated to this work, land it anyway: the Factory lands it{ci.repository ? ` in ${ci.repository}` : ""} on its next poll.
          </p>
          <p>
            <Button type="button" variant="primary" disabled={busy} onClick={() => run(() => api.act(project, t.id, "land_anyway", where))}>
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
          {options.includes("send_back") && (
            <>
              <FixNote label="Note for Send back to fix (required)" value={sendBackNote} onChange={setSendBackNote} />
              <p className="ci-fixes">
                <Button
                  type="button"
                  disabled={busy || !sendBackNote.trim()}
                  onClick={() => run(() => api.act(project, t.id, "send_back", { note: sendBackNote.trim() }))}
                >
                  Send back to fix
                </Button>
              </p>
            </>
          )}
          {options.includes("new_fix_ticket") && (
            <>
              <FixNote label="Note for New fix Ticket (required)" value={newFixNote} onChange={setNewFixNote} />
              <p className="ci-fixes">
                <Button
                  type="button"
                  disabled={busy || !newFixNote.trim()}
                  onClick={() => run(() => api.act(project, t.id, "new_fix_ticket", { note: newFixNote.trim(), ...where }))}
                >
                  New fix Ticket
                </Button>
              </p>
            </>
          )}
        </>
      )}
    </div>
  );
}
