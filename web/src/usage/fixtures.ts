// Fixed Usage and Storage figures for UsageBody's and the Session popup's
// stories and prop tests.
import type { SessionUsageResponse, StorageView, UsageSession, UsageView } from "../api/types";

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

// sessionResponses are one session's responses, oldest first, its context
// filling up and then compacted, for the Session popup.
export const sessionResponses:SessionUsageResponse[] = [12_000, 34_000, 58_000, 80_000, 41_000, 63_000].map((context_size, i) => ({
  time: `2026-11-07T09:${String(i * 4).padStart(2, "0")}:00Z`,
  context_size,
  input_tokens: 40 + i,
  cache_write_tokens: 2_000,
  cache_read_tokens: context_size - 2_040 - i,
  output_tokens: 600 + 50 * i,
}));

// storage is a Postgres Store two services share, for the Storage tab.
export const storage: StorageView = {
  kind: "postgres",
  host: "db.example.com",
  port: 5432,
  database: "central",
  size: 48_234_496,
  projects: [
    { key: "acme", name: "Acme", specs: 12, tickets: 140, usage_days: 61 },
    { key: "globex", name: "Globex", specs: 3, tickets: 22, usage_days: 9 },
    { key: "initech", name: "Initech", specs: 0, tickets: 0, usage_days: 0 },
  ],
  memory: { resident: 61_865_984, heap: 14_680_064 },
  services: [
    { host: "desktop", version: "1.3.0", renewed: "2026-11-07T09:00:00Z" },
    { host: "laptop", version: "1.2.0", renewed: "2026-11-07T08:59:50Z" },
  ],
};
