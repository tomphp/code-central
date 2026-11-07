// move: the human moves a Ticket to any State but Needs Answer and Failed
// (entered only by a Round or a Failure Report). This file is the pattern
// for every human action: declare its body and result by merging into
// TicketActions, then register the fake's behaviour.
import { registerFakeAction } from "../fakeActions";
import { ApiError, type State, type TicketView } from "../types";

declare module "../api" {
  interface TicketActions {
    move: { body: { to_state: State }; result: TicketView };
  }
}

registerFakeAction("move", (fake, t, { to_state }) => {
  if (to_state === t.state || to_state === "Needs Answer" || to_state === "Failed") {
    throw new ApiError(422, "invalid", `can't move ${t.id} from ${t.state} to ${to_state}`, "to_state");
  }
  return fake.updateTicket(t.id, {
    state: to_state,
    moves: [...t.moves, { from: t.state, to: to_state, actor: "human", time: fake.now.toISOString() }],
  });
});
