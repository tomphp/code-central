// A State list grouped By Spec: one Spec row per Spec with Tickets in the
// list, opening on those Tickets.
import type { State } from "../api/types";
import type { ProjectSpec, ProjectTicket } from "../live";
import { groupBySpec } from "./groupBySpec";
import { SpecRow, type SpecRowProps } from "./SpecRow";
import type { InboxRowLinkFactory } from "../ticket/TicketLine";

export function StateListBySpec({
  state,
  tickets,
  specs,
  ticketsOf,
  now,
  link,
  specLink,
  selectedTicket,
}: {
  state: State;
  tickets: ProjectTicket[];
  specs: Map<string, ProjectSpec>;
  // ticketsOf is all of a Spec's Tickets, in every State.
  ticketsOf(spec: string): ProjectTicket[];
  now: Date;
  link: InboxRowLinkFactory;
  specLink(id: string): SpecRowProps["link"];
  selectedTicket?: string;
}) {
  const groups = groupBySpec(tickets);
  if (groups.length === 0) return <p className="empty">No Tickets {state}.</p>;
  return (
    <div className="spec-rows">
      {groups.map((g) => (
        <SpecRow
          key={g.id}
          id={g.id}
          spec={specs.get(g.id)}
          all={ticketsOf(g.id)}
          shown={g.tickets}
          here={state}
          now={now}
          link={specLink(g.id)}
          ticketLink={link}
          selectedTicket={selectedTicket}
        />
      ))}
    </div>
  );
}
