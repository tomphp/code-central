// keep_as_draft: the human keeps an Incoming Issue as a Draft, so it
// leaves Needs Me for good, writing nothing to GitHub (code-central-287).
import { registerFakeAction } from "../fakeActions";
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    keep_as_draft: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}

registerFakeAction("keep_as_draft", (fake, t) => fake.keepAsDraft(t));
