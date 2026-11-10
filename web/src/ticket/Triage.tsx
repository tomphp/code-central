// The triage actions (Spec code-central-279): what the human may do with
// an Incoming Issue, offered on every one, awaiting triage or not: Keep
// as Draft, To Requirement with its context box, and Won't fix with its
// reason box.
import { useState } from "react";
import type { Api } from "../api/api";
import "../api/actions/keep_as_draft";
import "../api/actions/to_requirement";
import "../api/actions/wont_fix";
import type { TicketView } from "../api/types";
import { useAct } from "./useAct";

export function TriageActions({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const { busy, error, run } = useAct();
  // open is the triage action whose box is open, if any.
  const [open, setOpen] = useState<"to_requirement" | "wont_fix">();
  const [context, setContext] = useState("");
  const [reason, setReason] = useState("");
  const actions = t.triage_actions ?? [];
  const back = (
    <button type="button" className="btn" disabled={busy} onClick={() => setOpen(undefined)}>
      Back
    </button>
  );
  return (
    <>
      <p className="actions">
        {actions.includes("keep_as_draft") && (
          <button type="button" className="btn" disabled={busy} onClick={() => run(() => api.act(project, t.id, "keep_as_draft", {}))}>
            Keep as Draft
          </button>
        )}
        {actions.includes("to_requirement") && !open && (
          <button type="button" className="btn" disabled={busy} onClick={() => setOpen("to_requirement")}>
            To Requirement…
          </button>
        )}
        {actions.includes("wont_fix") && !open && (
          <button type="button" className="btn" disabled={busy} onClick={() => setOpen("wont_fix")}>
            Won't fix…
          </button>
        )}
      </p>
      {open === "to_requirement" && (
        <form
          aria-label="To Requirement"
          onSubmit={(e) => {
            e.preventDefault();
            const c = context.trim();
            run(() => api.act(project, t.id, "to_requirement", c ? { context: c } : {}));
          }}
        >
          <label>
            <span className="small muted">Context (optional), added to the Requirement and posted to the issue as a comment</span>
            <textarea rows={3} value={context} onChange={(e) => setContext(e.target.value)} />
          </label>
          <p className="small muted">
            To Requirement makes a Requirement of the issue's title and body, Cancels this Ticket, and from then on only writes the work's progress to
            the issue.
          </p>
          <p className="actions">
            <button type="submit" className="btn primary" disabled={busy}>
              To Requirement
            </button>
            {back}
          </p>
        </form>
      )}
      {open === "wont_fix" && (
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
            {back}
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
        writes nothing to GitHub. To Requirement makes a Requirement of it. Won't fix Cancels it and closes the issue as not planned.
      </p>
      <TriageActions {...props} />
    </section>
  );
}
