// land: the work is in the base, so an Accepted (or In Review) Ticket
// moves to Done, recording the commit that put it there on the Move with
// via merge (ADR 0006).
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    land: { body: { commit: string }; result: TicketAcknowledgement };
  }
}
