// The In Review panel: the agent's finish summary, then Accept (on a spec
// Stage, Split into Tickets and Implement, with the Spec recommendation
// highlighted), and Reject, which can't be sent without a comment saying
// what to change.
import { Fragment, useId, useState } from "react";
import "../../api/actions/accept";
import "../../api/actions/implement";
import "../../api/actions/reject";
import type { CommentView, TicketView } from "../../api/types";
import { Markdown } from "../../Markdown";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { useAct } from "../../ticket/useAct";
import { Button } from "../../controls/Button";
import "./review.css";

// specChoices are a spec Stage's two ways to accept, each with the Spec
// recommendation that names it. With a recommendation only that choice is
// primary, marked Recommended beside it; with none, both are primary.
const specChoices = [
  { recommendation: "split", action: "accept", label: "Split into Tickets" },
  { recommendation: "implement", action: "implement", label: "Implement" },
] as const satisfies readonly { recommendation: NonNullable<TicketView["recommendation"]>; action: string; label: string }[];

export function ReviewPanel({ api, project, ticket: t }: PanelProps) {
  const [comment, setComment] = useState("");
  const { busy, error, run } = useAct();
  const summary = finishSummary(t);
  const canReject = comment.trim() !== "" && !busy;
  const recommendedId = useId();

  return (
    <section aria-label="In Review" className="panel review-panel">
      <h3>Review the finished work</h3>
      {summary ? (
        <div aria-label="Summary" className="summary">
          <Markdown>{summary.body}</Markdown>
        </div>
      ) : (
        <p className="muted">No finish summary is recorded.</p>
      )}
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
      <p>
        {t.labels.includes("stage:spec") ? (
          specChoices.map(({ recommendation, action, label }) => {
            const recommended = t.recommendation === recommendation;
            return (
              <Fragment key={recommendation}>
                <Button
                  type="button"
                  variant={!t.recommendation || recommended ? "primary" : "default"}
                  aria-describedby={recommended ? recommendedId : undefined}
                  disabled={busy}
                  onClick={() => run(() => api.act(project, t.id, action, {}))}
                >
                  {label}
                </Button>{" "}
                {recommended && (
                  <>
                    <span id={recommendedId} className="small muted">
                      Recommended
                    </span>{" "}
                  </>
                )}
              </Fragment>
            );
          })
        ) : (
          <Button type="button" variant="primary" disabled={busy} onClick={() => run(() => api.act(project, t.id, "accept", {}))}>
            Accept
          </Button>
        )}
      </p>
      <label className="reject-comment">
        <span className="small muted">What to change (required to reject)</span>
        <textarea rows={2} value={comment} onChange={(e) => setComment(e.target.value)} />
      </label>
      <p>
        <Button
          type="button"
          variant="danger"
          disabled={!canReject}
          onClick={() => run(() => api.act(project, t.id, "reject", { comment: comment.trim() }))}
        >
          Reject
        </Button>
      </p>
    </section>
  );
}

// finishSummary is the finish comment: the non-human comment made at the
// time of the latest move into In Review (finish stores both in one write).
function finishSummary(t: TicketView): CommentView | undefined {
  const entered = t.moves.filter((m) => m.to === "In Review").at(-1);
  if (!entered) return undefined;
  return t.comments.find((c) => c.time === entered.time && c.actor !== "human");
}

export const panel: PanelDef = { reasons: ["in_review"], Component: ReviewPanel };
