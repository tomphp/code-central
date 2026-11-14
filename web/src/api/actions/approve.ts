// approve: the human moves a Draft Ticket to Ready, as Start on a Map's
// frontier does.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    approve: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}
