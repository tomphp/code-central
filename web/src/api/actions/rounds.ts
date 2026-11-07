// The human's Round actions: answer(question, text), dismiss_question
// (question) and accept_recommended(round). Question numbers run across
// the whole Ticket, so answer and dismiss_question name a Question, not a
// Round. Answers stay editable until the Round is complete; the action
// that closes its last open Question moves the Ticket to Answered.
import type { FakeApi } from "../fake";
import { registerFakeAction } from "../fakeActions";
import { ApiError, type RoundView, type TicketView } from "../types";

declare module "../api" {
  interface TicketActions {
    answer: { body: { question: number; text: string }; result: TicketView };
    dismiss_question: { body: { question: number }; result: TicketView };
    accept_recommended: { body: { round: number }; result: TicketView };
  }
}

registerFakeAction("answer", (fake, t, { question, text }) => {
  if (!text.trim()) throw new ApiError(422, "invalid", "an answer needs text; to decline a Question, dismiss it", "text");
  return onQuestion(fake, t, question, (q) => {
    q.status = "answered";
    q.answer = text;
  });
});

registerFakeAction("dismiss_question", (fake, t, { question }) =>
  onQuestion(fake, t, question, (q) => {
    q.status = "dismissed";
    delete q.answer;
  }),
);

registerFakeAction("accept_recommended", (fake, t, { round }) => {
  const r = t.rounds[round - 1];
  if (!r) throw new ApiError(404, "not_found", `there is no Round ${round} on ${t.id}`);
  return onRound(fake, t, r, () => {
    for (const q of r.questions) {
      if (q.status === "open" && q.recommended) {
        q.status = "answered";
        q.answer = q.recommended;
      }
    }
  });
});

type Question = RoundView["questions"][number];

function onQuestion(fake: FakeApi, t: TicketView, n: number, apply: (q: Question) => void): TicketView {
  for (const r of t.rounds) {
    const q = r.questions.find((q) => q.number === n);
    if (q) return onRound(fake, t, r, () => apply(q));
  }
  throw new ApiError(404, "not_found", `there is no Question ${n} on ${t.id}`);
}

// onRound mirrors core's onRound: no change to a complete Round, the
// change itself, then Answered (and the Needs Me row with it) once no
// Question is open.
function onRound(fake: FakeApi, t: TicketView, r: RoundView, apply: () => void): TicketView {
  const open = () => r.questions.filter((q) => q.status === "open").length;
  if (open() === 0) {
    throw new ApiError(409, "action_not_allowed", `Round ${r.number} on ${t.id} is complete, so its answers can't change; add a comment instead`);
  }
  const before = r.questions.map((q) => JSON.stringify(q));
  apply();
  const now = fake.now.toISOString();
  const changed = r.questions.filter((q, i) => JSON.stringify(q) !== before[i]);
  if (!changed.length) return t;
  for (const q of changed) q.closed = now;
  const done = open() === 0;

  const groups = fake.needsMeGroups;
  const at = groups.findIndex((g) => g.tickets.some((row) => row.id === t.id));
  if (at >= 0) {
    const row = groups[at].tickets.find((row) => row.id === t.id)!;
    if (done) {
      groups[at].tickets = groups[at].tickets.filter((x) => x !== row);
      const answered = groups.find((g) => g.reason === "answered");
      const moved = { ...row, reason: "answered" as const, state: "Answered" as const, since: now, round: undefined };
      if (answered) answered.tickets.push(moved);
      else groups.splice(at + 1, 0, { reason: "answered", tickets: [moved] });
    } else if (row.round) {
      row.round = { ...row.round, open: open() };
    }
    fake.setNeedsMe(groups.filter((g) => g.tickets.length));
  }
  return fake.updateTicket(t.id, {
    rounds: t.rounds,
    ...(done && {
      state: "Answered" as const,
      moves: [...t.moves, { from: t.state, to: "Answered" as const, actor: "human", time: now }],
    }),
  });
}
