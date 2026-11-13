// The In Review panel: the agent's finish summary, then Accept (on a spec
// Stage, Split into Tickets and Implement), and Reject, which can't be sent
// without a comment saying what to change.
import { useState } from "react";
import "../../api/actions/accept";
import "../../api/actions/implement";
import "../../api/actions/reject";
import type { CommentView, TicketView } from "../../api/types";
import { Markdown } from "../../Markdown";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { useAct } from "../../ticket/useAct";
import { Button } from "../../controls/Button";
import "./review.css";

export function ReviewPanel({ api, project, ticket: t }: PanelProps) {
  const [comment, setComment] = useState("");
  const { busy, error, run } = useAct();
  const summary = finishSummary(t);
  const canReject = comment.trim() !== "" && !busy;

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
          <>
            <Button type="button" variant="primary" disabled={busy} onClick={() => run(() => api.act(project, t.id, "accept", {}))}>
              Split into Tickets
            </Button>{" "}
            <Button type="button" variant="primary" disabled={busy} onClick={() => run(() => api.act(project, t.id, "implement", {}))}>
              Implement
            </Button>
          </>
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
