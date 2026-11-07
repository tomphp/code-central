// The fake's behaviour for each human action. Each action lives in its own
// file under ./actions/, which declares the action's types (merging into
// TicketActions) and registers its fake behaviour here; fake.ts loads
// every file in ./actions/ on its own, so adding an action edits no shared
// list.
import type { TicketAction, TicketActions } from "./api";
import type { FakeApi } from "./fake";
import type { TicketView } from "./types";

export type FakeActionHandler<A extends TicketAction> = (
  fake: FakeApi,
  ticket: TicketView,
  body: TicketActions[A]["body"],
) => TicketActions[A]["result"];

export const fakeActions: { [A in TicketAction]?: FakeActionHandler<A> } = {};

export function registerFakeAction<A extends TicketAction>(action: A, handler: FakeActionHandler<A>) {
  (fakeActions as Record<string, unknown>)[action] = handler;
}
