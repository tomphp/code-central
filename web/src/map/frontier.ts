import type { TicketRow } from "../api/types";

// isFrontier reports whether a Map Ticket is on the frontier: Draft and
// not blocked, so the human can Start it.
export const isFrontier = (t: TicketRow) => t.state === "Draft" && !t.blocked;
