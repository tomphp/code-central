// The Resume-then-Release panel, one for an Answered Ticket no session has
// picked up and for a Silent or Long Claim: how long it has waited, been
// quiet or gone without a move, then the cheap, safe option (resume the
// session), then Release, which warns what it costs first.
import type { RoundView, TicketView } from "../../api/types";
import { ago, duration } from "../../format";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { Release } from "./Release";
import { ResumeCommand, sessionOf } from "./ResumeCommand";
import "./resume.css";

function ResumeRelease({ project, ticket: t, now }: PanelProps) {
  const answered = t.state === "Answered";
  const round = t.rounds?.at(-1);
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
      <Release project={project} ticket={t} />
    </section>
  );
}

// answeredAt is when the Ticket last moved to Answered.
function answeredAt(t: TicketView): string {
  return t.moves?.filter((m) => m.to === "Answered").at(-1)?.time ?? t.updated;
}

// movedAt is when the Long Claim last moved into In Progress.
function movedAt(t: TicketView): string {
  return t.moves?.filter((m) => m.to === "In Progress").at(-1)?.time ?? t.claims?.find((c) => !c.ended)?.started ?? t.updated;
}

// quietSince is the Silent Claim's last activity.
function quietSince(t: TicketView): string {
  return t.silent_since ?? t.claims?.find((c) => !c.ended)?.last_activity ?? t.updated;
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

export const panel: PanelDef = { reasons: ["answered", "silent_claim", "long_claim"], Component: ResumeRelease };
