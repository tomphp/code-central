// Fixed Usage figures for UsageBody's stories and prop tests.
import type { UsageSession, UsageView } from "../api/types";

const session = (over: Partial<UsageSession>): UsageSession => ({
  project: "acme",
  ticket: "acme-1",
  claim: 1,
  session_id: "s1",
  kind: "implementation",
  started: "2026-11-07T09:00:00Z",
  peak: 80_000,
  responses: 12,
  models: ["sonnet"],
  tokens: 120_000,
  subagents: [],
  ...over,
});

export const operations: UsageView["operations"] = [
  { op: "claim", calls: 4, request_bytes: 800, average_request_bytes: 200, response_bytes: 4000, average_response_bytes: 1000, errors: 0 },
  { op: "get_ticket", calls: 20, request_bytes: 2000, average_request_bytes: 100, response_bytes: 60000, average_response_bytes: 3000, errors: 1 },
  { op: "finish", calls: 2, request_bytes: 1200, average_request_bytes: 600, response_bytes: 200, average_response_bytes: 100, errors: 0 },
];

const days = ["2026-11-06", "2026-11-07", "2026-11-08"].map((day, i) => ({
  day,
  kinds: [{ kind: "implementation", input_tokens: 1000 * (i + 1), cache_write_tokens: 500, cache_read_tokens: 9000, output_tokens: 700 }],
}));

export const populated: UsageView = {
  operations,
  kinds: [{ kind: "implementation", sessions: 2, average_peak: 70_000, largest_peak: 80_000 }],
  sessions: [
    session({
      subagents: [session({ agent: "a1", agent_type: "Explore", description: "Map Usage", peak: 30_000, tokens: 20_000, responses: 3 })],
      total_tokens: 140_000,
    }),
    session({ session_id: "s2", ticket: "acme-2", peak: 60_000 }),
  ],
  days,
};

export const empty: UsageView = { operations: [], kinds: [], sessions: [], days };

export const manyProjects: UsageView = {
  ...populated,
  sessions: [
    ...populated.sessions,
    session({ project: "globex", ticket: "globex-7", session_id: "s3" }),
    session({ project: "initech", ticket: "initech-3", session_id: "s4" }),
  ],
};
