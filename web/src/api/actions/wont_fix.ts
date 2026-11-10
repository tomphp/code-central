// wont_fix: the human turns an Incoming Issue down, Cancelling it with
// the optional reason as its comment; Code Central then closes its issue
// as not planned, posting the reason there (code-central-289).
import { registerFakeAction } from "../fakeActions";
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    wont_fix: { body: { reason?: string }; result: TicketAcknowledgement };
  }
}

registerFakeAction("wont_fix", (fake, t, { reason }) => fake.wontFix(t, reason));
