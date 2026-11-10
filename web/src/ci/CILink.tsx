// CILink opens the CI view from the Inbox's Admin section, on the
// sidebar's Project, or All Projects when none is chosen, remembering
// that view so Back returns to it; className styles it as the row it
// sits in.
import { clickTo, hrefFor, type UiLocation } from "../url";

export interface CILinkProps {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  className: string;
}

export function CILink({ loc, navigate, className }: CILinkProps) {
  const to: Partial<UiLocation> = { view: "ci", project: loc.project, back: loc };
  return (
    <a href={hrefFor({ ...loc, ...to })} onClick={clickTo(navigate, to)} className={className}>
      CI
    </a>
  );
}
