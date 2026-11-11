// The CI failed panel (code-central-261): an Accepted Ticket whose CI
// result is failed, or cancelled. It is the Ticket's CI section, with each workflow's
// status, jobs and link, then the failure options, so the Ticket pane and
// the Focus Item both offer them; then the Accepted panel, so Land with a
// commit and Cancel stay at hand while it waits.
import type { PanelDef, PanelProps } from "../ActionPanel";
import { AcceptedPanel } from "../accepted/panel";
import { CISection } from "../../ticket/CISection";

export function CIFailedPanel(props: PanelProps) {
  const { api, project, ticket } = props;
  const cancelled = ticket.needs_me_reason === "ci_cancelled";
  const title = cancelled ? "CI cancelled" : "CI failed";
  return (
    <>
      <section aria-label={title} className="panel">
        <h3>{title}</h3>
        <p className="small">Its pushed commit's CI {cancelled ? "was cancelled" : "failed"}, so it waits Accepted until you choose what to do.</p>
        {ticket.ci_group && (
          <p className="small ci-group">
            {ticket.ci_group.spec} lands together: {ticket.ci_group.waiting} {ticket.ci_group.waiting === 1 ? "Ticket waits" : "Tickets wait"} on this
            pushed commit. Your choice acts on all of them. Also waiting:{" "}
            {ticket.ci_group.tickets.filter((id) => id !== ticket.id).join(", ") || "none"}.
          </p>
        )}
        <CISection api={api} project={project} ticket={ticket} />
      </section>
      <AcceptedPanel {...props} />
    </>
  );
}

export const panel: PanelDef = { reasons: ["ci_failed", "ci_cancelled"], Component: CIFailedPanel };
