// UsageLink opens Usage from the Inbox's Admin section, on All Projects
// and 7 days, remembering that view so Back returns to it; className
// styles it as the row it sits in.
import { clickTo, hrefFor, type UiLocation } from "../url";

export interface UsageLinkProps {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  className: string;
}

export function UsageLink({ loc, navigate, className }: UsageLinkProps) {
  const to: Partial<UiLocation> = { view: "usage", project: undefined, days: undefined, back: loc };
  return (
    <a href={hrefFor({ ...loc, ...to })} onClick={clickTo(navigate, to)} className={className}>
      Usage
    </a>
  );
}
