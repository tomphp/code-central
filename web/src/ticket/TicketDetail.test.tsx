// The Ticket detail against FakeApi: what it shows below the action
// panel, its merged history, edits' "show previous" and move.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FakeApi } from "../api/fake";
import type { CIRunView, TicketView } from "../api/types";
import { TicketDetail } from "./TicketDetail";

const at = (fake: FakeApi, minutesAgo: number) => new Date(fake.now.getTime() - minutesAgo * 60_000).toISOString();

function seeded() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const t = fake.addTicket("acme", {
    title: "Fix login",
    state: "In Progress",
    spec: { id: "acme-7", title: "Accounts" },
    labels: ["auth", "bug"],
    description: "Login fails after the session expires.",
    claims: [
      { number: 1, session_id: "s-old", started: at(fake, 300), last_activity: at(fake, 250), ended: at(fake, 240), ended_by: "release" },
      { number: 2, session_id: "s-new", started: at(fake, 60), last_activity: at(fake, 2) },
    ],
    moves: [
      { from: "Draft", to: "Ready", actor: "agent", time: at(fake, 400) },
      { from: "Ready", to: "In Progress", actor: "agent", time: at(fake, 60), claim: 2 },
    ],
    comments: [{ number: 1, actor: "human", time: at(fake, 30), body: "Check the cookie expiry\nand the refresh path" }],
    edits: [
      { number: 1, actor: "agent", time: at(fake, 200), previous: { description: "Login is broken." } },
      { number: 2, actor: "human", time: at(fake, 10), previous: { title: "Login bug", description: "Login fails." } },
    ],
  });
  return { fake, t };
}

// claimLine is a Claim row's own line, without its Live section.
const claimLine = (row: HTMLElement) => row.querySelector(".claim-line")!;

function renderDetail(fake: FakeApi, t: TicketView) {
  render(<TicketDetail api={fake} project="acme" ticket={t} now={fake.now} />);
  return screen.getByRole("article", { name: "Ticket details" });
}

describe("the Ticket detail", () => {
  it("shows the Spec, Labels, description and Claim activity", () => {
    const { fake, t } = seeded();
    const d = renderDetail(fake, t);
    expect(d).toHaveTextContent("acme-7 Accounts");
    expect(within(d).getByText("auth")).toBeInTheDocument();
    expect(within(d).getByText("bug")).toBeInTheDocument();
    expect(d).toHaveTextContent("Login fails after the session expires.");

    const claims = within(d).getByRole("list", { name: "Claims" });
    const rows = within(claims).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Claim 2");
    expect(rows[0]).toHaveTextContent("started 1h ago");
    expect(rows[0]).toHaveTextContent("last activity 2m ago");
    expect(rows[0]).toHaveTextContent("session s-new");
    expect(rows[1]).toHaveTextContent("Claim 1");
    expect(rows[1]).toHaveTextContent("ended by release 4h ago");
  });

  it("shows nothing more on a Claim row with no Session Events", () => {
    const { fake, t } = seeded();
    const rows = within(within(renderDetail(fake, t)).getByRole("list", { name: "Claims" })).getAllByRole("listitem");
    expect(rows.map((r) => claimLine(r).textContent)).toEqual([
      "Claim 2 · started 1h ago · last activity 2m ago · session s-new",
      "Claim 1 · started 5h ago · last activity 4h ago · ended by release 4h ago · session s-old",
    ]);
  });

  it("adds a Claim's latest Session Event and how long ago it was after its last activity", async () => {
    const { fake, t } = seeded();
    fake.postSessionEvents("s-new", [
      { time: at(fake, 4), kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: go test ./..." },
      { time: at(fake, 3), kind: "tool_result", tool_use_id: "tu-1", text: "ok" },
    ]);
    const rows = within(within(renderDetail(fake, await fake.getTicket("acme", t.id))).getByRole("list", { name: "Claims" })).getAllByRole("listitem");
    expect(claimLine(rows[0])).toHaveTextContent(/^Claim 2 · started 1h ago · last activity just now · Bash result · 3m ago · session s-new$/);
    expect(within(rows[0]).getByText("Bash result · 3m ago")).toHaveClass("latest-event");
    expect(rows[1]).not.toHaveTextContent("Bash");
  });

  it("renders the description as Markdown", () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const t = fake.addTicket("acme", { title: "Fix login", state: "Ready", description: "## Steps\n\n- [x] repro" });
    const d = renderDetail(fake, t);
    expect(within(d).getByRole("heading", { level: 2, name: "Steps" })).toBeInTheDocument();
    expect(within(d).getByRole("checkbox")).toBeChecked();
  });

  it("says there is no description when it is empty", () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const t = fake.addTicket("acme", { title: "Empty", state: "Ready", description: "" });
    expect(renderDetail(fake, t)).toHaveTextContent("No description.");
  });

  it("shows a gated Blocker's CI result beside its State, and marks a cleared one as no longer blocking", () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const t = fake.addTicket("acme", {
      title: "Login page",
      state: "Ready",
      blocked_by: ["acme-3", "acme-5", "acme-6"],
      blockers: [
        { id: "acme-3", title: "Login API", state: "Accepted", ci_result: "running", cleared: true },
        { id: "acme-5", title: "Sessions", state: "Accepted", ci_result: "failed", cleared: false },
        { id: "acme-6", title: "Cookies", state: "Ready", cleared: false },
      ],
    });
    expect(renderDetail(fake, t)).toHaveTextContent(
      "Blocked by acme-3 Login API (Accepted · CI running · no longer blocking), acme-5 Sessions (Accepted · CI failed), acme-6 Cookies (Ready)",
    );
  });

  it("keeps a collapsed history of moves, comments and edits, newest first", () => {
    const { fake, t } = seeded();
    const d = renderDetail(fake, t);
    const history = within(d).getByText(/^History/).closest("details")!;
    expect(history).not.toHaveAttribute("open");
    expect(within(history).getByText(/^History/)).toHaveTextContent("History (2 moves, 1 comment, 2 edits)");

    const items = within(history).getAllByRole("listitem", { hidden: true });
    expect(items.map((li) => li.textContent?.replace(/show previous|hide previous/, "").trim())).toEqual([
      "10m ago title and description edited by human",
      "30m ago human: Check the cookie expiry",
      "1h ago Ready → In Progress by agent",
      "3h ago description edited by agent",
      "6h ago Draft → Ready by agent",
    ]);
  });

  it("shows an edit's previous text on demand, with no restore action", async () => {
    const { fake, t } = seeded();
    const d = renderDetail(fake, t);
    await userEvent.click(within(d).getByText(/^History/));
    const edit = within(d).getByText("description edited by agent").closest("li")!;
    expect(within(edit).queryByText("Login is broken.")).toBeNull();

    await userEvent.click(within(edit).getByRole("button", { name: "show previous" }));
    expect(within(edit).getByText("Login is broken.")).toBeVisible();
    expect(within(d).queryByRole("button", { name: /restore/i })).toBeNull();

    await userEvent.click(within(edit).getByRole("button", { name: "hide previous" }));
    expect(within(edit).queryByText("Login is broken.")).toBeNull();

    const both = within(d).getByText("title and description edited by human").closest("li")!;
    await userEvent.click(within(both).getByRole("button", { name: "show previous" }));
    expect(both).toHaveTextContent("Title: Login bug");
    expect(both).toHaveTextContent("Description: Login fails.");
  });

  it("renders an edit's previous description as Markdown and its previous title as text", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const t = fake.addTicket("acme", {
      title: "Fix login",
      state: "Ready",
      edits: [{ number: 1, actor: "agent", time: at(fake, 5), previous: { title: "# Login", description: "**Login** is broken." } }],
    });
    const d = renderDetail(fake, t);
    await userEvent.click(within(d).getByText(/^History/));
    await userEvent.click(within(d).getByRole("button", { name: "show previous" }));
    expect(within(d).getByText("Login").tagName).toBe("STRONG");
    expect(within(d).queryByRole("heading")).toBeNull();
    expect(d).toHaveTextContent("Title: # Login");
  });

  it("offers move to every State but the current one, Needs Answer and Failed", async () => {
    const { fake, t } = seeded();
    const d = renderDetail(fake, t);
    const select = within(d).getByRole("combobox", { name: "Move to" });
    const options = within(select).getAllByRole("option").map((o) => o.textContent);
    expect(options).toEqual(["Draft", "Ready", "Answered", "In Review", "Accepted", "Done", "Cancelled"]);

    await userEvent.selectOptions(select, "Ready");
    await userEvent.click(within(d).getByRole("button", { name: "Move" }));
    expect(fake.calls).toContain("move");
    expect((await fake.getTicket("acme", t.id)).state).toBe("Ready");
  });

  it("shows a rejected move's message", async () => {
    const { fake, t } = seeded();
    const d = renderDetail(fake, t);
    // The fake's Ticket has moved on since the detail was rendered.
    fake.updateTicket(t.id, { state: "Ready" });
    await userEvent.selectOptions(within(d).getByRole("combobox", { name: "Move to" }), "Ready");
    await userEvent.click(within(d).getByRole("button", { name: "Move" }));
    expect(await within(d).findByRole("alert")).toHaveTextContent("can't move acme-1 from Ready to Ready");
  });
});

describe("a Prototype's Ticket detail", () => {
  function prototype(tag?: string) {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    return fake.addTicket("acme", {
      title: "Prototype: the theme picker",
      state: "Needs Answer",
      labels: ["prototype"],
      tags: tag === undefined ? {} : { prototype: tag },
    });
  }
  const line = (t: TicketView) => {
    render(<TicketDetail api={new FakeApi()} project="acme" ticket={t} now={new Date()} />);
    return screen.queryByText(/^Prototype at/);
  };

  it("shows where the Prototype is from its prototype Tag", () => {
    const l = line(prototype("prototype/acme-3@1a2b3c4 /home/me/code/app.worktrees/acme-3"));
    expect(l).toHaveTextContent("Prototype at /home/me/code/app.worktrees/acme-3 on prototype/acme-3 (1a2b3c4)");
    expect(within(l!).getByText("/home/me/code/app.worktrees/acme-3").tagName).toBe("CODE");
    expect(within(l!).getByText("prototype/acme-3").tagName).toBe("CODE");
    expect(within(l!).getByText("1a2b3c4").tagName).toBe("CODE");
  });

  it("keeps the spaces in the worktree's path", () => {
    expect(line(prototype("prototype/acme-3@1a2b3c4 /Users/me/My Code/app.worktrees/acme-3"))).toHaveTextContent(
      "Prototype at /Users/me/My Code/app.worktrees/acme-3 on prototype/acme-3 (1a2b3c4)",
    );
  });

  it.each(["just some text", "prototype/acme-3 /home/me/app", "prototype/acme-3@1a2b3c4", "@1a2b3c4 /home/me/app"])(
    "shows a Tag that doesn't parse, %j, as it is",
    (raw) => {
      expect(line(prototype(raw))).toHaveTextContent(`Prototype at ${raw}`);
    },
  );

  it("shows no line without the Tag", () => {
    expect(line(prototype())).toBeNull();
  });
});

// A Ticket's CI section (code-central-256).
describe("the Ticket detail's CI section", () => {
  const run = (id: number, workflow: string, commit: string, more: Partial<CIRunView> = {}): CIRunView => ({
    id,
    attempt: 1,
    workflow,
    name: `Build ${workflow}`,
    branch: "main",
    commit,
    status: "completed",
    conclusion: "success",
    url: `https://github.com/acme/web/actions/runs/${id}`,
    jobs: [],
    ticket: "acme-9",
    ignored: false,
    ...more,
  });

  function pushed(runs: CIRunView[], ci_result: TicketView["ci_result"]) {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const t = fake.addTicket("acme", {
      title: "Pushed",
      state: "Accepted",
      pushed_commit: { commit: "def4567890", pushed: fake.now.toISOString() },
      ci_result,
      ci_runs: runs,
    });
    return { fake, t };
  }

  it("isn't there for a Ticket never pushed", () => {
    const { fake, t } = seeded();
    expect(within(renderDetail(fake, t)).queryByRole("region", { name: "CI" })).toBeNull();
  });

  it("isn't there for a pushed Ticket with no CI Runs reported yet", () => {
    const { fake, t } = pushed([], "running");
    expect(within(renderDetail(fake, t)).queryByRole("region", { name: "CI" })).toBeNull();
  });

  it("shows the CI result and each workflow with its status, jobs and link", () => {
    const { fake, t } = pushed(
      [
        run(1, "ci.yml", "def4567890", {
          conclusion: "failure",
          jobs: [
            { name: "test", conclusion: "failure", url: "https://github.com/acme/web/actions/runs/1/job/1" },
            { name: "lint", conclusion: "success", url: "https://github.com/acme/web/actions/runs/1/job/2" },
          ],
        }),
        run(2, "deploy.yml", "def4567890", { status: "in_progress", conclusion: undefined, jobs: [{ name: "ship", url: "https://github.com/acme/web/actions/runs/2/job/1" }] }),
      ],
      "failed",
    );
    const ci = within(renderDetail(fake, t)).getByRole("region", { name: "CI" });
    expect(within(ci).getByRole("heading")).toHaveTextContent("CI failed def4567");

    const failed = within(ci).getByRole("listitem", { name: "Build ci.yml" });
    expect(within(failed).getByRole("link", { name: "Build ci.yml" })).toHaveAttribute("href", "https://github.com/acme/web/actions/runs/1");
    expect(failed).toHaveTextContent("ci.yml failed");
    expect(within(failed).getByRole("link", { name: "test" })).toHaveAttribute("href", "https://github.com/acme/web/actions/runs/1/job/1");
    expect(failed).toHaveTextContent("test failed");
    expect(failed).toHaveTextContent("lint passed");

    const running = within(ci).getByRole("listitem", { name: "Build deploy.yml" });
    expect(running).toHaveTextContent("deploy.yml running");
    expect(running).toHaveTextContent("ship running");
  });

  it("greys an ignored run and marks it ignored", () => {
    const { fake, t } = pushed([run(1, "ci.yml", "def4567890"), run(2, "nightly.yml", "def4567890", { conclusion: "failure", ignored: true })], "passed");
    const ci = within(renderDetail(fake, t)).getByRole("region", { name: "CI" });
    const ignored = within(ci).getByRole("listitem", { name: "Build nightly.yml" });
    expect(ignored).toHaveClass("ignored");
    expect(within(ignored).getByText("ignored")).toBeInTheDocument();
    const counted = within(ci).getByRole("listitem", { name: "Build ci.yml" });
    expect(counted).not.toHaveClass("ignored");
    expect(within(counted).queryByText("ignored")).toBeNull();
  });

  it("keeps an earlier push's runs below, folded away", async () => {
    const { fake, t } = pushed([run(2, "ci.yml", "def4567890"), run(1, "ci.yml", "abc1234567", { conclusion: "failure" })], "passed");
    const ci = within(renderDetail(fake, t)).getByRole("region", { name: "CI" });
    const earlier = within(ci).getByText("Earlier pushes (1 run)");
    expect(within(ci).getByRole("heading")).toHaveTextContent("CI passed def4567");
    await userEvent.click(earlier);
    expect(ci).toHaveTextContent("abc1234");
  });
});
