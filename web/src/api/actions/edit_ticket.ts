// edit_ticket: the human changes a Ticket's title or description. Only
// the fields that changed are sent; the service records the text it
// replaces in the Ticket's edits.
import { registerFakeAction } from "../fakeActions";
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    edit_ticket: { body: { title?: string; description?: string }; result: TicketAcknowledgement };
  }
}

registerFakeAction("edit_ticket", (fake, t, body) => fake.editTicket(t, body));
