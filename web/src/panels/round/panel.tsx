// The Round panel: every Question of the open Round at once, each with an
// answer box, Answer and Dismiss, plus Accept all recommended. A box starts
// as the saved answer, else the recommendation as an unsaved draft. Each
// action saves one Question (or every recommendation) at once; answers stay
// editable until the Round is complete, which moves the Ticket to Answered
// and wakes the waiting agent. After an action the panel does nothing: the
// service's change notice refetches the Ticket.
import "../../api/actions/rounds";
import type { QuestionView } from "../../api/types";
import { Markdown } from "../../Markdown";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { QuestionAnswer } from "./QuestionAnswer";
import { useAction } from "./useAction";
import "./round.css";

export const panel: PanelDef = { reasons: ["needs_answer"], Component: RoundPanel };

export function RoundPanel({ api, project, ticket }: PanelProps) {
  const accept = useAction();
  const round = ticket.rounds.at(-1);
  if (!round) return null;
  const open = round.questions.filter((q) => q.status === "open");

  return (
    <section aria-label={`Round ${round.number} Questions`} className="panel round-panel">
      <div className="round-head">
        <strong>Round {round.number}</strong>
        <span className="muted">
          {open.length} of {round.questions.length} open
        </span>
        {open.some((q) => q.recommended) && (
          <button
            className="btn"
            disabled={accept.busy}
            onClick={() => accept.run(() => api.act(project, ticket.id, "accept_recommended", { round: round.number }))}
          >
            Accept all recommended
          </button>
        )}
      </div>
      {accept.alert}
      {round.questions.map((q) => (
        <fieldset key={q.number} className={`question question-${q.status}`}>
          <legend>
            <span className="tid">Q{q.number}</span> {q.title}
          </legend>
          <span className={`qstatus qstatus-${q.status}`}>{STATUS[q.status]}</span>
          <div className="question-body">
            <Markdown>{q.body}</Markdown>
          </div>
          <QuestionAnswer api={api} project={project} ticket={ticket.id} q={q} />
        </fieldset>
      ))}
    </section>
  );
}

const STATUS: Record<QuestionView["status"], string> = { open: "Open", answered: "Answered", dismissed: "Dismissed" };
