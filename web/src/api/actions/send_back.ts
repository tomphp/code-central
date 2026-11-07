// send_back: the human sends a Failed Ticket back to Ready, with an
// optional note. No note tells the next agent nothing has changed.
import { registerFakeAction } from "../fakeActions";
import { fakeTransition } from "../fakeTransition";
import type { TicketView } from "../types";

declare module "../api" {
  interface TicketActions {
    send_back: { body: { note?: string }; result: TicketView };
  }
}

registerFakeAction("send_back", (fake, t, { note }) => fakeTransition(fake, t, "send_back", ["Failed"], "Ready", note));
