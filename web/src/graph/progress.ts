import type { NeedsMeReason, SegmentProgress, TicketRow } from "../api/types";

// nodeProgress is a Ticket graph node's colour, in the Stage strip's
// progress states: done once finished, needs while Needs Me lists it (or
// it waits on the human anyway), queued when a Factory can claim it,
// running while one is on it, and waiting when Draft or blocked.
export function nodeProgress(t: TicketRow, reason?: NeedsMeReason): SegmentProgress {
  if (t.state === "Done" || t.state === "Cancelled") return "done";
  if (reason) return "needs";
  switch (t.state) {
    case "Draft":
      return "waiting";
    case "Ready":
      return t.blocked ? "waiting" : "queued";
    case "In Progress":
    case "Answered":
    case "Accepted":
      return "running";
    default:
      return "needs";
  }
}
