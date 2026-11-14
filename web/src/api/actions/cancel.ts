// cancel: the human cancels any unfinished Ticket, naming Replacements or
// none; the optional reason is stored as a comment. (Ticket 36's Failed
// panel and ticket 37's Blocked panel both use it.)
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    cancel: { body: { replacements?: string[]; reason?: string }; result: TicketAcknowledgement };
  }
}
