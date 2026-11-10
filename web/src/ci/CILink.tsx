// CILink opens the CI view on loc's Project, or All Projects when none is
// chosen, remembering loc so Back returns to it: from the Inbox's Admin
// section, and as the Activity Dashboard's CI Running block. className
// styles it as the row or block it sits in, and children are its content,
// CI by default.
import type { ReactNode } from "react";
import { clickTo, hrefFor, type UiLocation } from "../url";

export interface CILinkProps {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  className: string;
  children?: ReactNode;
}

export function CILink({ loc, navigate, className, children = "CI" }: CILinkProps) {
  const to: Partial<UiLocation> = { view: "ci", project: loc.project, back: loc };
  return (
    <a href={hrefFor({ ...loc, ...to })} onClick={clickTo(navigate, to)} className={className}>
      {children}
    </a>
  );
}
