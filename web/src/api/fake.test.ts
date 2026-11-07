import { apiContract, type Driver } from "./contract";
import { FakeApi } from "./fake";

// The fake half of the contract suite. The driver plays the agents by
// editing the fake the way the service would end up.
apiContract("FakeApi", async (): Promise<Driver> => {
  const fake = new FakeApi();
  return {
    api: fake,
    createProject: async (key, name) => fake.addProject(key, name),
    createTicket: async (key, title, state) => fake.addTicket(key, { title, state }).id,
    postRound: async (key, id, questions) => {
      const posted = fake.now.toISOString();
      fake.updateTicket(id, {
        state: "Needs Answer",
        rounds: [
          {
            number: 1,
            claim: 1,
            posted,
            questions: questions.map((q, i) => ({ number: i + 1, ...q, status: "open" as const })),
          },
        ],
      });
      const needs = (await fake.needsMe()).groups;
      const group = needs.find((g) => g.reason === "needs_answer") ?? { reason: "needs_answer" as const, tickets: [] };
      group.tickets.push({
        project: key,
        id,
        title: (await fake.getTicket(key, id)).title,
        state: "Needs Answer",
        reason: "needs_answer",
        since: posted,
        round: { number: 1, open: questions.length, questions: questions.length },
      });
      fake.setNeedsMe([group, ...needs.filter((g) => g !== group)]);
    },
    addBlocker: async (_key, id, blocker) => {
      const b = await fake.getTicket(_key, blocker);
      const t = await fake.getTicket(_key, id);
      fake.updateTicket(id, {
        blocked_by: [...t.blocked_by, blocker],
        blockers: [...t.blockers, { id: blocker, title: b.title, state: b.state }],
        blocked: true,
      });
    },
    // ticket 35
    claim: async (_key, id, session_id) => {
      fake.updateTicket(id, {
        state: "In Progress",
        claims: [{ number: 1, session_id, started: fake.now.toISOString(), last_activity: fake.now.toISOString() }],
        moves: [{ from: "Ready", to: "In Progress", actor: "agent", time: fake.now.toISOString(), claim: 1 }],
      });
    },
    // ticket 42
    linkTicket: async (key, title, link, external_state) => {
      const t = fake.addTicket(key, { title, link: { ...link, external_state, detached: false } });
      if (external_state !== "open") {
        const needs = (await fake.needsMe()).groups;
        fake.setNeedsMe([...needs, { reason: "closed_externally", tickets: [{ id: t.id, link: t.link }] }]);
      }
      return t.id;
    },
    close: async () => {},
  };
});
