// cancel: the human cancels any unfinished Ticket, naming Replacements or
// none; the optional reason is stored as a comment. (Ticket 36's Failed
// panel and ticket 37's Blocked panel both use it.)
import { registerFakeAction } from "../fakeActions";
import { ApiError, FINISHED, type TicketView } from "../types";

declare module "../api" {
  interface TicketActions {
    cancel: { body: { replacements?: string[]; reason?: string }; result: TicketView };
  }
}

registerFakeAction("cancel", (fake, t, { replacements = [], reason }) => {
  if (FINISHED.includes(t.state)) {
    throw new ApiError(409, "action_not_allowed", `${t.id} is ${t.state}; cancel needs an unfinished Ticket`);
  }
  const at = fake.now.toISOString();
  const project = t.id.slice(0, t.id.lastIndexOf("-"));
  // A dependant's Blocker view shows this Ticket Cancelled (the fake
  // doesn't follow Replacements; seed the dependant if a test needs that).
  if (replacements.length === 0) {
    for (const d of fake.ticketsIn(project)) {
      if (d.blockers.some((b) => b.id === t.id)) {
        fake.updateTicket(d.id, { blockers: d.blockers.map((b) => (b.id === t.id ? { ...b, state: "Cancelled" } : b)) });
      }
    }
  }
  fake.dropNeedsMe(t.id);
  return fake.updateTicket(t.id, {
    state: "Cancelled",
    replacements,
    moves: [...t.moves, { from: t.state, to: "Cancelled", actor: "human", time: at }],
    comments: reason
      ? [...t.comments, { number: t.comments.length + 1, actor: "human", time: at, body: reason }]
      : t.comments,
  });
});
