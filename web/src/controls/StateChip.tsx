import type { ReactNode } from "react";

// stateSlug is a State, or a Spec's kind, as a class name: "In Progress"
// is in-progress.
export const stateSlug = (s: string) => s.toLowerCase().replace(/\s+/g, "-");

// StateChip is a State as a coloured chip. Children replace the State's
// name, for a chip that also carries a count; here marks the State a row
// is listed under.
export function StateChip({ state, here, children }: { state: string; here?: boolean; children?: ReactNode }) {
  return (
    <span className={`state-chip st-${stateSlug(state)}${here ? " here" : ""}`} aria-current={here ? "true" : undefined}>
      {children ?? state}
    </span>
  );
}
