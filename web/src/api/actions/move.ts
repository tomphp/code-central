// move: the human moves a Ticket to any State but its own, Needs Answer
// and Failed (entered only by a Round or a Failure Report). This file is
// the pattern for every human action: declare its body and result by
// merging into TicketActions, then register the fake's behaviour.
import { registerFakeAction } from "../fakeActions";
import { ApiError, STATES, type State, type TicketView } from "../types";

declare module "../api" {
  interface TicketActions {
    move: { body: { to_state: State }; result: TicketView };
  }
}

// MOVE_EXCEPT is the move row's `except` in the published workflow: the
// States a move never enters.
export const MOVE_EXCEPT: readonly State[] = ["Needs Answer", "Failed"];

// moveTargets is every State a Ticket in from may be moved to.
export function moveTargets(from: State): State[] {
  return STATES.filter((s) => s !== from && !MOVE_EXCEPT.includes(s));
}

registerFakeAction("move", (fake, t, { to_state }) => {
  if (!moveTargets(t.state).includes(to_state)) {
    throw new ApiError(422, "invalid", `can't move ${t.id} from ${t.state} to ${to_state}`, "to_state");
  }
  return fake.updateTicket(t.id, {
    state: to_state,
    moves: [...t.moves, { from: t.state, to: to_state, actor: "human", time: fake.now.toISOString() }],
  });
});
