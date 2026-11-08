// The Ticket detail against FakeApi: what it shows below the action
// panel, its merged history, edits' "show previous" and move.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FakeApi } from "../api/fake";
import type { TicketView } from "../api/types";
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
