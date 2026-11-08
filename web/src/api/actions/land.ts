// land: the work is in the base, so an Accepted (or In Review) Ticket
// moves to Done, recording the commit that put it there on the Move with
// via merge (ADR 0006).
import { registerFakeAction } from "../fakeActions";
import { fakeTransition } from "../fakeTransition";
import { ApiError, type TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    land: { body: { commit: string }; result: TicketAcknowledgement };
  }
}

registerFakeAction("land", (fake, t, { commit }) => {
  const c = commit.trim();
  if (!c) throw new ApiError(422, "invalid", "land needs the commit that put the work into the base", "commit");
  const v = fakeTransition(fake, t, "land", ["Accepted", "In Review"], "Done");
  const last = v.moves.at(-1)!;
  return fake.updateTicket(t.id, { moves: [...v.moves.slice(0, -1), { ...last, via: "merge", commit: c }] });
});
