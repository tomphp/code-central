// send_back: the human sends a Failed Ticket back to Ready, with an
// optional note, where no note tells the next agent nothing has changed;
// or an Accepted one whose CI failed, to fix it on its branch, with the
// note required (code-central-264).
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    send_back: { body: { note?: string }; result: TicketAcknowledgement };
  }
}
