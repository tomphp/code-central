// Activity Dashboard tests: the real App against FakeApi, at ?view=focus,
// with the Focus Mode Option at its default, Activity Dashboard. They
// check what the human sees: where the dashboard shows, its counts and
// its In Progress list.
import { screen, within, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { ApiError, type CIRunView } from "../api/types";
import { change, focus, renderApp } from "./focusTesting";

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addTicket("acme", { title: "Login form", state: "Ready" }); // acme-1
  fake.addTicket("acme", { title: "Sign-up page", state: "In Progress" }); // acme-2
  fake.addTicket("beta", { title: "Beta export", state: "Ready" }); // beta-1
  fake.addTicket("acme", { title: "Old work", state: "Done" }); // acme-3
  return fake;
}

const dashboard = () => within(focus()).findByRole("region", { name: "Activity Dashboard" });
// block is state's block, named by its State; count is the number on it.
const block = (d: HTMLElement, state: string) => within(d).getByRole("group", { name: state });
const count = (d: HTMLElement, state: string) => within(block(d, state)).getByTestId("count").textContent;

describe("where the Activity Dashboard shows", () => {
  it("shows where All clear would by default", async () => {
    renderApp(world());
    const d = await dashboard();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
  });

  it("shows All clear, never the dashboard, until the Options are read", async () => {
    const fake = world();
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const options = fake.options.bind(fake);
    vi.spyOn(fake, "options").mockImplementation(async () => {
      await gate;
      return options();
    });
    renderApp(fake);
    expect(await within(focus()).findByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    release();
    expect(await dashboard()).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
  });

  it("shows All clear, not the dashboard, when the Options can't be read", async () => {
    const fake = world();
    vi.spyOn(fake, "options").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));
    renderApp(fake);
    expect(await within(focus()).findByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();
    expect(screen.queryByText("Code Central isn't reachable")).toBeNull();
  });

  it("switches live between the dashboard and All clear on options.changed, with no reload", async () => {
    const fake = world();
    renderApp(fake);
    await dashboard();

    await act(() => fake.setOptions({ focus_clear: "all_clear" }));
    expect(await within(focus()).findByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    await act(() => fake.setOptions({ focus_clear: "activity_dashboard" }));
    expect(await dashboard()).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
  });

  it("gives way to a Focus Item the moment one appears, and comes back when it goes", async () => {
    const fake = world();
    renderApp(fake);
    await dashboard();

    change(fake, [{ reason: "failed", tickets: [{ id: "acme-2" }] }], "acme-2");
    expect(await within(focus()).findByRole("article", { name: "Focus Item" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    change(fake, [], "acme-2");
    expect(await dashboard()).toBeInTheDocument();
  });

  it("leaves the skipped screen as it is", async () => {
    const fake = world();
    fake.updateTicket("acme-2", { state: "Failed" });
    fake.setNeedsMe([{ reason: "failed", tickets: [{ id: "acme-2" }] }]);
    renderApp(fake);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Skip" }));

    expect(await within(focus()).findByRole("region", { name: "Only skipped Focus Items left" })).toHaveTextContent("You skipped 1, go through them again?");
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();
  });
});

describe("the counts", () => {
  it("cover only Focus Mode's Project when scoped, and every Project when not", async () => {
    const fake = world();
    const { unmount } = renderApp(fake, "/?view=focus&project=beta");
    let d = await dashboard();
    expect(count(d, "Ready")).toBe("1");
    expect(count(d, "In Progress")).toBe("0");
    unmount();

    renderApp(fake);
    d = await dashboard();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
  });

  it("leave out Archived Projects", async () => {
    const fake = world();
    fake.addProject("old", "Old", { archived: true });
    fake.addTicket("old", { title: "Left behind", state: "Ready" });
    fake.addTicket("old", { title: "Still going", state: "In Progress" });
    renderApp(fake);
    const d = await dashboard();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
    expect(within(d).queryByText("Still going")).toBeNull();
  });

  it("include Stage Tickets and blocked Ready Tickets, and follow a change live", async () => {
    const fake = world();
    const spec = fake.addSpec("acme", { title: "Dark mode" });
    fake.addTicket("acme", { title: "Grilling: Dark mode", state: "In Progress", spec: { id: spec.id, title: spec.title }, labels: ["stage:grilling"] });
    fake.addTicket("acme", { title: "Waits on login", state: "Ready", blocked: true });
    renderApp(fake);
    const d = await dashboard();
    expect(count(d, "Ready")).toBe("3");
    expect(count(d, "In Progress")).toBe("2");

    act(() => void fake.updateTicket("acme-1", { state: "In Progress" }));
    await within(d).findByText("Login form");
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("3");
  });
});

// busy is world with more work In Progress, out of order: acme-12 with a
// Session posting events, beta-2, and acme-11 a Stage Ticket of the
// Dark mode Requirement (acme-10).
function busy() {
  const fake = world();
  fake.addTicket("beta", { title: "Beta import", state: "In Progress" }); // beta-2
  const spec = fake.addSpec("acme", { id: "acme-10", title: "Dark mode" });
  fake.addTicket("acme", { title: "Spec: Dark mode", state: "In Progress", spec: { id: spec.id, title: spec.title }, labels: ["stage:spec"] }); // acme-11
  const claims = [{ number: 1, session_id: "s-busy", started: fake.now.toISOString(), last_activity: fake.now.toISOString() }];
  fake.addTicket("acme", { title: "Theme toggle", state: "In Progress", claims }); // acme-12
  fake.postSessionEvents("s-busy", [{ time: new Date(fake.now.getTime() - 2 * 60_000).toISOString(), kind: "tool_call", tool: "Read", text: "Read: web/src/App.tsx" }]);
  return fake;
}

const inProgress = (d: HTMLElement) => within(d).getByRole("list", { name: "In Progress" });
const entries = (d: HTMLElement) => within(inProgress(d)).getAllByRole("listitem");

describe("the In Progress list", () => {
  it("lists the In Progress Tickets by Project, then Ticket number", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();
    expect(entries(d).map((e) => within(e).getByText(/^(acme|beta)-\d+$/).textContent)).toEqual(["acme-2", "acme-11", "acme-12", "beta-2"]);
  });

  it("shows each entry's ID, title, Project and latest event line", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();
    const [quiet, , theme, beta] = entries(d);
    expect(quiet).toHaveTextContent(/^acme-2Sign-up pageAcme$/);
    expect(theme).toHaveTextContent(/^acme-12Theme toggleAcmeRead: web\/src\/App.tsx · 2m ago$/);
    expect(beta).toHaveTextContent(/^beta-2Beta importBeta$/);
  });

  it("shows a Stage Ticket's Requirement and Stage", async () => {
    const fake = busy();
    renderApp(fake);
    const stage = entries(await dashboard())[1];
    expect(within(stage).getByText("Requirement: Dark mode · Spec")).toBeInTheDocument();
  });

  it("leaves out the Project when scoped to one", async () => {
    const fake = busy();
    renderApp(fake, "/?view=focus&project=acme");
    const d = await dashboard();
    expect(entries(d)[0]).toHaveTextContent(/^acme-2Sign-up page$/);
    expect(entries(d)).toHaveLength(3);
  });

  it("follows the live feed: a new latest event, and a Ticket leaving In Progress", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();
    act(() => fake.postSessionEvents("s-busy", [{ time: fake.now.toISOString(), kind: "text", text: "Writing the test first." }]));
    expect(await within(d).findByText("Writing the test first. · just now")).toBeInTheDocument();

    act(() => void fake.updateTicket("beta-2", { state: "In Review" }));
    await waitFor(() => expect(count(d, "In Progress")).toBe("3"));
    expect(within(d).queryByText("Beta import")).toBeNull();
  });

  it("says Nothing in progress when it's empty, with the blocks still showing", async () => {
    const fake = world();
    renderApp(fake, "/?view=focus&project=beta");
    const d = await dashboard();
    expect(within(d).getByText("Nothing in progress")).toBeInTheDocument();
    expect(within(d).queryByRole("listitem")).toBeNull();
    expect(count(d, "Ready")).toBe("1");
    expect(count(d, "In Progress")).toBe("0");
  });

  it("opens an entry's Ticket in the Inbox, keeping the Project filter", async () => {
    const fake = busy();
    renderApp(fake, "/?view=focus&project=acme");
    await userEvent.click(within(entries(await dashboard())[0]).getByRole("link"));
    expect(window.location.search).toBe("?view=inbox&project=acme&ticket=acme-2");
    expect(await screen.findByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
  });

  it("opens a Stage Ticket in its Requirement", async () => {
    const fake = busy();
    renderApp(fake);
    await userEvent.click(within(entries(await dashboard())[1]).getByRole("link"));
    expect(window.location.search).toBe("?view=inbox&requirement=acme-10&ticket=acme-11");
  });
});

// pulsing is whether el shows it is pulsing.
const pulsing = (el: HTMLElement) => el.getAttribute("data-pulsing") === "true";
const entry = (d: HTMLElement, id: string) => within(inProgress(d)).getByText(id).closest("li") as HTMLElement;

describe("pulses", () => {
  it("marks neither a block nor an entry on first show", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();
    expect(pulsing(block(d, "Ready"))).toBe(false);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
    expect(entries(d).filter(pulsing)).toEqual([]);
  });

  it("marks a block when its count changes, and not the other", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();

    act(() => void fake.updateTicket("beta-1", { state: "Done" }));
    await waitFor(() => expect(count(d, "Ready")).toBe("1"));
    expect(pulsing(block(d, "Ready"))).toBe(true);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
  });

  it("marks only the entry whose Ticket a notice names, and no block when a refetch leaves the counts the same", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();

    act(() => fake.postSessionEvents("s-busy", [{ time: fake.now.toISOString(), kind: "text", text: "Writing the test first." }]));
    await within(d).findByText("Writing the test first. · just now");
    expect(entries(d).filter(pulsing)).toEqual([entry(d, "acme-12")]);
    expect(pulsing(block(d, "Ready"))).toBe(false);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
  });

  it("marks an entry on a notice that changes nothing it shows", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();

    act(() => fake.emit({ type: "ticket.changed", project: "beta", id: "beta-2" }));
    await waitFor(() => expect(pulsing(entry(d, "beta-2"))).toBe(true));
    expect(entries(d).filter(pulsing)).toEqual([entry(d, "beta-2")]);
  });

  it("marks an entry that joins the list", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();

    act(() => void fake.updateTicket("acme-1", { state: "In Progress" }));
    await within(d).findByText("Login form");
    expect(entries(d).filter(pulsing)).toEqual([entry(d, "acme-1")]);
    expect(pulsing(block(d, "In Progress"))).toBe(true);
  });

  it("marks the first entry to join an empty list", async () => {
    const fake = world();
    renderApp(fake, "/?view=focus&project=beta");
    const d = await dashboard();

    act(() => void fake.updateTicket("beta-1", { state: "In Progress" }));
    await within(d).findByText("Beta export");
    expect(pulsing(entry(d, "beta-1"))).toBe(true);
  });
});

// ---- The CI Running block (code-central-277) ----

// ciWorld is world with CI Tracking and Push on in Acme, and acme-4 In
// Review with abc123 pushed. Beta keeps CI Tracking off.
async function ciWorld() {
  const fake = world();
  await fake.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
  await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"] });
  fake.addTicket("acme", { title: "Theme toggle", state: "In Review" }); // acme-4
  await fake.reportPushedCommit("acme", "acme-4", "abc123");
  return fake;
}

// ciRun is a run of workflow on acme-4's abc123, started minutes ago and still going.
function ciRun(fake: FakeApi, id: number, workflow: string, more: Partial<CIRunView> = {}): Omit<CIRunView, "ignored"> {
  return {
    id,
    attempt: 1,
    workflow,
    name: workflow,
    branch: "acme-4",
    commit: "abc123",
    status: "in_progress",
    started: new Date(fake.now.getTime() - id * 60_000).toISOString(),
    url: `https://github.com/acme/web/actions/runs/${id}`,
    jobs: [],
    ticket: "acme-4",
    ...more,
  };
}

const ciBlock = (d: HTMLElement) => block(d, "CI Running");

describe("the CI Running block", () => {
  it("counts the CI Runs that count towards a Ticket's CI result, after Ready and In Progress", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [
      ciRun(fake, 1, "ci.yml"),
      ciRun(fake, 2, "lint.yml", { status: "queued" }),
      ciRun(fake, 3, "e2e.yml", { status: "completed", conclusion: "success", finished: fake.now.toISOString() }),
      ciRun(fake, 4, "nightly.yml"),
    ]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(count(d, "CI Running")).toBe("2"));
    expect(within(ciBlock(d)).getByText("CI Running")).toBeInTheDocument();
    const blocks = within(d).getAllByRole("group").map((g) => g.getAttribute("aria-label"));
    expect(blocks).toEqual(["Ready", "In Progress", "CI Running"]);
  });

  it("reads 0 with nothing running", async () => {
    const fake = await ciWorld();
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));
  });

  it("isn't there, and reads nothing, without CI Tracking in scope", async () => {
    const fake = world();
    renderApp(fake);
    const d = await dashboard();
    expect(within(d).getAllByRole("group").map((g) => g.getAttribute("aria-label"))).toEqual(["Ready", "In Progress"]);
    expect(fake.calls).not.toContain("ciOverview");
  });

  it("honours the Project filter: not there in a Project with CI Tracking off", async () => {
    const fake = await ciWorld();
    renderApp(fake, "/?view=focus&project=beta");
    const d = await dashboard();
    expect(within(d).queryByRole("group", { name: "CI Running" })).toBeNull();
    expect(fake.calls).not.toContain("ciOverview");
  });

  it("isn't there when the only Project with CI Tracking on is Archived", async () => {
    const fake = await ciWorld();
    fake.addProject("old", "Old");
    await fake.setCITracking("old", { tracking: true, provider: "github_actions", repository: "old/web", ignored_workflows: [] });
    await fake.archiveProject("old");
    await fake.setCITracking("acme", { tracking: false, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    renderApp(fake);
    const d = await dashboard();
    expect(within(d).queryByRole("group", { name: "CI Running" })).toBeNull();
    expect(fake.calls).not.toContain("ciOverview");
  });

  it("reads the running runs in Focus Mode's Project scope", async () => {
    const fake = await ciWorld();
    const read = vi.spyOn(fake, "ciOverview");
    renderApp(fake, "/?view=focus&project=acme");
    await waitFor(() => expect(read).toHaveBeenCalledWith({ project: "acme", running: true }));
  });

  it("opens the CI view in Focus Mode's scope, with Back to Focus Mode", async () => {
    const fake = await ciWorld();
    renderApp(fake, "/?view=focus&project=acme");
    const d = await dashboard();
    // The block is drawn afresh when its first read arrives.
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));
    const link = within(ciBlock(d)).getByRole("link");
    expect(link).toHaveAttribute("href", "?view=ci&project=acme&back=view%3Dfocus%26project%3Dacme");

    await userEvent.click(link);
    expect(window.location.search).toBe("?view=ci&project=acme&back=view%3Dfocus%26project%3Dacme");
    await userEvent.click(await screen.findByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus&project=acme");
  });

  it("opens the CI view on All Projects from All Projects", async () => {
    const fake = await ciWorld();
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));
    expect(within(ciBlock(d)).getByRole("link")).toHaveAttribute("href", "?view=ci&back=view%3Dfocus");
  });

  it("follows the live feed: runs changing, and a newer pushed commit", async () => {
    const fake = await ciWorld();
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));

    await act(() => fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml"), ciRun(fake, 2, "lint.yml")]));
    await waitFor(() => expect(count(d, "CI Running")).toBe("2"));

    await act(() => fake.reportPushedCommit("acme", "acme-4", "def456"));
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));
  });

  it("rereads the running runs on a ticket.changed notice", async () => {
    const fake = await ciWorld();
    const read = vi.spyOn(fake, "ciOverview");
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));
    const reads = read.mock.calls.length;

    act(() => fake.emit({ type: "ticket.changed", project: "acme", id: "acme-4" }));
    await waitFor(() => expect(read.mock.calls.length).toBeGreaterThan(reads));
  });

  it("pulses when its count changes, but not on first show", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml")]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(count(d, "CI Running")).toBe("1"));
    expect(pulsing(ciBlock(d))).toBe(false);

    await act(() => fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml", { status: "completed", conclusion: "success", finished: fake.now.toISOString() })]));
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));
    expect(pulsing(ciBlock(d))).toBe(true);
    expect(pulsing(block(d, "Ready"))).toBe(false);
  });

  it("shows a failed read's error below the In Progress list, keeps the rest, and retries on the next change", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml")]);
    vi.spyOn(fake, "ciOverview").mockRejectedValueOnce(new ApiError(500, "internal", "The CI Runs can't be read"));
    renderApp(fake);
    const d = await dashboard();
    const alert = await within(d).findByRole("alert");
    expect(alert).toHaveTextContent("The CI Runs can't be read");
    expect(inProgress(d).compareDocumentPosition(alert) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(ciBlock(d)).toBeInTheDocument();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
    expect(entries(d)).toHaveLength(1);

    act(() => fake.emit({ type: "ci.changed", project: "acme" }));
    await waitFor(() => expect(count(d, "CI Running")).toBe("1"));
    expect(within(d).queryByRole("alert")).toBeNull();
  });

  it("leaves the Focus Item to take over when something needs the human, and shows no CI Watcher status", async () => {
    const fake = await ciWorld();
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));
    expect(within(d).queryByText(/CI Watcher|Watching|hasn't reported/)).toBeNull();

    change(fake, [{ reason: "failed", tickets: [{ id: "acme-2" }] }], "acme-2");
    expect(await within(focus()).findByRole("article", { name: "Focus Item" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();
  });
});

// ---- The CI Running list (code-central-278) ----

const ciList = (d: HTMLElement) => within(d).getByRole("list", { name: "CI Running" });
const ciEntries = (d: HTMLElement) => within(ciList(d)).getAllByRole("listitem");
const ciEntry = (d: HTMLElement, name: string) => within(ciList(d)).getByRole("listitem", { name });

describe("the CI Running list", () => {
  it("lists the runs the block counts, newest started first, below the In Progress list", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [
      ciRun(fake, 3, "ci.yml"),
      ciRun(fake, 1, "lint.yml", { status: "queued" }),
      ciRun(fake, 2, "e2e.yml", { status: "completed", conclusion: "success", finished: fake.now.toISOString() }),
      ciRun(fake, 4, "nightly.yml"),
    ]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d).map((e) => e.getAttribute("aria-label"))).toEqual(["lint.yml", "ci.yml"]));
    expect(count(d, "CI Running")).toBe("2");
    expect(inProgress(d).compareDocumentPosition(ciList(d)) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("reads No CI Runs running when nothing is", async () => {
    const fake = await ciWorld();
    renderApp(fake, "/?view=focus&project=acme");
    const d = await dashboard();
    await waitFor(() => expect(count(d, "CI Running")).toBe("0"));
    expect(within(d).getByText("No CI Runs running")).toBeInTheDocument();
    expect(within(d).queryByRole("list", { name: "CI Running" })).toBeNull();
  });

  it("isn't there without CI Tracking in scope", async () => {
    renderApp(world());
    const d = await dashboard();
    expect(within(d).queryByRole("list", { name: "CI Running" })).toBeNull();
    expect(within(d).queryByText("No CI Runs running")).toBeNull();
  });

  it("shows each entry's workflow, ID, Project, branch, short commit and how long it has been running", async () => {
    const fake = await ciWorld();
    await fake.reportPushedCommit("acme", "acme-4", "abc1234def");
    await fake.reportCIRuns("acme", [ciRun(fake, 3, "ci.yml", { name: "CI", commit: "abc1234def" }), ciRun(fake, 2, "lint.yml", { name: "", commit: "abc1234def" })]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(2));
    expect(ciEntry(d, "CI")).toHaveTextContent(/^acme-4CIAcmeacme-4 · abc1234 · running 3m 0sView on GitHub$/);
    expect(ciEntry(d, "lint.yml")).toHaveTextContent(/^acme-4lint.ymlAcmeacme-4 · abc1234 · running 2m 0sView on GitHub$/);
  });

  it("leaves out the Project when scoped to one", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 3, "ci.yml", { name: "CI" })]);
    renderApp(fake, "/?view=focus&project=acme");
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(1));
    expect(ciEntry(d, "CI")).toHaveTextContent(/^acme-4CIacme-4 · abc123 · running 3m 0sView on GitHub$/);
  });

  it("opens an entry's Ticket in the Inbox, keeping the Project filter", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml", { name: "CI" })]);
    renderApp(fake, "/?view=focus&project=acme");
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(1));
    const link = within(ciEntry(d, "CI")).getByRole("link", { name: /acme-4/ });
    expect(link).toHaveAttribute("href", "?view=inbox&project=acme&ticket=acme-4");

    await userEvent.click(link);
    expect(window.location.search).toBe("?view=inbox&project=acme&ticket=acme-4");
    expect(await screen.findByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
  });

  it("opens a Stage Ticket's run in its Requirement", async () => {
    const fake = await ciWorld();
    const spec = fake.addSpec("acme", { id: "acme-10", title: "Dark mode" });
    fake.addTicket("acme", { title: "Spec: Dark mode", state: "In Review", spec: { id: spec.id, title: spec.title }, labels: ["stage:spec"] }); // acme-11
    await fake.reportPushedCommit("acme", "acme-11", "fed987");
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml", { name: "CI", branch: "acme-11", commit: "fed987", ticket: "acme-11" })]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(1));
    await userEvent.click(within(ciEntry(d, "CI")).getByRole("link", { name: /acme-11/ }));
    expect(window.location.search).toBe("?view=inbox&requirement=acme-10&ticket=acme-11");
  });

  it("carries the run's GitHub link beside the Ticket link, not inside it", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml", { name: "CI" })]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(1));
    const github = within(ciEntry(d, "CI")).getByRole("link", { name: "View on GitHub" });
    expect(github).toHaveAttribute("href", "https://github.com/acme/web/actions/runs/1");
    expect(github).toHaveAttribute("target", "_blank");
    expect(github.parentElement?.closest("a")).toBeNull();
    expect(within(ciEntry(d, "CI")).getAllByRole("link")).toHaveLength(2);
  });

  it("follows the live feed: a run joining, finishing, and a newer pushed commit", async () => {
    const fake = await ciWorld();
    renderApp(fake);
    const d = await dashboard();
    await within(d).findByText("No CI Runs running");

    await act(() => fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml"), ciRun(fake, 2, "lint.yml")]));
    await waitFor(() => expect(ciEntries(d).map((e) => e.getAttribute("aria-label"))).toEqual(["ci.yml", "lint.yml"]));

    await act(() => fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml", { status: "completed", conclusion: "failure", finished: fake.now.toISOString() })]));
    await waitFor(() => expect(ciEntries(d).map((e) => e.getAttribute("aria-label"))).toEqual(["lint.yml"]));

    await act(() => fake.reportPushedCommit("acme", "acme-4", "def456"));
    expect(await within(d).findByText("No CI Runs running")).toBeInTheDocument();
  });

  it("shows a failed read's error in its place", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml")]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(1));

    vi.spyOn(fake, "ciOverview").mockRejectedValueOnce(new ApiError(500, "internal", "The CI Runs can't be read"));
    act(() => fake.emit({ type: "ci.changed", project: "acme" }));
    expect(await within(d).findByRole("alert")).toHaveTextContent("The CI Runs can't be read");
    expect(within(d).queryByRole("list", { name: "CI Running" })).toBeNull();
    expect(entries(d)).toHaveLength(1);

    act(() => fake.emit({ type: "ci.changed", project: "acme" }));
    await waitFor(() => expect(ciEntries(d)).toHaveLength(1));
    expect(within(d).queryByRole("alert")).toBeNull();
  });
});

describe("CI Running pulses", () => {
  it("marks no entry on first show", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml"), ciRun(fake, 2, "lint.yml")]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(2));
    expect(ciEntries(d).filter(pulsing)).toEqual([]);
  });

  it("marks an entry when its run's status changes", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml", { status: "queued" }), ciRun(fake, 2, "lint.yml")]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(2));

    await act(() => fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml")]));
    await waitFor(() => expect(ciEntries(d).filter(pulsing)).toEqual([ciEntry(d, "ci.yml")]));
  });

  it("marks a re-run's next attempt as the same entry", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml"), ciRun(fake, 2, "lint.yml")]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(2));
    const before = ciEntry(d, "ci.yml");

    await act(() => fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml", { attempt: 2 })]));
    await waitFor(() => expect(ciEntries(d).filter(pulsing)).toEqual([ciEntry(d, "ci.yml")]));
    expect(ciEntry(d, "ci.yml")).toBe(before);
    expect(ciEntries(d)).toHaveLength(2);
  });

  it("marks an entry that joins after the dashboard first shows", async () => {
    const fake = await ciWorld();
    await fake.reportCIRuns("acme", [ciRun(fake, 2, "lint.yml")]);
    renderApp(fake);
    const d = await dashboard();
    await waitFor(() => expect(ciEntries(d)).toHaveLength(1));

    await act(() => fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml")]));
    await waitFor(() => expect(ciEntries(d)).toHaveLength(2));
    expect(ciEntries(d).filter(pulsing)).toEqual([ciEntry(d, "ci.yml")]);
  });

  it("marks the first entry to join an empty list", async () => {
    const fake = await ciWorld();
    renderApp(fake);
    const d = await dashboard();
    await within(d).findByText("No CI Runs running");

    await act(() => fake.reportCIRuns("acme", [ciRun(fake, 1, "ci.yml")]));
    await waitFor(() => expect(ciEntries(d)).toHaveLength(1));
    expect(pulsing(ciEntry(d, "ci.yml"))).toBe(true);
  });
});
