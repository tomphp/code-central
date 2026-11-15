// The one-line header of a full-width view: its title, a left slot, a
// spacer, then a right slot. Focus Mode's has no title.
import type { ReactNode } from "react";

export interface ViewHeaderProps {
  title?: string;
  left?: ReactNode;
  right?: ReactNode;
}

export function ViewHeader({ title, left, right }: ViewHeaderProps) {
  return (
    <header className="view-head">
      {title && <h1>{title}</h1>}
      {left}
      <div className="view-head-spacer" />
      {right}
    </header>
  );
}
