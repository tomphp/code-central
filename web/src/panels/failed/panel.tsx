// The Failed panel: how many times the Ticket has failed, the latest
// Failure Report's four fields, then the three ways forward (Send back
// with an optional note, To Draft, Cancel). Below the moves, the report's
// session to resume for investigating, and the earlier reports collapsed.
import { useState } from "react";
import "../../api/actions/cancel";
import "../../api/actions/send_back";
import "../../api/actions/to_draft";
import type { FailureReportView } from "../../api/types";
import { ago } from "../../format";
import { Markdown } from "../../Markdown";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { ResumeCommand } from "../resume/ResumeCommand";
import { useAct } from "../../ticket/useAct";
import { Button } from "../../controls/Button";
import "./failed.css";

export function FailedPanel({ api, project, ticket: t, now }: PanelProps) {
  const [note, setNote] = useState("");
  const { busy, error, run } = useAct();
  const reports = t.failure_reports;
  const latest = reports.at(-1);
  const earlier = reports.slice(0, -1).reverse();
  const n = reports.length;

  return (
    <section aria-label="Failed" className="panel failed-panel">
      <h3>{n === 1 ? "Failed once" : `Failed ${n} times`}</h3>
      {latest ? <Report report={latest} now={now} /> : <p className="muted">No Failure Report is recorded.</p>}

      <label className="send-back-note">
        <span className="small muted">Note for the next agent (optional)</span>
        <textarea rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
        <span className="small muted">No note tells the next agent nothing has changed.</span>
      </label>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
      <p className="moves">
        <Button
          type="button"
          variant="primary"
          disabled={busy}
          onClick={() => run(() => api.act(project, t.id, "send_back", note.trim() ? { note: note.trim() } : {}))}
        >
          Send back
        </Button>
        <Button type="button"  disabled={busy} onClick={() => run(() => api.act(project, t.id, "to_draft", {}))}>
          To Draft
        </Button>
        <Button type="button" variant="danger" disabled={busy} onClick={() => run(() => api.act(project, t.id, "cancel", {}))}>
          Cancel
        </Button>
      </p>

      <div className="investigate">
        <span className="small muted">Investigate in the session that reported it</span>
        <ResumeCommand sessionId={latest?.session_id} workingDir={latest?.working_dir} resumeCommand={latest?.resume_command} />
      </div>
      {earlier.length > 0 && (
        <details className="earlier">
          <summary>Earlier reports ({earlier.length})</summary>
          {earlier.map((r) => (
            <Report key={r.number} report={r} now={now} />
          ))}
        </details>
      )}
    </section>
  );
}

// Report is one Failure Report's four fields, each rendered as Markdown
// inside its definition.
function Report({ report: r, now }: { report: FailureReportView; now: Date }) {
  const label = `Failure Report ${r.number}`;
  return (
    <div role="group" aria-label={label} className="report">
      <p className="small muted">
        {label} · {r.actor} · {ago(now, r.time)}
      </p>
      <dl>
        <dt>What broke</dt>
        <dd>
          <Markdown>{r.what_broke}</Markdown>
        </dd>
        <dt>Why</dt>
        <dd>
          <Markdown>{r.why}</Markdown>
        </dd>
        <dt>What to fix</dt>
        <dd>
          <Markdown>{r.what_to_fix}</Markdown>
        </dd>
        <dt>Where the work stands</dt>
        <dd>
          <Markdown>{r.where_work_stands}</Markdown>
        </dd>
      </dl>
    </div>
  );
}

export const panel: PanelDef = { reasons: ["failed"], Component: FailedPanel };
