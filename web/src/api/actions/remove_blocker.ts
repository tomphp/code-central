// remove_blocker: the Ticket stops waiting on a stored Blocker. A Blocker
// shown only as a Cancelled Ticket's Replacement (its view has `via`) is
// removed by naming that Cancelled Ticket.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    remove_blocker: { body: { blocker: string }; result: TicketAcknowledgement };
  }
}
