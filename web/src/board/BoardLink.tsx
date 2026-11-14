// BoardLink opens the Board view from the Inbox's View section, on loc's
// Project or All Projects when none is chosen, with loc's grouping,
// remembering loc so Back returns to it. Its count is the Tickets in scope
// in the columns the Board opens with, and the needing Tickets in hidden States while its Needs Me column is on; className styles it as the row it
// sits in.
import { useNeedsMe } from "../needsme/context";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { hiddenColumns, needsMeColumn } from "../inbox/groupPreference";

export interface BoardLinkProps {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  className: string;
}

export function BoardLink({ loc, navigate, className }: BoardLinkProps) {
  const { tickets, reasonOf } = useNeedsMe();
  const to: Partial<UiLocation> = { view: "board", project: loc.project, group: loc.group, hide: undefined, needs: undefined, back: loc };
  const hidden = hiddenColumns();
  const needsMe = needsMeColumn();
  return (
    <a href={hrefFor({ ...loc, ...to })} onClick={clickTo(navigate, to)} className={className}>
      Board <span className="count">{tickets(loc.project).filter((t) => !hidden.includes(t.state) || (needsMe && reasonOf(t.id))).length}</span>
    </a>
  );
}
