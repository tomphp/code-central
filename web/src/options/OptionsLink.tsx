// OptionsLink opens the Options view from a view's header, remembering
// that view so leaving Options returns to it.
import type { MouseEvent } from "react";
import { hrefFor, opensInPlace, type UiLocation } from "../url";

export interface OptionsLinkProps {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  className?: string;
}

export function OptionsLink({ loc, navigate, className }: OptionsLinkProps) {
  const to = { view: "options" as const, from: loc.view === "focus" ? ("focus" as const) : ("inbox" as const) };
  const open = (e: MouseEvent) => {
    if (!opensInPlace(e)) return;
    e.preventDefault();
    navigate(to);
  };
  return (
    <a href={hrefFor({ ...loc, ...to })} onClick={open} className={className ?? "options-link"}>
      Options
    </a>
  );
}
