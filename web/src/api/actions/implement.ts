// implement: the human accepts a Requirement's In Review spec Stage by
// building the whole Spec, skipping the tickets Stage. One action makes
// three writes, as the service does: the spec Stage goes to Done, one Ready
// implementation Ticket is created on the Spec, and the tickets Stage is
// Cancelled with that Ticket as its Replacement.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    implement: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}
