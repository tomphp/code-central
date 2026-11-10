// The triage actions (Spec code-central-279): what the human may do with
// an Incoming Issue, offered on every one, awaiting triage or not. Keep as
// Draft is the first; To Requirement and Won't fix join it here.
import type { Api } from "../api/api";
import "../api/actions/keep_as_draft";
import type { TicketView } from "../api/types";
import { useAct } from "./useAct";

export function TriageActions({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const { busy, error, run } = useAct();
  const actions = t.triage_actions ?? [];
  return (
    <>
      <p className="actions">
        {actions.includes("keep_as_draft") && (
          <button type="button" className="btn" disabled={busy} onClick={() => run(() => api.act(project, t.id, "keep_as_draft", {}))}>
            Keep as Draft
          </button>
        )}
      </p>
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
        An Incoming Issue from GitHub Issues. Keep as Draft leaves it here as a Draft{props.ticket.awaiting_triage && ", out of Needs Me"}, and
        writes nothing to GitHub.
      </p>
      <TriageActions {...props} />
    </section>
  );
}
