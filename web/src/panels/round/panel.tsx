// The Round panel: every Question of the open Round at once, each with an
// answer box, Answer and Dismiss, plus Accept all recommended. A box starts
// as the saved answer, else the recommendation as an unsaved draft. Each
// action saves one Question (or every recommendation) at once; answers stay
// editable until the Round is complete, which moves the Ticket to Answered
// and wakes the waiting agent. After an action the panel does nothing: the
// service's change notice refetches the Ticket.
import { useState } from "react";
import "../../api/actions/rounds";
import type { QuestionView } from "../../api/types";
import type { PanelDef, PanelProps } from "../ActionPanel";
import "./round.css";

export const panel: PanelDef = { reasons: ["needs_answer"], Component: RoundPanel };

// useAction runs one request at a time and keeps its refusal to show.
function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const run = async (f: () => Promise<unknown>): Promise<boolean> => {
    setBusy(true);
    try {
      await f();
      setError(undefined);
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setBusy(false);
    }
  };
  const alert = error && (
    <p role="alert" className="warning">
      {error}
    </p>
  );
  return { busy, alert, run };
}

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
        <QuestionForm
          key={q.number}
          q={q}
          answer={(text) => api.act(project, ticket.id, "answer", { question: q.number, text })}
          dismiss={() => api.act(project, ticket.id, "dismiss_question", { question: q.number })}
        />
      ))}
    </section>
  );
}

const STATUS: Record<QuestionView["status"], string> = { open: "Open", answered: "Answered", dismissed: "Dismissed" };

function QuestionForm({ q, answer, dismiss }: { q: QuestionView; answer: (text: string) => Promise<unknown>; dismiss: () => Promise<unknown> }) {
  const { busy, alert, run } = useAction();
  // draft is what the human typed since the last save; until then the box
  // shows the saved answer, else the recommendation.
  const [draft, setDraft] = useState<string>();
  const saved = q.status === "answered" ? (q.answer ?? "") : (q.recommended ?? "");
  const text = draft ?? saved;
  const isRecommendation = q.status !== "answered" && !!q.recommended && text === q.recommended;
  const save = (f: () => Promise<unknown>) => run(f).then((ok) => ok && setDraft(undefined));
  const boxId = `question-${q.number}-answer`;

  return (
    <fieldset className={`question question-${q.status}`}>
      <legend>
        <span className="tid">Q{q.number}</span> {q.title}
      </legend>
      <span className={`qstatus qstatus-${q.status}`}>{STATUS[q.status]}</span>
      <div className="question-body">{q.body}</div>
      <label htmlFor={boxId} className={isRecommendation ? "draft-note" : "visually-hidden"}>
        {isRecommendation ? "Recommended answer, a draft: nothing is saved until you answer" : "Your answer"}
      </label>
      <textarea id={boxId} rows={2} value={text} onChange={(e) => setDraft(e.target.value)} />
      {alert}
      <div className="question-actions">
        <button
          className="btn primary"
          disabled={busy || text === "" || (q.status === "answered" && text === saved)}
          onClick={() => save(() => answer(text))}
        >
          {q.status === "answered" ? "Update answer" : "Answer"}
        </button>
        {q.status !== "dismissed" && (
          <button className="btn" disabled={busy} onClick={() => save(dismiss)}>
            Dismiss
          </button>
        )}
      </div>
    </fieldset>
  );
}
