// cancel: the human cancels any unfinished Ticket, optionally naming
// Replacements that take over its work and a reason (stored as a comment).
// The fake stores the Replacements as given; core checks them.
import { registerFakeAction } from "../fakeActions";
import { fakeTransition } from "../fakeTransition";
import { STATES, type TicketView } from "../types";

declare module "../api" {
  interface TicketActions {
    cancel: { body: { replacements?: string[]; reason?: string }; result: TicketView };
  }
}

const unfinished = STATES.filter((s) => s !== "Done" && s !== "Cancelled");

registerFakeAction("cancel", (fake, t, { replacements = [], reason }) =>
  fakeTransition(fake, t, "cancel", unfinished, "Cancelled", reason, { replacements }),
);
