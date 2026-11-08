// The Resume-then-Release panel, one for an Answered Ticket no session has
// picked up and for a Silent or Long Claim: how long it has waited, been
// quiet or gone without a move, then the cheap, safe option (resume the
// session), then Release, which warns what it costs first. On a Silent or
// Long Claim, release reads "Stop": the session may still be running.
import { useState } from "react";
import "../../api/actions/release";
import { ApiError, type RoundView, type TicketView } from "../../api/types";
import { ago, duration } from "../../format";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { ResumeCommand, type Session } from "./ResumeCommand";
import "./resume.css";

function ResumeRelease({ api, project, ticket: t, now }: PanelProps) {
  const answered = t.state === "Answered";
  const round = t.rounds.at(-1);
  return (
    <section aria-label="Resume or release" className="panel">
      <p>
        {answered
          ? `Answered ${ago(now, answeredAt(t))}, no session has picked it up.`
          : t.needs_me_reason === "long_claim"
            ? `Active, but no move for ${duration(now, movedAt(t))}.`
            : `Quiet for ${duration(now, quietSince(t))}.`}
      </p>
      <ResumeCommand {...sessionOf(t)} />
      {answered && round && <Answers round={round} />}
      <Release api={api} project={project} ticket={t} verb={answered ? "Release" : "Stop"} />
    </section>
  );
}

// answeredAt is when the Ticket last moved to Answered.
function answeredAt(t: TicketView): string {
  return t.moves.filter((m) => m.to === "Answered").at(-1)?.time ?? t.updated;
}

// movedAt is when the Long Claim last moved into In Progress.
function movedAt(t: TicketView): string {
  return t.moves.filter((m) => m.to === "In Progress").at(-1)?.time ?? t.claims.find((c) => !c.ended)?.started ?? t.updated;
}

// quietSince is the Silent Claim's last activity.
function quietSince(t: TicketView): string {
  return t.silent_since ?? t.claims.find((c) => !c.ended)?.last_activity ?? t.updated;
}

// sessionOf is the session to resume: the live Claim's (set_session keeps
// it current), its session ID falling back to the one that posted the
// latest Round.
function sessionOf(t: TicketView): Session {
  const live = t.claims.find((c) => !c.ended);
  return {
    sessionId: live?.session_id || t.rounds.at(-1)?.session_id || undefined,
    workingDir: live?.working_dir,
    resumeCommand: live?.resume_command,
  };
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

// Release is the release action, labelled verb: "Release", or "Stop" on a
// Silent or Long Claim. Either way it is the same release.
function Release({
  api,
  project,
  ticket: t,
  verb,
}: {
  api: PanelProps["api"];
  project: string;
  ticket: TicketView;
  verb: "Release" | "Stop";
}) {
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  if (!open) {
    return (
      <p>
        <button type="button" className="btn danger" onClick={() => setOpen(true)}>
          {verb}…
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
        {verb === "Stop" ? "Stopping" : "Releasing"} ends this Claim for good and moves {t.id} back to Ready: a live session's next write will be
        rejected, and the Claim can't be restored.
      </p>
      <label>
        <span className="small muted">Note (optional)</span>
        <textarea rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </label>
      {error && <p className="warning">{error}</p>}
      <p>
        <button type="button" className="btn danger" disabled={busy} onClick={release}>
          {verb} Claim
        </button>{" "}
        <button type="button" className="btn" disabled={busy} onClick={() => setOpen(false)}>
          Keep the Claim
        </button>
      </p>
    </div>
  );
}

export const panel: PanelDef = { reasons: ["answered", "silent_claim", "long_claim"], Component: ResumeRelease };
