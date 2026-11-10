// The triage actions (Spec code-central-279): what the human may do with
// an Incoming Issue, offered on every one, awaiting triage or not: Keep
// as Draft, and Won't fix with its reason box. To Requirement joins them
// here.
import { useState } from "react";
import type { Api } from "../api/api";
import "../api/actions/keep_as_draft";
import "../api/actions/wont_fix";
import type { TicketView } from "../api/types";
import { useAct } from "./useAct";

export function TriageActions({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const { busy, error, run } = useAct();
  const [wontFix, setWontFix] = useState(false);
  const [reason, setReason] = useState("");
  const actions = t.triage_actions ?? [];
  return (
    <>
      <p className="actions">
        {actions.includes("keep_as_draft") && (
          <button type="button" className="btn" disabled={busy} onClick={() => run(() => api.act(project, t.id, "keep_as_draft", {}))}>
            Keep as Draft
          </button>
        )}
        {actions.includes("wont_fix") && !wontFix && (
          <button type="button" className="btn" disabled={busy} onClick={() => setWontFix(true)}>
            Won't fix…
          </button>
        )}
      </p>
      {wontFix && (
        <form
          aria-label="Won't fix"
          onSubmit={(e) => {
            e.preventDefault();
            const r = reason.trim();
            run(() => api.act(project, t.id, "wont_fix", r ? { reason: r } : {}));
          }}
        >
          <label>
            <span className="small muted">Reason (optional), posted to the issue as a comment</span>
            <textarea rows={2} value={reason} onChange={(e) => setReason(e.target.value)} />
          </label>
          <p className="small muted">Won't fix Cancels this Ticket and closes the issue on GitHub as not planned.</p>
          <p className="actions">
            <button type="submit" className="btn primary" disabled={busy}>
              Won't fix
            </button>
            <button type="button" className="btn" disabled={busy} onClick={() => setWontFix(false)}>
              Back
            </button>
          </p>
        </form>
      )}
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </>
  );
}

// Triage is an Incoming Issue's triage actions in its Ticket details,
// where its action panel doesn't already offer them.
export function Triage(props: { api: Api; project: string; ticket: TicketView }) {
  return (
    <section aria-label="Triage">
      <p className="muted small">
        An Incoming Issue from GitHub Issues. Keep as Draft leaves it here as it is{props.ticket.awaiting_triage && ", out of Needs Me"}, and
        writes nothing to GitHub. Won't fix Cancels it and closes the issue as not planned.
      </p>
      <TriageActions {...props} />
    </section>
  );
}
