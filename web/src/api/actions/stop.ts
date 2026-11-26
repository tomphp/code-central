// stop: the human stops a Factory Claim's session, keeping the Claim and
// the Ticket's State. The optional note is stored as a comment.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    stop: { body: { note?: string }; result: TicketAcknowledgement };
  }
}
