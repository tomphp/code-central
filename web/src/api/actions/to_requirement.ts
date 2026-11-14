// to_requirement: the human turns an Incoming Issue into a Requirement
// whose Spec has an outbound Link to the issue, with optional context;
// the Incoming Issue is Cancelled with the grilling Stage as its
// Replacement (code-central-290).
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    to_requirement: { body: { context?: string }; result: TicketAcknowledgement };
  }
}
