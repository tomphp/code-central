// The plain Spec pane, for a Spec that is neither a Requirement nor a Map,
// above the shared Ticket pane: the header (ID, title, submitted time, New
// Prototype), its body, collapsed, the sums of its Tickets' CI badges, then the Spec's Tickets
// with their States.
import type { Api } from "../api/api";
import type { NeedsMeReason, SpecDetail } from "../api/types";
import { BodySection } from "../edit/BodySection";
import { ago } from "../format";
import { NewPrototype } from "../requirement/NewRequirement";
import { CISums } from "../ticket/CIBadge";
import { LinkBadge } from "../ticket/LinkBadge";
import { OpenExternal } from "../ticket/OpenExternal";
import { LandPerSpec } from "./LandPerSpec";
import { TicketList } from "./TicketList";

export interface SpecPaneProps {
  api: Api;
  spec: SpecDetail;
  reasons: Map<string, NeedsMeReason>;
  // archived is whether the Spec's Project is Archived, which takes no edits.
  archived?: boolean;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
}

export function SpecPane({ api, spec, archived, reasons, now, ticket, select }: SpecPaneProps) {
  return (
    <section aria-label="Spec" className="req-head">
      <span className="tid">{spec.id}</span> <span className="muted small">Spec · submitted {ago(now, spec.created)}</span>
      <h2>{spec.title}</h2>
      <div className="spec-actions">
        <NewPrototype api={api} spec={spec.id} onSubmitted={(t) => select(t.id)} />
        {spec.link && <LinkBadge link={spec.link} />}
        <OpenExternal link={spec.link} />
      </div>
      <LandPerSpec status={spec.land_per_spec} />
      <BodySection api={api} spec={spec} heading="Body" archived={archived} />
      <CISums tickets={spec.tickets} />
      {spec.tickets.length === 0 ? (
        <p className="muted">No Tickets yet.</p>
      ) : (
        <TicketList label="Tickets" tickets={spec.tickets} reasons={reasons} now={now} ticket={ticket} select={select} />
      )}
    </section>
  );
}
