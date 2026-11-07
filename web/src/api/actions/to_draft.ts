// to_draft: the human moves a Failed Ticket back to Draft.
import { registerFakeAction } from "../fakeActions";
import { fakeTransition } from "../fakeTransition";
import type { TicketView } from "../types";

declare module "../api" {
  interface TicketActions {
    to_draft: { body: Record<string, never>; result: TicketView };
  }
}

registerFakeAction("to_draft", (fake, t) => fakeTransition(fake, t, "to_draft", ["Failed"], "Draft"));
