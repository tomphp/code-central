// accept: the human accepts an In Review Ticket: into Accepted, to land,
// when its finish named a branch, and into Done otherwise (ADR 0006).
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    accept: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}
