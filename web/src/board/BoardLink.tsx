// BoardLink opens the Board view from the Inbox's View section, on loc's
// Project or All Projects when none is chosen, with loc's grouping,
// remembering loc so Back returns to it. Its count is the Tickets in scope
// in the columns the Board opens with, and the needing Tickets in hidden States while its Needs Me column is on; className styles it as the row it
// sits in.
import type { NeedsMeReason } from "../api/types";
import type { ProjectTicket } from "../live";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { hiddenColumns, needsMeColumn } from "../inbox/groupPreference";

export interface BoardLinkProps {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  // tickets is the Tickets in scope, in every State.
  tickets: ProjectTicket[];
  // reasons maps each Ticket that needs the human to its reason.
  reasons: Map<string, NeedsMeReason>;
  className: string;
}

export function BoardLink({ loc, navigate, tickets, reasons, className }: BoardLinkProps) {
  const to: Partial<UiLocation> = { view: "board", project: loc.project, group: loc.group, hide: undefined, needs: undefined, back: loc };
  const hidden = hiddenColumns();
  const needsMe = needsMeColumn();
  return (
    <a href={hrefFor({ ...loc, ...to })} onClick={clickTo(navigate, to)} className={className}>
      Board <span className="count">{tickets.filter((t) => !hidden.includes(t.state) || (needsMe && reasons.has(t.id))).length}</span>
    </a>
  );
}
