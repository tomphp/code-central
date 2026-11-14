// new_fix_ticket: the human opens a Ready Ticket on a CI failed Ticket's
// Spec carrying the note, and the failed Ticket lands as with Land anyway
// (code-central-264). Its result is the fix Ticket's acknowledgement.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    new_fix_ticket: { body: { note: string }; result: TicketAcknowledgement };
  }
}
