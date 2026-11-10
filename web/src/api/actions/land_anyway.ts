// land_anyway: the human accepts an Accepted Ticket's failed CI result,
// so the Factory lands it on its next poll (code-central-261). Offered
// only under Merge: under Branch the human's merge decides.
import { registerFakeAction } from "../fakeActions";
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    land_anyway: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}

registerFakeAction("land_anyway", (fake, t) => fake.landAnyway(t));
