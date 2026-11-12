// groupBySpec groups a list's Tickets (already in list order) by Spec: a
// Spec takes the place of its first Ticket, and its Tickets keep their
// order inside it.
import type { ProjectTicket } from "../live";

export interface SpecGroup {
  id: string;
  tickets: ProjectTicket[];
}

export function groupBySpec(tickets: ProjectTicket[]): SpecGroup[] {
  const groups = new Map<string, SpecGroup>();
  for (const t of tickets) {
    let g = groups.get(t.spec);
    if (!g) groups.set(t.spec, (g = { id: t.spec, tickets: [] }));
    g.tickets.push(t);
  }
  return [...groups.values()];
}
