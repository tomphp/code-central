import { apiContract, type Driver } from "./contract";
import { FakeApi } from "./fake";
import type { ApiError, NeedsMeReason, NeedsMeRow, State, TicketView } from "./types";

// claimAndStop is what the service ends up with after an agent claims a
// Ready Ticket (session "contract-session") and makes a stopping move.
function claimAndStop(fake: FakeApi, t: TicketView, to: State, endedBy: string): Partial<TicketView> {
  const at = fake.now.toISOString();
  return {
    state: to,
    claims: [{ number: 1, session_id: "contract-session", started: at, last_activity: at, ended: at, ended_by: endedBy }],
    moves: [
      ...t.moves,
      { from: t.state, to: "In Progress", actor: "agent", time: at },
      { from: "In Progress", to, actor: "agent", time: at, claim: 1 },
    ],
  };
}

// addNeedsMe appends a row to the reason's group, as the service would list it.
async function addNeedsMe(fake: FakeApi, reason: NeedsMeReason, row: Partial<NeedsMeRow> & { id: string }) {
  const groups = (await fake.needsMe()).groups;
  const group = groups.find((g) => g.reason === reason) ?? { reason, tickets: [] };
  group.tickets.push({ ...row, since: fake.now.toISOString() } as NeedsMeRow);
  fake.setNeedsMe(groups.includes(group) ? groups : [...groups, group]);
}

// The fake half of the contract suite. The driver plays the agents by
// editing the fake the way the service would end up.
apiContract("FakeApi", async (): Promise<Driver> => {
  const fake = new FakeApi();
  return {
    api: fake,
    createProject: async (key, name) => fake.addProject(key, name),
    // Every Ticket is under one Spec, as the real driver's are.
    createTicket: async (key, title, state) => fake.addTicket(key, { title, state, spec: { id: `${key}-0`, title: "Contract" } }).id,
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
    reportFailure: async (key, id, report) => {
      const t = await fake.getTicket(key, id);
      const at = fake.now.toISOString();
      fake.updateTicket(id, {
        ...claimAndStop(fake, t, "Failed", "report_failure"),
        failure_reports: [{ number: 1, claim: 1, session_id: "contract-session", actor: "agent", time: at, ...report }],
      });
      await addNeedsMe(fake, "failed", { id, failure: { headline: report.what_broke.split("\n")[0], reports: 1 } });
    },
    finish: async (key, id, summary, on) => {
      const t = await fake.getTicket(key, id);
      const at = fake.now.toISOString();
      fake.updateTicket(id, {
        ...claimAndStop(fake, t, "In Review", "finish"),
        ...on,
        comments: [...t.comments, { number: t.comments.length + 1, actor: "agent", time: at, claim: 1, body: summary }],
      });
      await addNeedsMe(fake, "in_review", { id, summary: summary.split("\n")[0] });
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
    // code-central-287
    incomingIssue: async (key, spec, title, link) => fake.addIncomingIssue(key, spec, title, link).id,
    // code-central-251
    reportPushRemote: async (key, url) => fake.reportPushRemote(key, url),
    reportCIWatcherStatus: async (key, status) => fake.reportCIWatcherStatus(key, status),
    // code-central-256
    reportPushedCommit: async (key, id, commit) => fake.reportPushedCommit(key, id, commit),
    reportCIRuns: async (key, runs) => fake.reportCIRuns(key, runs),
    logOutOfGH: async () => {
      const loggedOut = {
        reason: "gh isn't logged in to GitHub: You are not logged into any GitHub hosts. To log in, run: gh auth login",
        fix: "Run `gh auth login` as the user the service runs as.",
      };
      fake.seedCICheck(loggedOut);
      fake.seedIssuesCheck(loggedOut);
    },
    letGHTriage: async () => fake.seedIssuesCheck(),
    // ticket 109
    postSessionEvents: async (session_id, events) => fake.postSessionEvents(session_id, events),
    // Spec code-central-201: the service records an agent's call itself;
    // the fake answers it with its own reads and records it.
    mcpCall: async (key, op, args) => {
      let text: string;
      let error: string | undefined;
      try {
        const result = op === "get_ticket" ? await fake.getTicket(key, String(args.ticket)) : await fake.findTickets(key);
        text = JSON.stringify(result);
      } catch (e) {
        error = (e as ApiError).code;
        text = JSON.stringify({ code: error, message: (e as ApiError).message });
      }
      const bytes = (s: string) => new TextEncoder().encode(s).length;
      fake.recordCall(key, { op, request_bytes: bytes(JSON.stringify(args)), response_bytes: bytes(text), error });
      return { text, error };
    },
    postUsage: async (session_id, responses) => fake.postUsage(session_id, responses),
    // ticket 110
    dropTranscriptStreams: () => fake.dropTranscriptStreams(),
    close: async () => {},
  };
});
