// ActivityLink opens the Activity Dashboard in its own view from the Inbox's
// Admin section, on loc's Project or All Projects when none is chosen,
// remembering loc so Back returns to it; className styles it as the row it
// sits in.
import { clickTo, hrefFor, type UiLocation } from "../url";

export interface ActivityLinkProps {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  className: string;
}

export function ActivityLink({ loc, navigate, className }: ActivityLinkProps) {
  const to: Partial<UiLocation> = { view: "activity", project: loc.project, back: loc };
  return (
    <a href={hrefFor({ ...loc, ...to })} onClick={clickTo(navigate, to)} className={className}>
      Activity
    </a>
  );
}
