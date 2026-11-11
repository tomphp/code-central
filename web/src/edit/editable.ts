// Where the UI offers Edit. The service takes an edit to any Spec or
// Ticket in any State; this decides only where the human is offered one.
import type { LinkView, SpecDetail, TicketView } from "../api/types";

// owned is whether the Link's Integration owns the definition, so the
// service refuses the human's edit: attached, and not outbound.
const owned = (link?: LinkView) => !!link && !link.detached && !link.outbound;

// A Ticket only while it is Draft.
export function ticketEditable(t: TicketView, archived: boolean): boolean {
  return t.state === "Draft" && !owned(t.link) && !archived;
}

// A Spec until it is Finished, never a Map.
export function specEditable(s: SpecDetail, archived: boolean): boolean {
  return !s.finished && !s.map && !owned(s.link) && !archived;
}
