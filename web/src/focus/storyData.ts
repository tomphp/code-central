// The Activity Dashboard's and Focus Mode's data for their bodies' stories
// and tests, read from a WASM Core world as the views read it. Only
// stories and tests import this.
import type { Api } from "../api/api";
import { newWorld, type World } from "../api/core/core";
import { SESSION } from "../api/driver";
import { scenario, type Scenario } from "../api/scenario";
import type { CIOverview, CIRunView, TicketView } from "../api/types";
import { loadInbox, type InboxData } from "../live";
import type { FocusBodySession } from "./FocusBody";
import { itemsOf, type FocusItem } from "./session";

// FocusData is Acme and Beta with Flaky build Failed in Acme and Login
// page In Review in Beta, both Focus Items, read back: item is the first
// Focus Item in Needs Me's order, ticket its Ticket's read, remaining how
// many are left. api is the world's, for what the body acts through, so
// the world must outlive the body: build it in a story's loader.
export interface FocusData {
  api: Api;
  now: Date;
  data: InboxData;
  item: FocusItem;
  ticket: TicketView;
  remaining: number;
}

export async function focusData(s: Scenario, w: World): Promise<FocusData> {
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  await s.ticket("acme", { title: "Flaky build", state: "Failed" });
  await s.ticket("beta", { title: "Login page", state: "In Review" });
  const data = await loadInbox(w.api);
  const rows = data.needsMe.groups.flatMap((g) => g.tickets).filter((r) => r.focus_item);
  const [item] = itemsOf(rows[0]);
  return {
    api: w.api,
    now: w.now(),
    data,
    item,
    ticket: await w.api.getTicket(item.row.project, item.row.id),
    remaining: rows.length,
  };
}

// focusSession is f's session with its first Focus Item on screen, and
// callbacks that do nothing, with over over it.
export function focusSession(f: FocusData, over: Partial<FocusBodySession> = {}): FocusBodySession {
  return {
    item: f.item,
    ticket: f.ticket,
    api: f.api,
    remaining: f.remaining,
    clear: false,
    onlySkipped: false,
    skip: () => {},
    again: () => {},
    say: () => {},
    ...over,
  };
}

// DashboardData is Acme and Beta: in Acme, Login form Ready, Sign-up page
// In Progress with a latest Session Event, and Theme toggle Accepted with
// CI Tracking on and two runs going; in Beta, Beta export In Progress and
// Beta import Ready. now is the world's time; ciRuns is the running CI Runs
// as the dashboard reads them. The data is plain, so the world is dropped
// once it is read.
export interface DashboardData {
  now: Date;
  data: InboxData;
  ciRuns: CIOverview;
  ids: { login: string; signup: string; theme: string; betaExport: string };
}

const MIN = 60_000;

export async function dashboardData(): Promise<DashboardData> {
  const w = await newWorld();
  try {
    const s = scenario(w);
    await s.project("acme", "Acme");
    await s.project("beta", "Beta");
    const login = await s.ticket("acme", { title: "Login form", state: "Ready" });
    const signup = await s.ticket("acme", { title: "Sign-up page", state: "In Progress" });
    // A session of its own, so Sign-up page's Session Event is its alone.
    const betaExport = await s.ticket("beta", { title: "Beta export" });
    await s.claimFor("beta", betaExport, "s-beta");
    await s.ticket("beta", { title: "Beta import", state: "Ready" });
    const theme = await s.ticket("acme", { title: "Theme toggle", state: "Accepted" });
    await s.pushed("acme", theme, "abc1234def");
    const run = (id: number, workflow: string, more: Partial<CIRunView> = {}): Omit<CIRunView, "ignored"> => ({
      id,
      attempt: 1,
      workflow,
      name: workflow,
      branch: theme,
      commit: "abc1234def",
      status: "in_progress",
      started: new Date(w.now().getTime() - id * 3 * MIN).toISOString(),
      url: `https://github.com/acme/web/actions/runs/${id}`,
      jobs: [],
      ticket: theme,
      ...more,
    });
    await s.reportCIRuns("acme", [run(1, "ci.yml", { name: "CI" }), run(2, "lint.yml", { status: "queued" })]);
    w.advance(2 * MIN);
    await s.postSessionEvents(SESSION, [{ time: w.now().toISOString(), kind: "tool_call", tool: "Read", text: "Read: web/src/SignUp.tsx" }]);
    return {
      now: w.now(),
      data: await loadInbox(w.api),
      ciRuns: await w.api.ciOverview({ running: true }),
      ids: { login, signup, theme, betaExport },
    };
  } finally {
    w.drop();
  }
}
