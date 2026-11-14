// reject: the human sends an In Review Ticket back to Ready with a
// required comment saying what to change.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    reject: { body: { comment: string }; result: TicketAcknowledgement };
  }
}
