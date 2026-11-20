// The Requirement pane, above the shared Ticket pane: the header (ID,
// title, submitted time, New Prototype, Retro), its Changed Repositories'
// chips while the Project has several, the clickable Stage strip (its
// Map segment linking to the Map it was made from, if any), what it waits
// for, its body (the text, then the Spec) collapsed, its Tags, and under Build the sums of its implementation Tickets' CI
// badges, their Ticket graph when one blocks another, and those Tickets
// with their States.
import type { SpecDetail } from "../api/types";
import { RepositoryChips } from "../controls/RepositoryChips";
import { useApi } from "../live";
import { BodySection } from "../edit/BodySection";
import { hasEdge } from "../graph/edges";
import { TicketGraph } from "../graph/TicketGraph";
import { LandPerSpec } from "../spec/LandPerSpec";
import { SpecAutoAccept } from "../spec/SpecAutoAccept";
import { SpecTags } from "../spec/SpecTags";
import { RetroList } from "../spec/RetroList";
import { TicketList } from "../spec/TicketList";
import { CISums } from "../ticket/CIBadge";
import { LinkBadge } from "../ticket/LinkBadge";
import { OpenExternal } from "../ticket/OpenExternal";
import type { Link } from "../url";
import { NewPrototype, NewRetro, NewTicket } from "./NewRequirement";
import { isStage, waitingNote } from "./progress";
import { Strip } from "./Strip";
import { SpecPaneHeader } from "../controls/SpecPaneHeader";

export interface RequirementPaneProps {
  spec: SpecDetail & { requirement: NonNullable<SpecDetail["requirement"]> };
  // archived is whether the Requirement's Project is Archived, whose
  // Auto-accept switch is shown off. The Spec's editable and retroable
  // already count it.
  archived?: boolean;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
  // specLink links another Spec, such as the Map it was made from.
  specLink(spec: string): Link;
}

export function RequirementPane({ spec, archived, now, ticket, select, specLink }: RequirementPaneProps) {
  const api = useApi();
  const build = (spec.tickets ?? []).filter((t) => !isStage(t));
  const note = waitingNote(spec.requirement);
  const specDone = spec.requirement.stages.some((s) => s.name === "spec" && s.state === "Done");
  return (
    <section aria-label="Requirement" className="req-head">
      <SpecPaneHeader id={spec.id} kind="Requirement" title={spec.title} now={now} created={spec.created} />
      <RepositoryChips names={spec.repositories} />
      <div className="spec-actions">
        <NewPrototype spec={spec.id} onSubmitted={(t) => select(t.id)} />
        {spec.ticketable && <NewTicket spec={spec.id} onSubmitted={(t) => select(t.id)} />}
        {spec.retroable && <NewRetro target={spec.id} onSubmitted={(t) => select(t.id)} />}
        {spec.link && <LinkBadge link={spec.link} />}
        <OpenExternal link={spec.link} />
      </div>
      <Strip
        requirement={spec.requirement}
        build={build}
        current={ticket}
        mapLink={specLink}
        onPick={(s) => {
          // Build opens on its first Ticket that needs the human, else its first.
          const t = s.ticket ?? spec.requirement.needs.find((n) => !n.stage)?.ticket ?? build[0]?.id;
          if (t) select(t);
        }}
      />
      {note && <p className="panel muted waiting">{note}</p>}
      <SpecAutoAccept spec={spec} archived={archived} />
      <LandPerSpec status={spec.land_per_spec} />
      {/* The text is the Requirement's until its spec Stage has written the Spec. */}
      <BodySection api={api} spec={spec} heading={specDone ? "Spec" : "Requirement's text"} />
      <SpecTags spec={spec} archived={archived} />
      <CISums tickets={build} />
      {hasEdge(build) && <TicketGraph tickets={build} label="Build graph" ticket={ticket} select={select} />}
      {build.length > 0 && <TicketList label="Build" tickets={build} now={now} ticket={ticket} select={select} />}
      <RetroList retros={spec.retros} select={select} />
    </section>
  );
}
