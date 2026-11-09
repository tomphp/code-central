// approve: the human moves a Draft Ticket to Ready, as Start on a Map's
// frontier does.
import { registerFakeAction } from "../fakeActions";
import { fakeTransition } from "../fakeTransition";
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    approve: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}

registerFakeAction("approve", (fake, t) => fakeTransition(fake, t, "approve", ["Draft"], "Ready", undefined, { claimable: !t.blocked }));
