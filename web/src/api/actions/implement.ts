// implement: the human accepts a Requirement's In Review spec Stage by
// building the whole Spec, skipping the tickets Stage. One action makes
// three writes, as the service does: the spec Stage goes to Done, one Ready
// implementation Ticket is created on the Spec, and the tickets Stage is
// Cancelled with that Ticket as its Replacement.
import { registerFakeAction } from "../fakeActions";
import { fakeTransition } from "../fakeTransition";
import { ApiError, type TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    implement: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}

registerFakeAction("implement", (fake, t) => {
  if (!t.labels.includes("stage:spec")) {
    throw new ApiError(409, "action_not_allowed", `${t.id} is not a spec Stage, so implement is not allowed: only a Requirement's spec Stage is implemented`);
  }
  if (t.state !== "In Review") {
    throw new ApiError(409, "action_not_allowed", `${t.id} is ${t.state}, so implement is not allowed: the spec Stage must be In Review`);
  }
  const project = t.id.slice(0, t.id.lastIndexOf("-"));
  const tickets = fake.ticketsIn(project).find((o) => o.spec.id === t.spec.id && o.labels.includes("stage:tickets"));
  if (!tickets) {
    throw new ApiError(409, "action_not_allowed", `${t.spec.id} has no tickets Stage, so implement is not allowed`);
  }
  if (tickets.state !== "Draft" && tickets.state !== "Ready") {
    throw new ApiError(
      409,
      "action_not_allowed",
      `the tickets Stage ${tickets.id} is ${tickets.state}, so implement is not allowed: it must still be able to run (Draft or Ready, not claimed)`,
    );
  }
  const at = fake.now.toISOString();
  const build = fake.addTicket(project, {
    title: `Implement: ${t.spec.title}`,
    state: "Ready",
    spec: t.spec,
    description: `Implement the whole Spec [${t.spec.id}](${t.spec.id}). Its User Stories, Implementation Decisions and Testing Decisions are the acceptance criteria, including writing the glossary terms and ADRs it records. Read them with \`get_spec\` and \`sections\`.\n`,
    moves: [{ to: "Ready", actor: "human", time: at, via: "implement" }],
    claimable: true,
  });
  fake.dropNeedsMe(tickets.id);
  fake.updateTicket(tickets.id, {
    state: "Cancelled",
    replacements: [build.id],
    claimable: false,
    moves: [...tickets.moves, { from: tickets.state, to: "Cancelled", actor: "human", time: at, via: "implement" }],
    comments: [
      ...tickets.comments,
      { number: tickets.comments.length + 1, actor: "human", time: at, body: `The human chose Implement: ${build.id} builds the Spec without the tickets Stage.` },
    ],
  });
  const done = fakeTransition(fake, t, "implement", ["In Review"], "Done");
  const moves = done.moves.map((m, i) => (i === done.moves.length - 1 ? { ...m, via: "implement" } : m));
  return fake.updateTicket(t.id, { moves });
});
