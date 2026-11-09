// The plain Spec pane, for a Spec that is neither a Requirement nor a Map,
// above the shared Ticket pane: the header (ID, title, submitted time, New
// Prototype), then the Spec's Tickets with their States.
import type { Api } from "../api/api";
import type { NeedsMeReason, SpecDetail } from "../api/types";
import { ago } from "../format";
import { NewPrototype } from "../requirement/NewRequirement";
import { TicketList } from "./TicketList";

export interface SpecPaneProps {
  api: Api;
  spec: SpecDetail;
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
}

export function SpecPane({ api, spec, reasons, now, ticket, select }: SpecPaneProps) {
  return (
    <section aria-label="Spec" className="req-head">
      <span className="tid">{spec.id}</span> <span className="muted small">Spec · submitted {ago(now, spec.created)}</span>
      <h2>{spec.title}</h2>
      <div className="spec-actions">
        <NewPrototype api={api} spec={spec.id} onSubmitted={(t) => select(t.id)} />
      </div>
      {spec.tickets.length === 0 ? (
        <p className="muted">No Tickets yet.</p>
      ) : (
        <TicketList label="Tickets" tickets={spec.tickets} reasons={reasons} now={now} ticket={ticket} select={select} />
      )}
    </section>
  );
}
