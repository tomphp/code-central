// release: the human ends a Claim for good, moving an In Progress or
// Answered Ticket back to Ready. The released session's next write is
// rejected as stale. The optional note is stored as a comment.
import { registerFakeAction } from "../fakeActions";
import { ApiError, type TicketView } from "../types";

declare module "../api" {
  interface TicketActions {
    release: { body: { note?: string }; result: TicketView };
  }
}

registerFakeAction("release", (fake, t, { note }) => {
  if (t.state !== "In Progress" && t.state !== "Answered") {
    throw new ApiError(409, "action_not_allowed", `can't release ${t.id}: it is ${t.state}`, undefined, []);
  }
  const at = fake.now.toISOString();
  const live = t.claims.find((c) => !c.ended);
  fake.dropNeedsMe(t.id);
  return fake.updateTicket(t.id, {
    state: "Ready",
    claims: t.claims.map((c) => (c === live ? { ...c, ended: at, ended_by: "release" } : c)),
    comments: note
      ? [...t.comments, { number: t.comments.length + 1, actor: "human", time: at, claim: live?.number, body: note }]
      : t.comments,
    moves: [...t.moves, { from: t.state, to: "Ready", actor: "human", time: at, claim: live?.number }],
  });
});
