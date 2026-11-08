// remove_blocker: the Ticket stops waiting on a stored Blocker. A Blocker
// shown only as a Cancelled Ticket's Replacement (its view has `via`) is
// removed by naming that Cancelled Ticket.
import { registerFakeAction } from "../fakeActions";
import { ApiError, FINISHED, type TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    remove_blocker: { body: { blocker: string }; result: TicketAcknowledgement };
  }
}

registerFakeAction("remove_blocker", (fake, t, { blocker }) => {
  const project = t.id.slice(0, t.id.lastIndexOf("-"));
  const id = /^\d+$/.test(blocker) ? `${project}-${blocker}` : blocker;
  if (!t.blocked_by.includes(id)) {
    throw new ApiError(422, "invalid", `${id} is not a Blocker of ${t.id}`, "blocker");
  }
  const blockers = t.blockers.filter((b) => b.id !== id && b.via !== id);
  if (!blockers.some((b) => b.state === "Cancelled") && t.needs_me_reason === "blocked_by_cancelled") {
    fake.dropNeedsMe(t.id);
  }
  return fake.updateTicket(t.id, {
    blocked_by: t.blocked_by.filter((b) => b !== id),
    blockers,
    blocked: !FINISHED.includes(t.state) && blockers.some((b) => b.state !== "Done"),
  });
});
