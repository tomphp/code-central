// move: the human moves a Ticket to any State but its own, Needs Answer
// and Failed (entered only by a Round or a Failure Report). This file is
// the pattern for every human action: declare its body and result by
// merging into TicketActions.
import { STATES, type State, type TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    move: { body: { to_state: State }; result: TicketAcknowledgement };
  }
}

// MOVE_EXCEPT is the move row's `except` in the published workflow: the
// States a move never enters.
export const MOVE_EXCEPT: readonly State[] = ["Needs Answer", "Failed"];

// moveTargets is every State a Ticket in from may be moved to.
export function moveTargets(from: State): State[] {
  return STATES.filter((s) => s !== from && !MOVE_EXCEPT.includes(s));
}
