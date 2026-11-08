// reject: the human sends an In Review Ticket back to Ready with a
// required comment saying what to change.
import { registerFakeAction } from "../fakeActions";
import { fakeTransition } from "../fakeTransition";
import { ApiError, type TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    reject: { body: { comment: string }; result: TicketAcknowledgement };
  }
}

registerFakeAction("reject", (fake, t, { comment }) => {
  if (!comment?.trim()) throw new ApiError(422, "invalid", "reject needs a comment saying what to change", "comment");
  return fakeTransition(fake, t, "reject", ["In Review"], "Ready", comment);
});
