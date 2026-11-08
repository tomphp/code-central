// A Question's answer form, shared by the Round panel and Focus Mode so a
// Question's rules live in one place: the answer box, starting as the
// saved answer, else the recommendation as an unsaved draft (labelled as
// such), then Answer and Dismiss, which send answer and dismiss_question
// for that one Question, any further buttons the caller adds, and the
// refusal alert. The caller wraps it with the Question's title and body.
import { useState, type KeyboardEvent, type ReactNode } from "react";
import type { Api } from "../../api/api";
import "../../api/actions/rounds";
import type { QuestionView } from "../../api/types";
import "./round.css";
import { useAction } from "./useAction";

export interface QuestionAnswerProps {
  api: Api;
  project: string;
  ticket: string;
  q: QuestionView;
  // submitOnModEnter makes Cmd/Ctrl+Enter in the box send Answer.
  submitOnModEnter?: boolean;
  // children are further buttons, after Answer and Dismiss.
  children?: ReactNode;
}

export function QuestionAnswer({ api, project, ticket, q, submitOnModEnter, children }: QuestionAnswerProps) {
  const { busy, alert, run } = useAction();
  // draft is what the human typed since the last save; until then the box
  // shows the saved answer, else the recommendation.
  const [draft, setDraft] = useState<string>();
  const saved = q.status === "answered" ? (q.answer ?? "") : (q.recommended ?? "");
  const text = draft ?? saved;
  const isRecommendation = q.status !== "answered" && !!q.recommended && text === q.recommended;
  const canAnswer = !busy && text !== "" && !(q.status === "answered" && text === saved);
  const save = (f: () => Promise<unknown>) => run(f).then((ok) => ok && setDraft(undefined));
  const answer = () => save(() => api.act(project, ticket, "answer", { question: q.number, text }));
  const dismiss = () => save(() => api.act(project, ticket, "dismiss_question", { question: q.number }));
  const onKeyDown = (e: KeyboardEvent) => {
    if (!submitOnModEnter || e.key !== "Enter" || !(e.metaKey || e.ctrlKey)) return;
    e.preventDefault();
    if (canAnswer) answer();
  };
  const boxId = `question-${q.number}-answer`;

  return (
    <>
      <label htmlFor={boxId} className={isRecommendation ? "draft-note" : "visually-hidden"}>
        {isRecommendation ? "Recommended answer, a draft: nothing is saved until you answer" : "Your answer"}
      </label>
      <textarea id={boxId} rows={2} value={text} onChange={(e) => setDraft(e.target.value)} onKeyDown={onKeyDown} />
      {alert}
      <div className="question-actions">
        <button className="btn primary" disabled={!canAnswer} onClick={answer}>
          {q.status === "answered" ? "Update answer" : "Answer"}
        </button>
        {q.status !== "dismissed" && (
          <button className="btn" disabled={busy} onClick={dismiss}>
            Dismiss
          </button>
        )}
        {children}
      </div>
    </>
  );
}
