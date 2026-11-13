import type { NeedsMeReason } from "../api/types";
import type { ProjectTicket } from "../live";
import { isStage } from "../requirement/progress";
import { ListCIBadge } from "../ticket/CIBadge";
import { TicketLine, type InboxRowLinkFactory } from "../ticket/TicketLine";

// StateList is one State's Tickets, flat, with its Stage Tickets folded
// into a collapsed group after them: a Factory runs those. An In Progress
// row shows its latest Session Event under its title.
export function StateList({
  tickets,
  reasons,
  now,
  link,
}: {
  tickets: ProjectTicket[];
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  link: InboxRowLinkFactory;
}) {
  const stages = tickets.filter(isStage);
  const row = (t: ProjectTicket) => (
    <TicketLine
      key={t.id}
      t={t}
      link={link(t.id)}
      needsYou={reasons.has(t.id)}
      now={now}
    >
      <ListCIBadge t={t} />
    </TicketLine>
  );
  return (
    <>
      {tickets.filter((t) => !isStage(t)).map(row)}
      {stages.length > 0 && (
        <details className="quiet-group">
          <summary>Stage Tickets ({stages.length}): run by a Factory</summary>
          {stages.map(row)}
        </details>
      )}
    </>
  );
}
