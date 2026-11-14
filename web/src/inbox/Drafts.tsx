import type { ProjectTicket } from "../live";
import { TicketLine, type InboxRowLinkFactory } from "../ticket/TicketLine";

export function Drafts({
  drafts,
  link,
}: {
  drafts: ProjectTicket[];
  link: InboxRowLinkFactory;
}) {
  return (
    <details className="quiet-group">
      <summary>Drafts ({drafts.length}): not waiting on anyone</summary>
      {drafts.map((t) => (
        // A Draft's row is its ID and title alone.
        <TicketLine
          key={t.id}
          t={{ id: t.id, title: t.title, state: t.state }}
          link={link(t.id)}
          quiet
          dotless
        />
      ))}
    </details>
  );
}
