// The plain Spec pane, for a Spec that is neither a Requirement nor a Map,
// above the shared Ticket pane: the header (ID, title, submitted time, New
// Prototype, Retro), its Changed Repositories' chips while the Project has
// several, its body, collapsed, the sums of its Tickets' CI badges, then the Spec's Tickets
// with their States.
import type { SpecDetail } from "../api/types";
import { RepositoryChips } from "../controls/RepositoryChips";
import { useApi } from "../live";
import { BodySection } from "../edit/BodySection";
import { SpecPaneHeader } from "../controls/SpecPaneHeader";
import { NewPrototype, NewRetro } from "../requirement/NewRequirement";
import { CISums } from "../ticket/CIBadge";
import { LinkBadge } from "../ticket/LinkBadge";
import { OpenExternal } from "../ticket/OpenExternal";
import { LandPerSpec } from "./LandPerSpec";
import { SpecAutoAccept } from "./SpecAutoAccept";
import { RetroList } from "./RetroList";
import { TicketList } from "./TicketList";

export interface SpecPaneProps {
  spec: SpecDetail;
  // archived is whether the Spec's Project is Archived, which its header
  // and Auto-accept switch show. The Spec's editable and retroable already
  // count it.
  archived?: boolean;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
}

export function SpecPane({ spec, archived, now, ticket, select }: SpecPaneProps) {
  const api = useApi();
  const tickets = spec.tickets ?? [];
  return (
    <section aria-label="Spec" className="req-head">
      <SpecPaneHeader id={spec.id} kind="Spec" title={spec.title} now={now} created={spec.created} archived={archived} />
      <RepositoryChips names={spec.repositories} />
      <div className="spec-actions">
        <NewPrototype spec={spec.id} onSubmitted={(t) => select(t.id)} />
        {spec.retroable && <NewRetro target={spec.id} onSubmitted={(t) => select(t.id)} />}
        {spec.link && <LinkBadge link={spec.link} />}
        <OpenExternal link={spec.link} />
        <div className="spec-actions-end">
          <SpecAutoAccept spec={spec} archived={archived} />
        </div>
      </div>
      <LandPerSpec status={spec.land_per_spec} />
      <BodySection api={api} spec={spec} heading="Body" />
      <CISums tickets={tickets} />
      {tickets.length === 0 ? (
        <p className="muted">No Tickets yet.</p>
      ) : (
        <TicketList label="Tickets" tickets={tickets} now={now} ticket={ticket} select={select} />
      )}
      <RetroList retros={spec.retros} select={select} />
    </section>
  );
}
