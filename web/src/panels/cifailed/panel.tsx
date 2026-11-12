// The CI panel (code-central-261, code-central-432): an Accepted Ticket
// whose pushed commit's CI failed, was cancelled, or is unavailable. It
// names the commit's Needs Me item, which lists every Ticket waiting on
// the commit, across Specs, then is the Ticket's CI section, with each
// workflow's status, jobs and link, then the failure options, which act
// on every listed Ticket, so the Ticket pane and the Focus Item both
// offer them; then the Accepted panel, so Land with a commit and Cancel
// stay at hand while it waits.
import type { PanelDef, PanelProps } from "../ActionPanel";
import { AcceptedPanel } from "../accepted/panel";
import type { CIItem } from "../../api/types";
import { CISection } from "../../ticket/CISection";
import { hrefFor } from "../../url";

export function CIFailedPanel(props: PanelProps) {
  const { api, project, ticket } = props;
  const reason = ticket.needs_me_reason;
  const title = reason === "ci_cancelled" ? "CI cancelled" : reason === "ci_unavailable" ? "CI unavailable" : "CI failed";
  return (
    <>
      <section aria-label={title} className="panel">
        <h3>{title}</h3>
        <p className="small">
          {reason === "ci_unavailable"
            ? "No word of its pushed commit's CI has come for 10 minutes, so it waits Accepted until CI reports, or the CI Watcher is Watching again."
            : `Its pushed commit's CI ${reason === "ci_cancelled" ? "was cancelled" : "failed"}, so it waits Accepted until you choose what to do.`}
        </p>
        {ticket.ci_item && <CIItemTickets item={ticket.ci_item} self={ticket.id} title={title} />}
        <CISection api={api} project={project} ticket={ticket} />
      </section>
      <AcceptedPanel {...props} />
    </>
  );
}

// CIItemTickets names the commit's Needs Me item and links the other
// Tickets it lists: one item per commit, whose options act on all of them.
function CIItemTickets({ item, self, title }: { item: CIItem; self: string; title: string }) {
  const others = item.tickets.filter((t) => t.id !== self);
  return (
    <p className="small ci-item">
      {title} at <code>{item.commit.slice(0, 7)}</code> is one Needs Me item
      {others.length === 0 ? (
        <>, listing this Ticket alone.</>
      ) : (
        <>
          , listing {item.tickets.length} Tickets waiting on the commit. Your choice acts on all of them. Also waiting:{" "}
          {others.map((t, i) => (
            <span key={t.id}>
              {i > 0 && ", "}
              <a href={hrefFor({ view: "inbox", ticket: t.id })} title={t.title}>
                {t.id}
              </a>
            </span>
          ))}
          .
        </>
      )}
    </p>
  );
}

export const panel: PanelDef = { reasons: ["ci_failed", "ci_cancelled", "ci_unavailable"], Component: CIFailedPanel };
