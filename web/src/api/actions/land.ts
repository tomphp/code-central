// land: the work is in the base, so an Accepted (or In Review) Ticket
// moves to Done, recording the commit that put it there on the Move with
// via merge (ADR 0006). With several Repositories it names the
// Repository the work landed in (the Home Repository when left out), and
// the Ticket is Done once it has landed in each of its Changed
// Repositories.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    land: { body: { commit: string; repository?: string }; result: TicketAcknowledgement };
  }
}
