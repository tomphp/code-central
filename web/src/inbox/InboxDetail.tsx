import { projectOf } from "../api/api";
import type { ProjectRow } from "../api/types";
import { MapPane } from "../map/MapPane";
import { RequirementPane } from "../requirement/RequirementPane";
import { SpecPane } from "../spec/SpecPane";
import { SpecPaneNote } from "../spec/SpecPaneNote";
import { TicketPane } from "../ticket/TicketDetail";
import type { Link } from "../url";
import type { InboxProps } from "./Inbox";

// InboxDetail is the Inbox's right-hand pane: the selected Requirement, Map
// or Spec above the selected Ticket.
export function InboxDetail({
  now,
  loc,
  navigate,
  requirement,
  map,
  spec,
  requirementError,
  ticket,
  ticketError,
  projects,
  isArchived,
  specLink,
}: Pick<
  InboxProps,
  | "now"
  | "loc"
  | "navigate"
  | "requirement"
  | "map"
  | "spec"
  | "requirementError"
  | "ticket"
  | "ticketError"
> & {
  projects: ProjectRow[];
  isArchived(id: string): boolean;
  specLink(id: string): Link;
}) {
  // shown is the selected Ticket once read: one still being read is
  // another's.
  const shown = ticket && ticket.id === loc.ticket ? ticket : undefined;
  return (
    <aside aria-label="Ticket" className="inbox-detail">
      {loc.requirement &&
        (requirementError ? (
          <SpecPaneNote error={requirementError} />
        ) : requirement ? (
          <RequirementPane
            spec={requirement}
            archived={isArchived(requirement.id)}
            now={now}
            ticket={loc.ticket}
            select={(id) => navigate({ ticket: id })}
            specLink={specLink}
          />
        ) : map ? (
          <MapPane
            spec={map}
            now={now}
            ticket={loc.ticket}
            select={(id) => navigate({ ticket: id })}
            specLink={specLink}
            written={(id) =>
              navigate({
                list: "requirements",
                state: undefined,
                requirement: id,
                ticket: undefined,
              })
            }
          />
        ) : spec ? (
          <SpecPane
            spec={spec}
            archived={isArchived(spec.id)}
            now={now}
            ticket={loc.ticket}
            select={(id) => navigate({ ticket: id })}
          />
        ) : (
          <SpecPaneNote />
        ))}
      {/* Under a Requirement, Map or Spec, a Ticket still loading shows
          nothing rather than Loading…. */}
      {ticketError || (loc.ticket && (!loc.requirement || shown)) ? (
        <TicketPane
          project={projectOf(loc.ticket ?? "")}
          ticket={ticketError ? undefined : shown}
          error={ticketError}
          back={() => navigate({ ticket: undefined })}
          now={now}
          archived={!!loc.ticket && isArchived(loc.ticket)}
          reported={
            projects.find((p) => p.key === projectOf(loc.ticket ?? ""))
              ?.reported_models
          }
          openSpec={(id) => navigate({ requirement: id })}
          openTicket={(id) => navigate({ ticket: id })}
        />
      ) : (
        !loc.requirement && <p className="empty">Select a Ticket.</p>
      )}
    </aside>
  );
}
