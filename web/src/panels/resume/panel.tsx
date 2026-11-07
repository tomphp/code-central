// The Resume-then-Release panel, one for both an Answered Ticket no
// session has picked up and a Silent Claim: how long it has waited or been
// quiet, then the cheap, safe option (resume the session), then Release,
// which warns what it costs first.
import { useState } from "react";
import "../../api/actions/release";
import { ApiError, type RoundView, type TicketView } from "../../api/types";
import { ago, duration } from "../../format";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { ResumeCommand } from "./ResumeCommand";
import "./resume.css";

function ResumeRelease({ api, project, ticket: t, now }: PanelProps) {
  const answered = t.state === "Answered";
  const round = t.rounds.at(-1);
  return (
    <section aria-label="Resume or release" className="panel">
      <p>
        {answered
          ? `Answered ${ago(now, answeredAt(t))}, no session has picked it up.`
          : `Quiet for ${duration(now, quietSince(t))}.`}
      </p>
      <ResumeCommand sessionId={sessionOf(t)} />
      {answered && round && <Answers round={round} />}
      <Release api={api} project={project} ticket={t} />
    </section>
  );
}

// answeredAt is when the Ticket last moved to Answered.
function answeredAt(t: TicketView): string {
  return t.moves.filter((m) => m.to === "Answered").at(-1)?.time ?? t.updated;
}

// quietSince is the Silent Claim's last activity.
function quietSince(t: TicketView): string {
  return t.silent_since ?? t.claims.find((c) => !c.ended)?.last_activity ?? t.updated;
}

// sessionOf is the session to resume: the live Claim's (set_session keeps
// it current), else the one that posted the latest Round.
function sessionOf(t: TicketView): string | undefined {
  return t.claims.find((c) => !c.ended)?.session_id || t.rounds.at(-1)?.session_id || undefined;
}

function Answers({ round }: { round: RoundView }) {
  return (
    <details className="answers">
      <summary>Round {round.number} answers</summary>
      <ol>
        {round.questions.map((q) => (
          <li key={q.number}>
            <strong>{q.title}</strong>
            <div>{q.status === "dismissed" ? <em className="muted">Dismissed</em> : (q.answer ?? <em className="muted">Open</em>)}</div>
          </li>
        ))}
      </ol>
    </details>
  );
}

function Release({ api, project, ticket: t }: { api: PanelProps["api"]; project: string; ticket: TicketView }) {
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  if (!open) {
    return (
      <p>
        <button type="button" className="btn danger" onClick={() => setOpen(true)}>
          Release…
        </button>
      </p>
    );
  }

  // After a release, do nothing: the service's notice refetches the UI.
  const release = async () => {
    setBusy(true);
    setError("");
    try {
      const n = note.trim();
      await api.act(project, t.id, "release", n ? { note: n } : {});
    } catch (e) {
      setError(e instanceof ApiError || e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  };

  return (
    <div className="release">
      <p role="alert" className="warning">
        Releasing ends this Claim for good and moves {t.id} back to Ready: a live session's next write will be
        rejected, and the Claim can't be restored.
      </p>
      <label>
        <span className="small muted">Note (optional)</span>
        <textarea rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </label>
      {error && <p className="warning">{error}</p>}
      <p>
        <button type="button" className="btn danger" disabled={busy} onClick={release}>
          Release Claim
        </button>{" "}
        <button type="button" className="btn" disabled={busy} onClick={() => setOpen(false)}>
          Keep the Claim
        </button>
      </p>
    </div>
  );
}

export const panel: PanelDef = { reasons: ["answered", "silent_claim"], Component: ResumeRelease };
