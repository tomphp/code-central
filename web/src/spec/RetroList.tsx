// The Retros looking back on a Ticket or a Spec, in its view: each one's ID
// and title to select it, and its State. Nothing is shown when there are none.
import type { RetroView } from "../api/types";
import { TicketName } from "../ticket/TicketLine";

export interface RetroListProps {
  retros?: RetroView[];
  select(ticket: string): void;
}

export function RetroList({ retros, select }: RetroListProps) {
  if (!retros?.length) return null;
  return (
    <ul className="impl-list" aria-label="Retros">
      {retros.map((r) => (
        <li key={r.id}>
          <button className="link" onClick={() => select(r.id)}>
            <TicketName t={r} />
          </button>{" "}
          <span className="state">{r.state}</span>
        </li>
      ))}
    </ul>
  );
}
