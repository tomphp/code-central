// release: the human ends a Claim for good, moving an In Progress or
// Answered Ticket back to Ready. The released session's next write is
// rejected as stale. The optional note is stored as a comment.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    release: { body: { note?: string }; result: TicketAcknowledgement };
  }
}
