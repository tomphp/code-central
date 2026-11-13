// BoardLink opens the Board view from the Inbox's View section, on loc's
// Project or All Projects when none is chosen, with loc's grouping,
// remembering loc so Back returns to it. Its count is the Tickets in scope
// in the columns the Board opens with; className styles it as the row it
// sits in.
import type { ProjectTicket } from "../live";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { hiddenColumns } from "../inbox/groupPreference";

export interface BoardLinkProps {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  // tickets is the Tickets in scope, in every State.
  tickets: ProjectTicket[];
  className: string;
}

export function BoardLink({ loc, navigate, tickets, className }: BoardLinkProps) {
  const to: Partial<UiLocation> = { view: "board", project: loc.project, group: loc.group, hide: undefined, back: loc };
  const hidden = hiddenColumns();
  return (
    <a href={hrefFor({ ...loc, ...to })} onClick={clickTo(navigate, to)} className={className}>
      Board <span className="count">{tickets.filter((t) => !hidden.includes(t.state)).length}</span>
    </a>
  );
}
