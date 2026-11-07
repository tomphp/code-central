// fakeTransition is the fake's version of core's act(): a human action
// from the workflow table that moves a Ticket from one of `from` to `to`,
// with any note stored as a human comment. A Ticket in another State is
// refused with action_not_allowed, as core does. The Ticket leaves Needs
// Me: every action using this resolves the reason it was there for.
import type { FakeApi } from "./fake";
import { ApiError, type State, type TicketView } from "./types";

const claimStates: State[] = ["In Progress", "Needs Answer", "Answered"];

export function fakeTransition(
  fake: FakeApi,
  t: TicketView,
  action: string,
  from: State[],
  to: State,
  note?: string,
  patch: Partial<TicketView> = {},
): TicketView {
  if (!from.includes(t.state)) {
    throw new ApiError(409, "action_not_allowed", `${t.id} is ${t.state}, so ${action} is not allowed`);
  }
  const at = fake.now.toISOString();
  const live = t.claims.find((c) => !c.ended);
  const claims =
    live && claimStates.includes(t.state) && !claimStates.includes(to)
      ? t.claims.map((c) => (c === live ? { ...c, ended: at, ended_by: action } : c))
      : t.claims;
  const comments = note
    ? [...t.comments, { number: t.comments.length + 1, actor: "human", time: at, ...(live && { claim: live.number }), body: note }]
    : t.comments;
  fake.dropNeedsMe(t.id);
  return fake.updateTicket(t.id, {
    state: to,
    claims,
    comments,
    moves: [...t.moves, { from: t.state, to, actor: "human", time: at, ...(live && { claim: live.number }) }],
    ...patch,
  });
}
