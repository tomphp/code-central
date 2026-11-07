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
    close: async () => {},
  };
});
