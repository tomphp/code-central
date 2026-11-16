// Activity Dashboard tests: the real App on a WASM Core world, at
// ?view=focus, with the Focus Mode Option at its default, Activity
// Dashboard. They check what the human sees: where the dashboard shows, its
// counts and its In Progress list.
import { screen, within, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { WORK, type Scenario } from "../api/scenario";
import { ApiError, type CIRunView } from "../api/types";
import { focus, loadedDashboard, renderApp, scenario, settle } from "./worldTesting";

// dash is a world with Acme and Beta, its scenario and the IDs of its Tickets:
// Login form and Sign-up page (In Progress) in Acme, Beta export in Beta,
// and Old work Done.
async function dash() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  const login = await s.ticket("acme", { title: "Login form", state: "Ready" });
  const signup = await s.ticket("acme", { title: "Sign-up page", state: "In Progress" });
  const betaExport = await s.ticket("beta", { title: "Beta export", state: "Ready" });
  await s.ticket("acme", { title: "Old work", state: "Done" });
  return { w, s, login, signup, betaExport };
}

// dashboard settles w, then finds the Focus Mode region's Activity Dashboard.
const dashboard = (w: World) => loadedDashboard(w, focus());
// block is state's block, named by its State; count is the number on it.
const block = (d: HTMLElement, state: string) => within(d).getByRole("group", { name: state });
const count = (d: HTMLElement, state: string) => within(block(d, state)).getByTestId("count").textContent;

// touch has the agent comment on a Ticket: a change that shows nothing.
const touch = (s: Scenario, key: string, id: string) => s.agent(`${s.ticketPath(key, id)}/comment`, { body: "Noted" });
// typed has the Session s-busy post a new latest event. Core sends a Ticket's
// latest-event notices at most every 5 seconds of its clock, so the clock
// moves past the last.
async function typed(w: World, s: Scenario) {
  w.advance(5_000);
  await s.postSessionEvents("s-busy", [{ time: w.now().toISOString(), kind: "text", text: "Writing the test first." }]);
}
// cancel has the human cancel a Ticket.
const cancel = (s: Scenario, key: string, id: string) => s.human(`${s.ticketPath(key, id)}/cancel`, {});
// done takes a Ticket through acceptance to Done.
async function done(s: Scenario, key: string, id: string) {
  await s.accepted(key, id);
  await s.human(`${s.ticketPath(key, id)}/land`, { commit: WORK.commit });
}

describe("where the Activity Dashboard shows", () => {
  it("shows where All clear would by default", async () => {
    const { w } = await dash();
    await renderApp(w);
    const d = await dashboard(w);
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
  });

  it("shows All clear, never the dashboard, until the Options are read", async () => {
    const { w } = await dash();
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const options = w.api.options.bind(w.api);
    vi.spyOn(w.api, "options").mockImplementation(async () => {
      await gate;
      return options();
    });
    await renderApp(w);
    expect(within(focus()).getByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    release();
    expect(await dashboard(w)).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
  });

  it("shows All clear, not the dashboard, when the Options can't be read", async () => {
    const { w } = await dash();
    vi.spyOn(w.api, "options").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));
    await renderApp(w);
    expect(within(focus()).getByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();
    expect(screen.queryByText("Code Central isn't reachable")).toBeNull();
  });

  it("switches live between the dashboard and All clear on options.changed, with no reload", async () => {
    const { w } = await dash();
    await renderApp(w);
    await dashboard(w);

    await act(() => w.api.setOptions({ focus_clear: "all_clear" }));
    await settle(w);
    expect(within(focus()).getByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    await act(() => w.api.setOptions({ focus_clear: "activity_dashboard" }));
    expect(await dashboard(w)).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
  });

  it("gives way to a Focus Item the moment one appears, and comes back when it goes", async () => {
    const { w, s, signup } = await dash();
    await renderApp(w);
    await dashboard(w);

    await act(() => s.fail("acme", signup));
    expect(within(focus()).getByRole("article", { name: "Focus Item" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    await act(() => cancel(s, "acme", signup));
    expect(await dashboard(w)).toBeInTheDocument();
  });

  it("leaves the skipped screen as it is", async () => {
    const { w, s, signup } = await dash();
    await s.fail("acme", signup);
    await renderApp(w);
    await userEvent.click(within(focus()).getByRole("button", { name: "Skip" }));
    await settle(w);

    expect(within(focus()).getByRole("region", { name: "Only skipped Focus Items left" })).toHaveTextContent("You skipped 1, go through them again?");
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();
  });
});

describe("the counts", () => {
  it("cover only Focus Mode's Project when scoped, and every Project when not", async () => {
    const { w } = await dash();
    const { unmount } = await renderApp(w, "/?view=focus&project=beta");
    let d = await dashboard(w);
    expect(count(d, "Ready")).toBe("1");
    expect(count(d, "In Progress")).toBe("0");
    unmount();

    await renderApp(w);
    d = await dashboard(w);
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
  });

  // An Archived Project has only finished Tickets (Core refuses to archive
  // one with work left), so it adds nothing to the counts or the list.
  it("leave out Archived Projects", async () => {
    const { w, s } = await dash();
    await s.project("old", "Old");
    await done(s, "old", await s.ticket("old", { title: "Left behind" }));
    await w.api.archiveProject("old");
    await renderApp(w);
    const d = await dashboard(w);
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
    expect(within(d).queryByText("Left behind")).toBeNull();
  });

  it("include Stage Tickets and blocked Ready Tickets, and follow a change live", async () => {
    const { w, s, login } = await dash();
    const spec = await s.spec("acme", { title: "Dark mode" });
    await s.ticket("acme", { title: "Grilling: Dark mode", state: "In Progress", spec, labels: ["stage:grilling"] });
    const waits = await s.ticket("acme", { title: "Waits on login" });
    await s.addBlocker("acme", waits, login);
    await renderApp(w);
    const d = await dashboard(w);
    expect(count(d, "Ready")).toBe("3");
    expect(count(d, "In Progress")).toBe("2");

    await act(() => s.claimFor("acme", login));
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("3");
  });
});

// busy is dash with more work In Progress: Beta import, the Spec Stage
// Ticket of the Dark mode Requirement and Theme toggle, with a Session
// posting events. The Requirement and the Tickets after it are numbered
// from acme-10 on, so the list's order shows numbers are sorted as numbers.
async function busy() {
  const c = await dash();
  const { w, s } = c;
  const betaImport = await s.ticket("beta", { title: "Beta import", state: "In Progress" });
  let last = 0;
  while (last < 9) last = Number((await s.spec("acme", { title: "Filler" })).split("-").pop());
  const requirement = await s.spec("acme", { title: "Dark mode" });
  const stage = await s.ticket("acme", { title: "Spec: Dark mode", state: "In Progress", spec: requirement, labels: ["stage:spec"] });
  const theme = await s.ticket("acme", { title: "Theme toggle" });
  await s.factorySession("acme", theme, {
    session: "s-busy",
    events: [{ time: new Date(w.now().getTime() - 2 * 60_000).toISOString(), kind: "tool_call", tool: "Read", text: "Read: web/src/App.tsx" }],
  });
  return { ...c, betaImport, requirement, stage, theme };
}

const inProgress = (d: HTMLElement) => within(d).getByRole("list", { name: "In Progress" });
const entries = (d: HTMLElement) => within(inProgress(d)).getAllByRole("listitem");

describe("the In Progress list", () => {
  it("lists the In Progress Tickets by Project, then Ticket number", async () => {
    const { w, signup, stage, theme, betaImport } = await busy();
    await renderApp(w);
    const d = await dashboard(w);
    expect(entries(d).map((e) => within(e).getByText(/^(acme|beta)-\d+$/).textContent)).toEqual([signup, stage, theme, betaImport]);
  });

  it("shows each entry's ID, title, Project and latest event line", async () => {
    const { w, signup, theme, betaImport } = await busy();
    await renderApp(w);
    const d = await dashboard(w);
    const [quiet, , themed, beta] = entries(d);
    expect(quiet).toHaveTextContent(new RegExp(`^${signup}Sign-up pageAcme$`));
    expect(themed).toHaveTextContent(new RegExp(`^${theme}Theme toggleAcmeRead: web/src/App.tsx · 2m ago$`));
    expect(beta).toHaveTextContent(new RegExp(`^${betaImport}Beta importBeta$`));
  });

  it("shows a Stage Ticket's Requirement and Stage", async () => {
    const { w } = await busy();
    await renderApp(w);
    const stage = entries(await dashboard(w))[1];
    expect(within(stage).getByText("Requirement: Dark mode · Spec")).toBeInTheDocument();
  });

  it("leaves out the Project when scoped to one", async () => {
    const { w, signup } = await busy();
    await renderApp(w, "/?view=focus&project=acme");
    const d = await dashboard(w);
    expect(entries(d)[0]).toHaveTextContent(new RegExp(`^${signup}Sign-up page$`));
    expect(entries(d)).toHaveLength(3);
  });

  it("follows the live feed: a new latest event, and a Ticket leaving In Progress", async () => {
    const { w, s, betaImport } = await busy();
    await renderApp(w);
    const d = await dashboard(w);
    await act(() => typed(w, s));
    expect(within(d).getByText("Writing the test first. · just now")).toBeInTheDocument();

    // Core makes an In Review Ticket a Focus Item, which would take over
    // from the dashboard, so the Ticket leaves by being cancelled.
    await act(() => cancel(s, "beta", betaImport));
    expect(count(d, "In Progress")).toBe("3");
    expect(within(d).queryByText("Beta import")).toBeNull();
  });

  it("says Nothing in progress when it's empty, with the blocks still showing", async () => {
    const { w } = await dash();
    await renderApp(w, "/?view=focus&project=beta");
    const d = await dashboard(w);
    expect(within(d).getByText("Nothing in progress.")).toBeInTheDocument();
    expect(within(d).queryByRole("listitem")).toBeNull();
    expect(count(d, "Ready")).toBe("1");
    expect(count(d, "In Progress")).toBe("0");
  });

  it("opens an entry's Ticket in the Inbox, keeping the Project filter", async () => {
    const { w, signup } = await busy();
    await renderApp(w, "/?view=focus&project=acme");
    await userEvent.click(within(entries(await dashboard(w))[0]).getByRole("link"));
    await settle(w);
    expect(window.location.search).toBe(`?view=inbox&project=acme&ticket=${signup}`);
    expect(screen.getByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
  });

  it("opens a Stage Ticket in its Requirement", async () => {
    const { w, requirement, stage } = await busy();
    await renderApp(w);
    await userEvent.click(within(entries(await dashboard(w))[1]).getByRole("link"));
    expect(window.location.search).toBe(`?view=inbox&requirement=${requirement}&ticket=${stage}`);
  });
});

// pulsing is whether el shows it is pulsing.
const pulsing = (el: HTMLElement) => el.getAttribute("data-pulsing") === "true";
const entry = (d: HTMLElement, id: string) => within(inProgress(d)).getByText(id).closest("li") as HTMLElement;

describe("pulses", () => {
  it("marks neither a block nor an entry on first show", async () => {
    const { w } = await busy();
    await renderApp(w);
    const d = await dashboard(w);
    expect(pulsing(block(d, "Ready"))).toBe(false);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
    expect(entries(d).filter(pulsing)).toEqual([]);
  });

  it("marks a block when its count changes, and not the other", async () => {
    const { w, s, betaExport } = await busy();
    await renderApp(w);
    const d = await dashboard(w);

    await act(() => done(s, "beta", betaExport));
    expect(count(d, "Ready")).toBe("1");
    expect(pulsing(block(d, "Ready"))).toBe(true);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
  });

  it("marks only the entry whose Ticket a notice names, and no block when a refetch leaves the counts the same", async () => {
    const { w, s, theme } = await busy();
    await renderApp(w);
    const d = await dashboard(w);

    await act(() => typed(w, s));
    expect(within(d).getByText("Writing the test first. · just now")).toBeInTheDocument();
    expect(entries(d).filter(pulsing)).toEqual([entry(d, theme)]);
    expect(pulsing(block(d, "Ready"))).toBe(false);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
  });

  it("marks an entry on a notice that changes nothing it shows", async () => {
    const { w, s, betaImport } = await busy();
    await renderApp(w);
    const d = await dashboard(w);

    await act(() => touch(s, "beta", betaImport));
    expect(pulsing(entry(d, betaImport))).toBe(true);
    expect(entries(d).filter(pulsing)).toEqual([entry(d, betaImport)]);
  });

  it("marks an entry that joins the list", async () => {
    const { w, s, login } = await busy();
    await renderApp(w);
    const d = await dashboard(w);

    await act(() => s.claimFor("acme", login));
    expect(within(d).getByText("Login form")).toBeInTheDocument();
    expect(entries(d).filter(pulsing)).toEqual([entry(d, login)]);
    expect(pulsing(block(d, "In Progress"))).toBe(true);
  });

  it("marks the first entry to join an empty list", async () => {
    const { w, s, betaExport } = await dash();
    await renderApp(w, "/?view=focus&project=beta");
    const d = await dashboard(w);

    await act(() => s.claimFor("beta", betaExport));
    expect(within(d).getByText("Beta export")).toBeInTheDocument();
    expect(pulsing(entry(d, betaExport))).toBe(true);
  });
});

// ---- The CI Running block (code-central-277) ----

// ciWorld is dash with CI Tracking and Push on in Acme, and Theme toggle
// Accepted (In Review would be a Focus Item) with abc123 pushed. Beta keeps CI Tracking off.
async function ciWorld() {
  const c = await dash();
  const { w, s } = c;
  await w.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
  await w.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"] });
  const theme = await s.ticket("acme", { title: "Theme toggle", state: "Accepted" });
  await s.reportPushedCommit("acme", theme, "abc123");
  return { ...c, theme };
}
type CI = Awaited<ReturnType<typeof ciWorld>>;

// ciRun is a run of workflow on theme's abc123, started minutes ago and still going.
function ciRun(c: CI, id: number, workflow: string, more: Partial<CIRunView> = {}): Omit<CIRunView, "ignored"> {
  return {
    id,
    attempt: 1,
    workflow,
    name: workflow,
    branch: c.theme,
    commit: "abc123",
    status: "in_progress",
    started: new Date(c.w.now().getTime() - id * 60_000).toISOString(),
    url: `https://github.com/acme/web/actions/runs/${id}`,
    jobs: [],
    ticket: c.theme,
    ...more,
  };
}
// finished is a run that has completed.
const finished = (c: CI, id: number, workflow: string, conclusion: "success" | "failure", more: Partial<CIRunView> = {}) =>
  ciRun(c, id, workflow, { status: "completed", conclusion, finished: c.w.now().toISOString(), ...more });

const ciBlock = (d: HTMLElement) => block(d, "CI Running");

describe("the CI Running block", () => {
  it("counts the CI Runs that count towards a Ticket's CI result, after Ready and In Progress", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [
      ciRun(c, 1, "ci.yml"),
      ciRun(c, 2, "lint.yml", { status: "queued" }),
      finished(c, 3, "e2e.yml", "success"),
      ciRun(c, 4, "nightly.yml"),
    ]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(count(d, "CI Running")).toBe("2");
    expect(within(ciBlock(d)).getByText("CI Running")).toBeInTheDocument();
    const blocks = within(d).getAllByRole("group").map((g) => g.getAttribute("aria-label"));
    expect(blocks).toEqual(["Ready", "In Progress", "CI Running"]);
  });

  it("reads 0 with nothing running", async () => {
    const { w } = await ciWorld();
    await renderApp(w);
    const d = await dashboard(w);
    expect(count(d, "CI Running")).toBe("0");
  });

  it("isn't there, and reads nothing, without CI Tracking in scope", async () => {
    const { w } = await dash();
    await renderApp(w);
    const d = await dashboard(w);
    expect(within(d).getAllByRole("group").map((g) => g.getAttribute("aria-label"))).toEqual(["Ready", "In Progress"]);
    expect(w.calls).not.toContain("ciOverview");
  });

  it("honours the Project filter: not there in a Project with CI Tracking off", async () => {
    const { w } = await ciWorld();
    await renderApp(w, "/?view=focus&project=beta");
    const d = await dashboard(w);
    expect(within(d).queryByRole("group", { name: "CI Running" })).toBeNull();
    expect(w.calls).not.toContain("ciOverview");
  });

  // Core archives a Project only once its Tickets are finished, so the
  // Project with CI Tracking on that is Archived has none left to run.
  it("isn't there when the only Project with CI Tracking on is Archived", async () => {
    const { w, s, theme } = await ciWorld();
    await s.project("old", "Old");
    await w.api.setCITracking("old", { tracking: true, provider: "github_actions", repository: "old/web", ignored_workflows: [] });
    await done(s, "old", await s.ticket("old", { title: "Left behind" }));
    await w.api.archiveProject("old");
    await w.api.setCITracking("acme", { tracking: false, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    expect(theme).toBeTruthy();
    await renderApp(w);
    const d = await dashboard(w);
    expect(within(d).queryByRole("group", { name: "CI Running" })).toBeNull();
    expect(w.calls).not.toContain("ciOverview");
  });

  it("reads the running runs in Focus Mode's Project scope", async () => {
    const { w } = await ciWorld();
    const read = vi.spyOn(w.api, "ciOverview");
    await renderApp(w, "/?view=focus&project=acme");
    expect(read).toHaveBeenCalledWith({ project: "acme", running: true });
  });

  it("opens the CI view in Focus Mode's scope, with Back to Focus Mode", async () => {
    const { w } = await ciWorld();
    await renderApp(w, "/?view=focus&project=acme");
    const d = await dashboard(w);
    // The block is drawn afresh when its first read arrives.
    expect(count(d, "CI Running")).toBe("0");
    const link = within(ciBlock(d)).getByRole("link");
    expect(link).toHaveAttribute("href", "?view=ci&project=acme&back=view%3Dfocus%26project%3Dacme");

    await userEvent.click(link);
    await settle(w);
    expect(window.location.search).toBe("?view=ci&project=acme&back=view%3Dfocus%26project%3Dacme");
    await userEvent.click(screen.getByRole("button", { name: "Back to Focus Mode" }));
    await settle(w);
    expect(window.location.search).toBe("?view=focus&project=acme");
  });

  it("opens the CI view on All Projects from All Projects", async () => {
    const { w } = await ciWorld();
    await renderApp(w);
    const d = await dashboard(w);
    expect(count(d, "CI Running")).toBe("0");
    expect(within(ciBlock(d)).getByRole("link")).toHaveAttribute("href", "?view=ci&back=view%3Dfocus");
  });

  it("follows the live feed: runs changing, and a newer pushed commit outside the CI gate", async () => {
    const c = await ciWorld();
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(count(d, "CI Running")).toBe("0");

    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml"), ciRun(c, 2, "lint.yml")]));
    expect(count(d, "CI Running")).toBe("2");

    // Under the CI gate an earlier pushed commit still counts (code-central-430).
    await act(() => c.w.api.setLandingMethod("acme", { method: "branch", push: true, remote: "origin" }));
    await act(() => c.s.reportPushedCommit("acme", c.theme, "def456"));
    // Core makes an Accepted Ticket the branch method leaves a Focus Item
    // (To merge), which takes over from the dashboard, so the runs it no
    // longer counts are read from the service.
    expect(within(focus()).getByRole("article", { name: "Focus Item" })).toBeInTheDocument();
    expect((await c.w.api.ciOverview({ running: true })).runs).toHaveLength(0);
  });

  it("rereads the running runs on a ticket.changed notice", async () => {
    const { w, s, theme } = await ciWorld();
    const read = vi.spyOn(w.api, "ciOverview");
    await renderApp(w);
    const d = await dashboard(w);
    expect(count(d, "CI Running")).toBe("0");
    const reads = read.mock.calls.length;

    await act(() => touch(s, "acme", theme));
    expect(read.mock.calls.length).toBeGreaterThan(reads);
  });

  it("pulses when its count changes, but not on first show", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml")]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(count(d, "CI Running")).toBe("1");
    expect(pulsing(ciBlock(d))).toBe(false);

    await act(() => c.s.reportCIRuns("acme", [finished(c, 1, "ci.yml", "success")]));
    expect(count(d, "CI Running")).toBe("0");
    expect(pulsing(ciBlock(d))).toBe(true);
    expect(pulsing(block(d, "Ready"))).toBe(false);
  });

  it("shows a failed read's error below the In Progress list, keeps the rest, and retries on the next change", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml")]);
    vi.spyOn(c.w.api, "ciOverview").mockRejectedValueOnce(new ApiError(500, "internal", "The CI Runs can't be read"));
    await renderApp(c.w);
    const d = await dashboard(c.w);
    const alert = within(d).getByRole("alert");
    expect(alert).toHaveTextContent("The CI Runs can't be read");
    expect(inProgress(d).compareDocumentPosition(alert) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(ciBlock(d)).toBeInTheDocument();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
    expect(entries(d)).toHaveLength(1);

    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml", { attempt: 2 })]));
    expect(count(d, "CI Running")).toBe("1");
    expect(within(d).queryByRole("alert")).toBeNull();
  });

  it("leaves the Focus Item to take over when something needs the human, and shows no CI Watcher status", async () => {
    const { w, s, signup } = await ciWorld();
    await renderApp(w);
    const d = await dashboard(w);
    expect(count(d, "CI Running")).toBe("0");
    expect(within(d).queryByText(/CI Watcher|Watching|hasn't reported/)).toBeNull();

    await act(() => s.fail("acme", signup));
    expect(within(focus()).getByRole("article", { name: "Focus Item" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();
  });
});

// ---- The CI Running list (code-central-278) ----

const ciList = (d: HTMLElement) => within(d).getByRole("list", { name: "CI Running" });
const ciEntries = (d: HTMLElement) => within(ciList(d)).getAllByRole("listitem");
const ciEntry = (d: HTMLElement, name: string) => within(ciList(d)).getByRole("listitem", { name });
const labels = (d: HTMLElement) => ciEntries(d).map((e) => e.getAttribute("aria-label"));

describe("the CI Running list", () => {
  it("lists the runs the block counts, newest started first, below the In Progress list", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [
      ciRun(c, 3, "ci.yml"),
      ciRun(c, 1, "lint.yml", { status: "queued" }),
      finished(c, 2, "e2e.yml", "success"),
      ciRun(c, 4, "nightly.yml"),
    ]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(labels(d)).toEqual(["lint.yml", "ci.yml"]);
    expect(count(d, "CI Running")).toBe("2");
    expect(inProgress(d).compareDocumentPosition(ciList(d)) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("reads No CI Runs running when nothing is", async () => {
    const { w } = await ciWorld();
    await renderApp(w, "/?view=focus&project=acme");
    const d = await dashboard(w);
    expect(count(d, "CI Running")).toBe("0");
    expect(within(d).getByText("No CI Runs running.")).toBeInTheDocument();
    expect(within(d).queryByRole("list", { name: "CI Running" })).toBeNull();
  });

  it("isn't there without CI Tracking in scope", async () => {
    const { w } = await dash();
    await renderApp(w);
    const d = await dashboard(w);
    expect(within(d).queryByRole("list", { name: "CI Running" })).toBeNull();
    expect(within(d).queryByText("No CI Runs running.")).toBeNull();
  });

  it("shows each entry's workflow, ID, Project, branch, short commit and how long it has been running", async () => {
    const c = await ciWorld();
    await c.s.reportPushedCommit("acme", c.theme, "abc1234def");
    await c.s.reportCIRuns("acme", [ciRun(c, 3, "ci.yml", { name: "CI", commit: "abc1234def" }), ciRun(c, 2, "lint.yml", { name: "", commit: "abc1234def" })]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(2);
    expect(ciEntry(d, "CI")).toHaveTextContent(new RegExp(`^${c.theme}CIAcme${c.theme} · abc1234 · running 3m 0sView on GitHub$`));
    expect(ciEntry(d, "lint.yml")).toHaveTextContent(new RegExp(`^${c.theme}lint.ymlAcme${c.theme} · abc1234 · running 2m 0sView on GitHub$`));
  });

  it("says a queued run is queued, and leaves out the duration of one not started", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 2, "ci.yml", { status: "queued" }), ciRun(c, 1, "lint.yml", { status: "queued", started: undefined })]);
    await renderApp(c.w, "/?view=focus&project=acme");
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(2);
    expect(ciEntry(d, "ci.yml")).toHaveTextContent(new RegExp(`^${c.theme}ci.yml${c.theme} · abc123 · queued 2m 0sView on GitHub$`));
    expect(ciEntry(d, "lint.yml")).toHaveTextContent(new RegExp(`^${c.theme}lint.yml${c.theme} · abc123 · queuedView on GitHub$`));
  });

  it("leaves out the Project when scoped to one", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 3, "ci.yml", { name: "CI" })]);
    await renderApp(c.w, "/?view=focus&project=acme");
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(1);
    expect(ciEntry(d, "CI")).toHaveTextContent(new RegExp(`^${c.theme}CI${c.theme} · abc123 · running 3m 0sView on GitHub$`));
  });

  it("opens an entry's Ticket in the Inbox, keeping the Project filter", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml", { name: "CI" })]);
    await renderApp(c.w, "/?view=focus&project=acme");
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(1);
    const link = within(ciEntry(d, "CI")).getByRole("link", { name: new RegExp(c.theme) });
    expect(link).toHaveAttribute("href", `?view=inbox&project=acme&ticket=${c.theme}`);

    await userEvent.click(link);
    await settle(c.w);
    expect(window.location.search).toBe(`?view=inbox&project=acme&ticket=${c.theme}`);
    expect(screen.getByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
  });

  it("opens a Stage Ticket's run in its Requirement", async () => {
    const c = await ciWorld();
    const requirement = await c.s.spec("acme", { title: "Dark mode" });
    const stage = await c.s.ticket("acme", { title: "Tickets: Dark mode", state: "Accepted", spec: requirement, labels: ["stage:tickets"] });
    await c.s.reportPushedCommit("acme", stage, "fed987");
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml", { name: "CI", branch: stage, commit: "fed987", ticket: stage })]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(1);
    await userEvent.click(within(ciEntry(d, "CI")).getByRole("link", { name: new RegExp(stage) }));
    expect(window.location.search).toBe(`?view=inbox&requirement=${requirement}&ticket=${stage}`);
  });

  it("carries the run's GitHub link beside the Ticket link, not inside it", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml", { name: "CI" })]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(1);
    const github = within(ciEntry(d, "CI")).getByRole("link", { name: "View on GitHub" });
    expect(github).toHaveAttribute("href", "https://github.com/acme/web/actions/runs/1");
    expect(github).toHaveAttribute("target", "_blank");
    expect(github.parentElement?.closest("a")).toBeNull();
    expect(within(ciEntry(d, "CI")).getAllByRole("link")).toHaveLength(2);
  });

  it("follows the live feed: a run joining, finishing, and a newer pushed commit outside the CI gate", async () => {
    const c = await ciWorld();
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(within(d).getByText("No CI Runs running.")).toBeInTheDocument();

    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml"), ciRun(c, 2, "lint.yml")]));
    expect(labels(d)).toEqual(["ci.yml", "lint.yml"]);

    await act(() => c.s.reportCIRuns("acme", [finished(c, 1, "ci.yml", "failure")]));
    expect(labels(d)).toEqual(["lint.yml"]);

    // Under the CI gate an earlier pushed commit still counts (code-central-430).
    await act(() => c.w.api.setLandingMethod("acme", { method: "branch", push: true, remote: "origin" }));
    await act(() => c.s.reportPushedCommit("acme", c.theme, "def456"));
    // Core makes an Accepted Ticket the branch method leaves a Focus Item
    // (To merge), which takes over from the dashboard, so the runs it no
    // longer counts are read from the service.
    expect(within(focus()).getByRole("article", { name: "Focus Item" })).toBeInTheDocument();
    expect((await c.w.api.ciOverview({ running: true })).runs).toHaveLength(0);
  });

  it("drops a run whose workflow becomes Ignored at its next report", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml"), ciRun(c, 2, "lint.yml")]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(2);

    await act(async () => {
      await c.w.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["lint.yml"] });
      await c.s.reportCIRuns("acme", [ciRun(c, 2, "lint.yml")]);
    });
    expect(labels(d)).toEqual(["ci.yml"]);
  });

  it("shows a failed read's error in its place", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml")]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(1);

    vi.spyOn(c.w.api, "ciOverview").mockRejectedValueOnce(new ApiError(500, "internal", "The CI Runs can't be read"));
    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml", { attempt: 2 })]));
    expect(within(d).getByRole("alert")).toHaveTextContent("The CI Runs can't be read");
    expect(within(d).queryByRole("list", { name: "CI Running" })).toBeNull();
    expect(entries(d)).toHaveLength(1);

    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml", { attempt: 3 })]));
    expect(ciEntries(d)).toHaveLength(1);
    expect(within(d).queryByRole("alert")).toBeNull();
  });
});

describe("CI Running pulses", () => {
  it("marks no entry on first show", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml"), ciRun(c, 2, "lint.yml")]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(2);
    expect(ciEntries(d).filter(pulsing)).toEqual([]);
  });

  it("marks an entry when its run's status changes", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml", { status: "queued" }), ciRun(c, 2, "lint.yml")]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(2);

    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml")]));
    expect(ciEntries(d).filter(pulsing)).toEqual([ciEntry(d, "ci.yml")]);
  });

  it("marks a re-run's next attempt as the same entry", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml"), ciRun(c, 2, "lint.yml")]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(2);
    const before = ciEntry(d, "ci.yml");

    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml", { attempt: 2 })]));
    expect(ciEntries(d).filter(pulsing)).toEqual([ciEntry(d, "ci.yml")]);
    expect(ciEntry(d, "ci.yml")).toBe(before);
    expect(ciEntries(d)).toHaveLength(2);
  });

  it("marks an entry that joins after the dashboard first shows", async () => {
    const c = await ciWorld();
    await c.s.reportCIRuns("acme", [ciRun(c, 2, "lint.yml")]);
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(ciEntries(d)).toHaveLength(1);

    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml")]));
    expect(ciEntries(d)).toHaveLength(2);
    expect(ciEntries(d).filter(pulsing)).toEqual([ciEntry(d, "ci.yml")]);
  });

  it("marks the first entry to join an empty list", async () => {
    const c = await ciWorld();
    await renderApp(c.w);
    const d = await dashboard(c.w);
    expect(within(d).getByText("No CI Runs running.")).toBeInTheDocument();

    await act(() => c.s.reportCIRuns("acme", [ciRun(c, 1, "ci.yml")]));
    expect(ciEntries(d)).toHaveLength(1);
    expect(pulsing(ciEntry(d, "ci.yml"))).toBe(true);
  });
});
