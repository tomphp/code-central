// Requirement tests: the real App against FakeApi. "New requirement", the
// Stage strip, the Requirement pane and the Requirements view.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const params = () => new URLSearchParams(window.location.search);

describe("New requirement", () => {
  it("opens a modal with one text box, submits it and selects the new Requirement", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog", { name: "New requirement" });
    expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "Export a Spec as Markdown{Enter}One file.");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));

    expect(fake.calls).toContain("submitRequirement");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(await within(detail()).findByText("Waiting for a Factory to claim Grilling. Nothing needs you yet.")).toBeInTheDocument();
    expect(within(detail()).getByRole("heading", { name: "Export a Spec as Markdown", level: 2 })).toBeInTheDocument();
    expect(params().get("requirement")).toBe("acme-1");
  });

  it("closes on Cancel without submitting", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(fake.calls).not.toContain("submitRequirement");
  });

  it("asks which Project when there are several and none is chosen, and goes to the chosen one", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addProject("beta", "Beta");
    renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog");
    await userEvent.selectOptions(within(modal).getByRole("combobox", { name: "Project" }), "beta");
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "Beta work");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));

    await waitFor(() => expect(params().get("requirement")).toBe("beta-1"));
  });

  it("submits to the filtered Project without asking", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addProject("beta", "Beta");
    renderApp(fake, "/?view=inbox&project=beta");

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog");
    expect(within(modal).queryByRole("combobox")).toBeNull();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "Beta work");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));
    await waitFor(() => expect(params().get("requirement")).toBe("beta-1"));
  });

  it("won't submit blank text", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog");
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "   ");
    expect(within(modal).getByRole("button", { name: "Submit requirement" })).toBeDisabled();
  });
});

const hoursAgo = (fake: FakeApi, h: number) => new Date(fake.now.getTime() - h * 3_600_000);

// building is a Requirement in Build: every Stage Done, one implementation
// Ticket Done, one In Review (needing the human), one Ready, one Cancelled.
async function building() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const now = fake.now;
  fake.now = hoursAgo(fake, 2);
  const title = "Archive finished Projects";
  const r = await fake.submitRequirement("acme", `${title}\nOnce everything is Done.`); // acme-1, stages acme-2..4
  fake.now = now;
  for (const t of r.stages) fake.updateTicket(t.id, { state: "Done" });
  const spec = { id: r.spec.id, title };
  fake.addTicket("acme", { title: "store: archived flag", state: "Done", spec }); // acme-5
  fake.addTicket("acme", { title: "Archive in the Project menu", state: "In Review", spec }); // acme-6
  fake.addTicket("acme", { title: "CLI: archive-project", state: "Ready", spec }); // acme-7
  fake.addTicket("acme", { title: "Dropped idea", state: "Cancelled", spec }); // acme-8
  fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-6", summary: "Menu item added" }] }]);
  return fake;
}

const pane = () => within(detail()).getByRole("region", { name: "Requirement" });
const segment = (name: RegExp) => within(within(pane()).getByRole("list", { name: "Stages" })).getByRole("button", { name });

describe("the Requirement pane", () => {
  it("shows the header and strip, a queued Stage dashed, while the Requirement waits for a Factory", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.submitRequirement("acme", "Export a Spec as Markdown");
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    await within(detail()).findByRole("region", { name: "Requirement" });
    expect(pane()).toHaveTextContent("acme-1");
    expect(pane()).toHaveTextContent("submitted just now");
    expect(segment(/^Grilling/).closest("li")).toHaveAttribute("data-progress", "queued");
    expect(segment(/^Spec/).closest("li")).toHaveAttribute("data-progress", "waiting");
    expect(segment(/^Build/)).toBeDisabled();
    expect(within(pane()).getByText("Waiting for a Factory to claim Grilling. Nothing needs you yet.")).toBeInTheDocument();
    // With no Ticket needing the human, the Ticket pane opens on the current Stage.
    expect(await within(detail()).findByRole("heading", { name: "Grilling: Export a Spec as Markdown" })).toBeInTheDocument();
  });

  it("opens on the Ticket that needs the human, lists Build's Tickets with their States, and selects a Stage from the strip", async () => {
    const fake = await building();
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    expect(await within(detail()).findByRole("heading", { name: "Archive in the Project menu" })).toBeInTheDocument();
    expect(pane()).toHaveTextContent("submitted 2h ago");
    expect(within(pane()).getByRole("heading", { name: "Archive finished Projects" })).toBeInTheDocument();
    expect(segment(/^Build/)).toHaveTextContent("Build 1/3");
    expect(segment(/^Build/).closest("li")).toHaveAttribute("data-progress", "needs");
    expect(segment(/^Build/).closest("li")).toHaveAttribute("aria-current", "step");
    // acme-7 can be claimed, but something needs the human.
    expect(within(pane()).getByText("Waiting for a Factory to claim a Build Ticket.")).toBeInTheDocument();

    const build = within(pane()).getByRole("list", { name: "Build" });
    const items = within(build).getAllByRole("listitem").map((li) => li.textContent);
    expect(items).toEqual([
      "acme-5 store: archived flag Done",
      "acme-6 Archive in the Project menu In Review",
      "acme-7 CLI: archive-project Ready",
      "acme-8 Dropped idea Cancelled",
    ]);

    await userEvent.click(segment(/^Spec/));
    expect(params().get("ticket")).toBe("acme-3");
    expect(params().get("requirement")).toBe("acme-1");
    expect(await within(detail()).findByRole("heading", { name: "Spec: Archive finished Projects" })).toBeInTheDocument();
    expect(segment(/^Spec/).closest("li")).toHaveAttribute("aria-current", "step");

    await userEvent.click(within(build).getByRole("button", { name: /acme-7/ }));
    expect(await within(detail()).findByRole("heading", { name: "CLI: archive-project" })).toBeInTheDocument();
  });

  it("shows an In Progress Build Ticket's latest Session Event faintly, and nothing on one with none", async () => {
    const fake = await building();
    const spec = { id: "acme-1", title: "Archive finished Projects" };
    const claim = (session_id: string) => [{ number: 1, session_id, started: fake.now.toISOString(), last_activity: fake.now.toISOString() }];
    fake.addTicket("acme", { title: "Archive in the API", state: "In Progress", spec, claims: claim("s-busy") }); // acme-9
    fake.addTicket("acme", { title: "Archive in the store", state: "In Progress", spec, claims: claim("s-quiet") }); // acme-10
    fake.postSessionEvents("s-busy", [{ time: new Date(fake.now.getTime() - 5 * 60_000).toISOString(), kind: "tool_call", tool: "Edit", text: "Edit: core/archive.go" }]);
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    await within(detail()).findByRole("region", { name: "Requirement" });
    const build = within(pane()).getByRole("list", { name: "Build" });
    const items = within(build).getAllByRole("listitem");
    expect(within(items[4]).getByText("Edit: core/archive.go · 5m ago")).toHaveClass("latest-event");
    expect(items.map((li) => li.textContent)).toEqual([
      "acme-5 store: archived flag Done",
      "acme-6 Archive in the Project menu In Review",
      "acme-7 CLI: archive-project Ready",
      "acme-8 Dropped idea Cancelled",
      "acme-9 Archive in the API In ProgressEdit: core/archive.go · 5m ago",
      "acme-10 Archive in the store In Progress",
    ]);
  });

  it("updates an In Progress Build Ticket's latest Session Event on the live feed", async () => {
    const fake = await building();
    const spec = { id: "acme-1", title: "Archive finished Projects" };
    const claims = [{ number: 1, session_id: "s-busy", started: fake.now.toISOString(), last_activity: fake.now.toISOString() }];
    fake.addTicket("acme", { title: "Archive in the API", state: "In Progress", spec, claims }); // acme-9
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    const build = await within(detail()).findByRole("list", { name: "Build" });
    act(() => fake.postSessionEvents("s-busy", [{ time: fake.now.toISOString(), kind: "text", text: "Writing the test first." }]));
    expect(await within(build).findByText("Writing the test first. · just now")).toHaveClass("latest-event");
  });

  it("says when Build waits for a Factory", async () => {
    const fake = await building();
    fake.updateTicket("acme-6", { state: "Done" });
    fake.setNeedsMe([]);
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    expect(await within(detail()).findByText("Waiting for a Factory to claim a Build Ticket. Nothing needs you yet.")).toBeInTheDocument();
    expect(segment(/^Build/)).toHaveTextContent("Build 2/3");
    expect(segment(/^Build/).closest("li")).toHaveAttribute("data-progress", "queued");
  });
});

describe("the Requirements view", () => {
  it("lists every Requirement with its strip, finished ones included, and opens one", async () => {
    const fake = await building(); // acme-1
    const fresh = await fake.submitRequirement("acme", "Export a Spec"); // acme-9
    const done = await fake.submitRequirement("acme", "Render Markdown"); // acme-13
    for (const t of done.stages) fake.updateTicket(t.id, { state: "Done" });
    fake.addTicket("acme", { title: "Plain work", state: "Ready", spec: { id: "acme-20", title: "Not a Requirement" } });
    renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "Requirements 2" }));
    const list = screen.getByRole("main", { name: "Tickets" });
    const ids = () => [...list.querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id);
    await waitFor(() => expect(ids()).toEqual([fresh.spec.id, "acme-1", done.spec.id]));

    const row = (id: string) => list.querySelector<HTMLElement>(`a[data-id="${id}"]`)!;
    expect(within(row("acme-1")).getByRole("list", { name: "Stages" })).toHaveTextContent("Build 1/3");
    expect(row("acme-1")).toHaveTextContent("Build: In Review");
    expect(within(row("acme-1")).getByLabelText("Needs you")).toBeInTheDocument();
    expect(row(fresh.spec.id)).toHaveTextContent("Grilling: waiting for a Factory");
    expect(row(done.spec.id)).toHaveTextContent("Finished");
    expect(params().get("list")).toBe("requirements");

    await userEvent.click(row("acme-1"));
    expect(params().get("requirement")).toBe("acme-1");
    expect(await within(detail()).findByRole("heading", { name: "Archive in the Project menu" })).toBeInTheDocument();
    expect(row("acme-1")).toHaveAttribute("aria-current", "true");
  });
});
