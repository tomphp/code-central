// A Ticket graph's edges: each runs from a Blocker to the Ticket it
// blocks, both in the set; a Blocker outside the set draws no edge.
import type { TicketRow } from "../api/types";

type Node = Pick<TicketRow, "id" | "blocked_by">;

export interface Edge {
  from: string;
  to: string;
}

// edgesOf is the set's edges, in the Tickets' order and then each one's
// Blockers' order.
export function edgesOf(tickets: Node[]): Edge[] {
  const ids = new Set(tickets.map((t) => t.id));
  return tickets.flatMap((t) => (t.blocked_by ?? []).filter((b) => ids.has(b)).map((b) => ({ from: b, to: t.id })));
}

// hasEdge reports whether some Ticket in the set is blocked by another in
// the set: whether a Spec view or a Requirement's view draws its graph.
export const hasEdge = (tickets: Node[]) => edgesOf(tickets).length > 0;
