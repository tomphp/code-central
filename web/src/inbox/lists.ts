import { FINISHED, type State } from "../api/types";
import type { ProjectSpec, ProjectTicket } from "../live";

// stateList is one State's Tickets: in claim order, as found, except the
// finished States, which read newest first.
export function stateList(tickets: ProjectTicket[], state: State): ProjectTicket[] {
  const rows = tickets.filter((t) => t.state === state);
  return FINISHED.includes(state)
    ? rows.sort((a, b) => b.updated.localeCompare(a.updated))
    : rows;
}

// byCreation puts Specs from several lists back in creation order: by
// Project, then ID.
export function byCreation(specs: ProjectSpec[]): ProjectSpec[] {
  const n = (s: ProjectSpec) => Number(s.id.slice(s.id.lastIndexOf("-") + 1));
  return [...specs].sort(
    (a, b) => a.project.localeCompare(b.project) || n(a) - n(b),
  );
}

// requirementList is the Requirements view's order: unfinished ones, then
// finished ones, newest first in each.
export function requirementList(specs: ProjectSpec[]): ProjectSpec[] {
  const newest = [...specs].reverse();
  return [
    ...newest.filter((s) => !s.finished),
    ...newest.filter((s) => s.finished),
  ];
}
