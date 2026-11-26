// resume: the human ends a Stopped Claim's mark, so the Factory takes its
// session up again. The optional note is stored as a comment.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    resume: { body: { note?: string }; result: TicketAcknowledgement };
  }
}
