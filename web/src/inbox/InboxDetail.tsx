import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import type { NeedsMeReason, ProjectRow } from "../api/types";
import { MapPane } from "../map/MapPane";
import { RequirementPane } from "../requirement/RequirementPane";
import { SpecPane } from "../spec/SpecPane";
import { TicketPane } from "../ticket/TicketDetail";
import type { Link } from "../url";
import type { InboxProps } from "./Inbox";

// InboxDetail is the Inbox's right-hand pane: the selected Requirement, Map
// or Spec above the selected Ticket.
export function InboxDetail({
  api,
  now,
  loc,
  navigate,
  requirement,
  map,
  spec,
  requirementError,
  ticket,
  ticketError,
  reasons,
  projects,
  isArchived,
  specLink,
}: Pick<
  InboxProps,
  | "api"
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
  reasons: Map<string, NeedsMeReason>;
  projects: ProjectRow[];
  isArchived(id: string): boolean;
  specLink(id: string): Link;
}) {
  return (
    <aside aria-label="Ticket" className="inbox-detail">
      {loc.requirement &&
        (requirementError ? (
          <p className="empty">{requirementError}</p>
        ) : requirement ? (
          <RequirementPane
            api={api}
            spec={requirement}
            archived={isArchived(requirement.id)}
            reasons={reasons}
            now={now}
            ticket={loc.ticket}
            select={(id) => navigate({ ticket: id })}
            specLink={specLink}
          />
        ) : map ? (
          <MapPane
            api={api}
            spec={map}
            reasons={reasons}
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
            api={api}
            spec={spec}
            archived={isArchived(spec.id)}
            reasons={reasons}
            now={now}
            ticket={loc.ticket}
            select={(id) => navigate({ ticket: id })}
          />
        ) : (
          <p className="empty">Loading…</p>
        ))}
      {ticketError ? (
        <p className="empty">{ticketError}</p>
      ) : ticket && loc.ticket && ticket.id === loc.ticket ? (
        <TicketPane
          api={api}
          project={projectOf(ticket.id)}
          ticket={ticket}
          now={now}
          archived={isArchived(ticket.id)}
          reported={
            projects.find((p) => p.key === projectOf(ticket.id))
              ?.reported_models
          }
          openSpec={(id) => navigate({ requirement: id })}
        />
      ) : (
        !loc.requirement && (
          <p className="empty">
            {loc.ticket ? "Loading…" : "Select a Ticket."}
          </p>
        )
      )}
    </aside>
  );
}
