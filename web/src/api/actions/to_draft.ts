// to_draft: the human moves a Failed Ticket back to Draft.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    to_draft: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}
