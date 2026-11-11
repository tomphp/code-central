// The Requirement pane, above the shared Ticket pane: the header (ID,
// title, submitted time, New Prototype), the clickable Stage strip (its
// Map segment linking to the Map it was made from, if any), what it waits
// for, its body (the text, then the Spec) collapsed, and under Build the sums of its implementation Tickets' CI badges
// and those Tickets with their States.
import type { Api } from "../api/api";
import type { NeedsMeReason, SpecDetail } from "../api/types";
import { BodySection } from "../edit/BodySection";
import { ago } from "../format";
import { TicketList } from "../spec/TicketList";
import { CISums } from "../ticket/CIBadge";
import { LinkBadge } from "../ticket/LinkBadge";
import { OpenExternal } from "../ticket/OpenExternal";
import type { Link } from "../url";
import { NewPrototype } from "./NewRequirement";
import { progress, waitingNote } from "./progress";
import { Strip } from "./Strip";

export interface RequirementPaneProps {
  api: Api;
  spec: SpecDetail & { requirement: NonNullable<SpecDetail["requirement"]> };
  reasons: Map<string, NeedsMeReason>;
  // archived is whether the Requirement's Project is Archived, which takes no edits.
  archived?: boolean;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
  // specLink links another Spec, such as the Map it was made from.
  specLink(spec: string): Link;
}

export function RequirementPane({ api, spec, archived, reasons, now, ticket, select, specLink }: RequirementPaneProps) {
  const p = progress(spec.requirement, spec.tickets, reasons);
  const note = waitingNote(p);
  const specDone = spec.requirement.stages.some((s) => s.name === "spec" && s.state === "Done");
  return (
    <section aria-label="Requirement" className="req-head">
      <span className="tid">{spec.id}</span> <span className="muted small">Requirement · submitted {ago(now, spec.created)}</span>
      <h2>{spec.title}</h2>
      <div className="spec-actions">
        <NewPrototype api={api} spec={spec.id} onSubmitted={(t) => select(t.id)} />
        {spec.link && <LinkBadge link={spec.link} />}
        <OpenExternal link={spec.link} />
      </div>
      <Strip
        p={p}
        current={ticket}
        mapLink={specLink}
        onPick={(s) => {
          const t = s.ticket ?? (p.build.find((t) => reasons.has(t.id)) ?? p.build[0])?.id;
          if (t) select(t);
        }}
      />
      {note && <p className="panel muted waiting">{note}</p>}
      {/* The text is the Requirement's until its spec Stage has written the Spec. */}
      <BodySection api={api} spec={spec} heading={specDone ? "Spec" : "Requirement's text"} archived={archived} />
      <CISums tickets={p.build} />
      {p.build.length > 0 && <TicketList label="Build" tickets={p.build} reasons={reasons} now={now} ticket={ticket} select={select} />}
    </section>
  );
}
