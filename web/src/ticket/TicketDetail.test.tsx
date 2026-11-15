// The Ticket detail on a WASM Core world: what it shows below the action
// panel, its merged history, edits' "show previous" and move, and its
// header and loading and error states.
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { scenario, type Scenario } from "../api/scenario";
import type { CIRunView, TicketView } from "../api/types";
import { shortOf } from "../inbox/ShortId";
import { renderApp, renderLive } from "../worldTesting";
import { TicketDetail } from "./TicketDetail";

const MIN = 60_000;

async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  return { w, s };
}

// seeded is "Fix login" In Progress under the Spec "Accounts", reached over
// the previous six hours: made as a Draft and approved, edited by an agent
// and by the human, claimed in session s-old and released, then claimed in
// session s-new, with a comment from the human.
async function seeded() {
  const { w, s } = await acme();
  const spec = await s.spec("acme", { title: "Accounts" });
  const id = await s.ticket("acme", { title: "Login bug", spec, state: "Draft", labels: ["auth", "bug"], description: "Login is broken." });
  await w.api.act("acme", id, "approve", {});
  w.advance(100 * MIN);
  await s.agent(`${s.ticketPath("acme", id)}/edit_ticket`, { description: "Login fails." });
  await s.claim("acme", id, "s-old");
  w.advance(60 * MIN);
  await w.api.act("acme", id, "release", {});
  w.advance(180 * MIN);
  await s.claim("acme", id, "s-new");
  await s.modelChoice("acme", id, { model: "opus", effort: "high" });
  w.advance(30 * MIN);
  await s.human(`${s.ticketPath("acme", id)}/comment`, { body: "Check the cookie expiry\nand the refresh path" });
  w.advance(20 * MIN);
  await w.api.act("acme", id, "edit_ticket", { title: "Fix login", description: "Login fails after the session expires." });
  w.advance(10 * MIN);
  return { w, s, id, spec };
}

// claimLine is a Claim row's own line, without its Live section.
const claimLine = (row: HTMLElement) => row.querySelector(".claim-line")!;

function renderDetail(w: World, t: TicketView) {
  renderLive(w, <TicketDetail project="acme" ticket={t} now={w.now()} />);
  return screen.getByRole("article", { name: "Ticket details" });
}

const detailOf = async (w: World, id: string) => renderDetail(w, await w.api.getTicket("acme", id));

describe("the Ticket detail", () => {
  it("shows the Spec, Labels, description and Claim activity", async () => {
    const { w, id, spec } = await seeded();
    const d = await detailOf(w, id);
    // The Spec by its Short ID, with its title on hover.
    expect(within(d).getByText(shortOf(spec))).toHaveAttribute("title", `${spec} Accounts`);
    expect(within(d).getByText("auth")).toBeInTheDocument();
    expect(within(d).getByText("bug")).toBeInTheDocument();
    expect(d).toHaveTextContent("Login fails after the session expires.");

    const claims = within(d).getByRole("list", { name: "Claims" });
    const rows = within(claims).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Claim 2");
    expect(rows[0]).toHaveTextContent("started 1h ago");
    expect(rows[0]).toHaveTextContent("last activity 1h ago");
    expect(rows[0]).toHaveTextContent("session s-new");
    expect(rows[1]).toHaveTextContent("Claim 1");
    expect(rows[1]).toHaveTextContent("ended by release 4h ago");
  });

  it("shows nothing more on a Claim row with no Session Events", async () => {
    const { w, id } = await seeded();
    const rows = within(within(await detailOf(w, id)).getByRole("list", { name: "Claims" })).getAllByRole("listitem");
    expect(rows.map((r) => claimLine(r).textContent)).toEqual([
      "Claim 2 · started 1h ago · last activity 1h ago · model opus · effort high · session s-new",
      "Claim 1 · started 5h ago · last activity 5h ago · ended by release 4h ago · model Claude Code's default · effort Claude Code's default · session s-old",
    ]);
  });

  it("shows the model and effort each Claim's latest turn ran with, or Claude Code's default", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Tune it" });
    await s.claim("acme", id, "s-1");
    await s.modelChoice("acme", id, { effort: "low" });
    await w.api.act("acme", id, "release", {});
    await s.claim("acme", id, "s-2");
    await s.modelChoice("acme", id, { model: "claude-opus-5-5" });
    const rows = within(within(await detailOf(w, id)).getByRole("list", { name: "Claims" })).getAllByRole("listitem");
    expect(claimLine(rows[0])).toHaveTextContent("model claude-opus-5-5 · effort Claude Code's default");
    expect(claimLine(rows[1])).toHaveTextContent("model Claude Code's default · effort low");
  });

  it("adds a Claim's latest Session Event and how long ago it was after its last activity", async () => {
    const { w, s, id } = await seeded();
    const at = (minutesAgo: number) => new Date(w.now().getTime() - minutesAgo * MIN).toISOString();
    await s.postSessionEvents("s-new", [
      { time: at(4), kind: "tool_call", tool: "Bash", tool_use_id: "tu-1", text: "Bash: go test ./..." },
      { time: at(3), kind: "tool_result", tool_use_id: "tu-1", text: "ok" },
    ]);
    const rows = within(within(await detailOf(w, id)).getByRole("list", { name: "Claims" })).getAllByRole("listitem");
    expect(claimLine(rows[0])).toHaveTextContent(/^Claim 2 · started 1h ago · last activity just now · Bash result · 3m ago · model opus · effort high · session s-new$/);
    expect(within(rows[0]).getByText("Bash result · 3m ago")).toHaveClass("latest-event");
    expect(rows[1]).not.toHaveTextContent("Bash");
  });

  it("renders the description as Markdown", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Fix login", description: "## Steps\n\n- [x] repro" });
    const d = await detailOf(w, id);
    expect(within(d).getByRole("heading", { level: 2, name: "Steps" })).toBeInTheDocument();
    expect(within(d).getByRole("checkbox")).toBeChecked();
  });

  it("says there is no description when it is empty", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Empty", description: "" });
    expect(await detailOf(w, id)).toHaveTextContent("No description.");
  });

  it("shows each Blocker's State, and marks a cleared one as no longer blocking", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme");
    // A Spec-landed Blocker clears once the Factory merged it into the Spec Branch.
    const api = await s.ticket("acme", { title: "Login API", spec, state: "Accepted" });
    await s.specMerged("acme", api);
    const sessions = await s.ticket("acme", { title: "Sessions", spec, state: "Accepted" });
    const cookies = await s.ticket("acme", { title: "Cookies", spec });
    const id = await s.ticket("acme", { title: "Login page", spec });
    for (const b of [api, sessions, cookies]) await s.addBlocker("acme", id, b);
    expect(await detailOf(w, id)).toHaveTextContent(
      `Blocked by ${shortOf(api)} (Accepted · no longer blocking), ${shortOf(sessions)} (Accepted), ${shortOf(cookies)} (Ready)`,
    );
    // Each Blocker's title is on hover.
    expect(screen.getByText(shortOf(sessions))).toHaveAttribute("title", `${sessions} Sessions`);
  });

  it("keeps a collapsed history of moves, comments and edits, newest first", async () => {
    const { w, id } = await seeded();
    const d = await detailOf(w, id);
    const history = within(d).getByText(/^History/).closest("details")!;
    expect(history).not.toHaveAttribute("open");
    expect(within(history).getByText(/^History/)).toHaveTextContent("History (4 moves, 1 comment, 2 edits)");

    const items = within(history).getAllByRole("listitem", { hidden: true });
    expect(items.map((li) => li.textContent?.replace(/show previous|hide previous/, "").trim())).toEqual([
      "10m ago title and description edited by human",
      "30m ago human: Check the cookie expiry",
      "1h ago Ready → In Progress by agent",
      "4h ago In Progress → Ready by human",
      "5h ago Ready → In Progress by agent",
      "5h ago description edited by agent",
      "6h ago Draft → Ready by human",
    ]);
  });

  it("shows an edit's previous text on demand, with no restore action", async () => {
    const { w, id } = await seeded();
    const d = await detailOf(w, id);
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
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "# Login", description: "**Login** is broken." });
    await w.api.act("acme", id, "edit_ticket", { title: "Fix login", description: "Fixed." });
    const d = await detailOf(w, id);
    await userEvent.click(within(d).getByText(/^History/));
    await userEvent.click(within(d).getByRole("button", { name: "show previous" }));
    expect(within(d).getByText("Login").tagName).toBe("STRONG");
    expect(within(d).queryByRole("heading")).toBeNull();
    expect(d).toHaveTextContent("Title: # Login");
  });

  it("offers move to every State but the current one, Needs Answer and Failed", async () => {
    const { w, id } = await seeded();
    const d = await detailOf(w, id);
    const select = within(d).getByRole("combobox", { name: "Move to" });
    const options = within(select).getAllByRole("option").map((o) => o.textContent);
    expect(options).toEqual(["Draft", "Ready", "Answered", "In Review", "Accepted", "Done", "Cancelled"]);

    await userEvent.selectOptions(select, "Ready");
    await userEvent.click(within(d).getByRole("button", { name: "Move" }));
    await within(d).findByRole("button", { name: "Move" });
    expect((await w.api.getTicket("acme", id)).state).toBe("Ready");
  });

  it("shows a rejected move's message", async () => {
    const { w, id } = await seeded();
    const d = await detailOf(w, id);
    // The Ticket has moved on since the detail was rendered.
    await w.api.act("acme", id, "release", {});
    await userEvent.selectOptions(within(d).getByRole("combobox", { name: "Move to" }), "Ready");
    await userEvent.click(within(d).getByRole("button", { name: "Move" }));
    expect(await within(d).findByRole("alert")).toHaveTextContent(`${id} is already Ready`);
  });
});

describe("a Prototype's Ticket detail", () => {
  async function prototype(tag?: string) {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Prototype: the theme picker", state: "Needs Answer", labels: ["prototype"] });
    if (tag !== undefined) await w.api.setTags("acme", id, { prototype: tag });
    return { w, t: await w.api.getTicket("acme", id) };
  }
  const line = async (tag?: string) => {
    const { w, t } = await prototype(tag);
    renderLive(w, <TicketDetail project="acme" ticket={t} now={w.now()} />);
    return screen.queryByText(/^Prototype at/);
  };

  it("shows where the Prototype is from its prototype Tag", async () => {
    const l = await line("prototype/acme-3@1a2b3c4 /home/me/code/app.worktrees/acme-3");
    expect(l).toHaveTextContent("Prototype at /home/me/code/app.worktrees/acme-3 on prototype/acme-3 (1a2b3c4)");
    expect(within(l!).getByText("/home/me/code/app.worktrees/acme-3").tagName).toBe("CODE");
    expect(within(l!).getByText("prototype/acme-3").tagName).toBe("CODE");
    expect(within(l!).getByText("1a2b3c4").tagName).toBe("CODE");
  });

  it("keeps the spaces in the worktree's path", async () => {
    expect(await line("prototype/acme-3@1a2b3c4 /Users/me/My Code/app.worktrees/acme-3")).toHaveTextContent(
      "Prototype at /Users/me/My Code/app.worktrees/acme-3 on prototype/acme-3 (1a2b3c4)",
    );
  });

  it.each(["just some text", "prototype/acme-3 /home/me/app", "prototype/acme-3@1a2b3c4", "@1a2b3c4 /home/me/app"])(
    "shows a Tag that doesn't parse, %j, as it is",
    async (raw) => {
      expect(await line(raw)).toHaveTextContent(`Prototype at ${raw}`);
    },
  );

  it("shows no line without the Tag", async () => {
    expect(await line()).toBeNull();
  });
});

// A Ticket's CI section (code-central-256).
describe("the Ticket detail's CI section", () => {
  const run = (id: number, workflow: string, commit: string, more: Partial<Omit<CIRunView, "ignored">> = {}): Omit<CIRunView, "ignored"> => ({
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
    ...more,
  });

  // pushed is an Accepted Ticket whose commits were pushed in order, each
  // with the CI Runs the CI Watcher reported for it.
  async function pushed(pushes: { commit: string; runs: Omit<CIRunView, "ignored">[] }[], ignoredWorkflows: string[] = []) {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Pushed" });
    await s.accepted("acme", id, { branch: id, commit: pushes[0].commit });
    await s.trackCI("acme");
    if (ignoredWorkflows.length) {
      await s.human(`${s.projectPath("acme")}/set_ci_tracking`, { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ignoredWorkflows });
    }
    for (const p of pushes) {
      await s.reportPushedCommit("acme", id, p.commit);
      if (p.runs.length) await s.reportCIRuns("acme", p.runs.map((r) => ({ ...r, ticket: id })));
    }
    return { w, s, id };
  }

  it("isn't there for a Ticket never pushed", async () => {
    const { w, id } = await seeded();
    expect(within(await detailOf(w, id)).queryByRole("region", { name: "CI" })).toBeNull();
  });

  it("isn't there for a pushed Ticket with no CI Runs reported yet", async () => {
    const { w, id } = await pushed([{ commit: "def4567890", runs: [] }]);
    expect(within(await detailOf(w, id)).queryByRole("region", { name: "CI" })).toBeNull();
  });

  it("shows the CI result and each workflow with its status, jobs and link", async () => {
    const { w, id } = await pushed([
      {
        commit: "def4567890",
        runs: [
          run(1, "ci.yml", "def4567890", {
            conclusion: "failure",
            jobs: [
              { name: "test", conclusion: "failure", url: "https://github.com/acme/web/actions/runs/1/job/1" },
              { name: "lint", conclusion: "success", url: "https://github.com/acme/web/actions/runs/1/job/2" },
            ],
          }),
          run(2, "deploy.yml", "def4567890", { status: "in_progress", conclusion: undefined, jobs: [{ name: "ship", url: "https://github.com/acme/web/actions/runs/2/job/1" }] }),
        ],
      },
    ]);
    const ci = within(await detailOf(w, id)).getByRole("region", { name: "CI" });
    // Core's result is running while any counted run is unfinished.
    expect(within(ci).getByRole("heading")).toHaveTextContent("CI running def4567");

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

  it("greys an ignored run and marks it ignored", async () => {
    const { w, id } = await pushed(
      [{ commit: "def4567890", runs: [run(1, "ci.yml", "def4567890"), run(2, "nightly.yml", "def4567890", { conclusion: "failure" })] }],
      ["nightly.yml"],
    );
    const ci = within(await detailOf(w, id)).getByRole("region", { name: "CI" });
    const ignored = within(ci).getByRole("listitem", { name: "Build nightly.yml" });
    expect(ignored).toHaveClass("ignored");
    expect(within(ignored).getByText("ignored")).toBeInTheDocument();
    const counted = within(ci).getByRole("listitem", { name: "Build ci.yml" });
    expect(counted).not.toHaveClass("ignored");
    expect(within(counted).queryByText("ignored")).toBeNull();
  });

  it("keeps an earlier push's runs below, folded away", async () => {
    const { w, id } = await pushed([
      { commit: "abc1234567", runs: [run(1, "ci.yml", "abc1234567", { conclusion: "failure" })] },
      { commit: "def4567890", runs: [run(2, "ci.yml", "def4567890")] },
    ]);
    const ci = within(await detailOf(w, id)).getByRole("region", { name: "CI" });
    const earlier = within(ci).getByText("Earlier pushes (1 run)");
    expect(within(ci).getByRole("heading")).toHaveTextContent("CI passed def4567");
    await userEvent.click(earlier);
    expect(ci).toHaveTextContent("abc1234");
  });
});

// TicketDetail through its props alone: its loading and error states, its
// header, and the parts it shows only once the view has read its Spec.
describe("the Ticket pane's body", () => {
  it("says Loading… until the Ticket is read", async () => {
    const { w } = await acme();
    renderLive(w, <TicketDetail project="acme" now={w.now()} />);
    expect(screen.getByText("Loading…")).toBeInTheDocument();
    expect(screen.queryByRole("article", { name: "Ticket details" })).toBeNull();
  });

  it("shows the read's error", async () => {
    const { w } = await acme();
    renderLive(w, <TicketDetail project="acme" now={w.now()} error="acme-9 not found" />);
    expect(screen.getByRole("alert")).toHaveTextContent("acme-9 not found");
    expect(screen.queryByText("Loading…")).toBeNull();
  });

  it("heads the Ticket with its State chip and Reason badge", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Pick a theme", state: "Needs Answer" });
    renderLive(w, <TicketDetail project="acme" ticket={await w.api.getTicket("acme", id)} now={w.now()} />);
    expect(screen.getByText("Needs Answer", { selector: ".state-chip" })).toHaveClass("st-needs-answer");
    expect(screen.getByText("Questions", { selector: ".reason" })).toHaveClass("reason-needs_answer");
    expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Pick a theme");
  });

  it("has no Reason badge when the Ticket doesn't need the human", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Pick a theme" });
    renderLive(w, <TicketDetail project="acme" ticket={await w.api.getTicket("acme", id)} now={w.now()} />);
    expect(screen.getByText("Ready", { selector: ".state-chip" })).toBeInTheDocument();
    expect(document.querySelector(".reason")).toBeNull();
  });

  it("offers the Auto-accept switch only once its Spec's Labels are given", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Pick a theme" });
    const t = await w.api.getTicket("acme", id);
    const { rerender } = renderLive(w, <TicketDetail project="acme" ticket={t} now={w.now()} />);
    expect(screen.queryByRole("group", { name: "Auto-accept" })).toBeNull();
    rerender(<TicketDetail project="acme" ticket={t} now={w.now()} specLabels={[]} />);
    expect(screen.getByRole("group", { name: "Auto-accept" })).toBeInTheDocument();
  });

  it("opens its Spec through openSpec", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming" });
    const id = await s.ticket("acme", { title: "Pick a theme", spec });
    const openSpec = vi.fn();
    renderLive(w, <TicketDetail project="acme" ticket={await w.api.getTicket("acme", id)} now={w.now()} openSpec={openSpec} />);
    await userEvent.click(screen.getByRole("button", { name: `${spec} Theming` }));
    expect(openSpec).toHaveBeenCalledWith(spec);
  });

  it("opens a new Retro's Ticket through openTicket", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Pick a theme", state: "Done" });
    const openTicket = vi.fn();
    renderLive(w, <TicketDetail project="acme" ticket={await w.api.getTicket("acme", id)} now={w.now()} openTicket={openTicket} />);
    await userEvent.click(screen.getByRole("button", { name: "Retro" }));
    await userEvent.click(within(screen.getByRole("dialog", { name: "Retro" })).getByRole("button", { name: "Start retro" }));
    const [retro] = (await w.api.getTicket("acme", id)).retros ?? [];
    expect(openTicket).toHaveBeenCalledWith(retro.id);
  });

  it("leaves exactly one change feed open when the App shows the Ticket pane", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming" });
    const id = await s.ticket("acme", { title: "Pick a theme", spec });
    renderApp(w, `/?view=inbox&ticket=${id}`);
    await screen.findByRole("article", { name: "Ticket details" });
    await act(async () => {});
    expect(w.subscriptions()).toBe(1);
  });
});
