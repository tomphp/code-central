// land_anyway: the human accepts an Accepted Ticket's failed CI result,
// so the Factory lands it on its next poll (code-central-261). Offered
// only under Merge: under Branch the human's merge decides. With several
// Repositories, repository names the CI item's, the only one it lands in.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    land_anyway: { body: { repository?: string }; result: TicketAcknowledgement };
  }
}
