// send_back: the human sends a Failed Ticket back to Ready, with an
// optional note, where no note tells the next agent nothing has changed;
// or an Accepted one whose CI failed, to fix it on its branch, with the
// note required (code-central-264).
import { registerFakeAction } from "../fakeActions";
import { fakeTransition } from "../fakeTransition";
import { ApiError, type TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    send_back: { body: { note?: string }; result: TicketAcknowledgement };
  }
}

registerFakeAction("send_back", (fake, t, { note }) => {
  if (t.state === "Accepted" && !note?.trim()) {
    throw new ApiError(422, "invalid", "sending back Accepted work needs a note saying why", "note");
  }
  return fakeTransition(fake, t, "send_back", ["Failed", "Accepted"], "Ready", note);
});
