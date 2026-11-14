// New Prototype tests: the real App on a WASM Core world. "New Prototype" on
// each Spec's view (Requirement, Map and plain Spec), and the plain Spec
// view it sits on.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";

function renderApp(w: World, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

// acme is a world with the Project acme, and the scenario builder on it.
async function acme(...more: string[]) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  for (const key of more) await s.project(key, key[0].toUpperCase() + key.slice(1));
  return { w, s };
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const params = () => new URLSearchParams(window.location.search);

// prototype asks for a Prototype from the view named view and returns
// the dialog's submit, once the text is typed.
async function prototype(view: string, text: string) {
  const pane = await within(detail()).findByRole("region", { name: view });
  await userEvent.click(within(pane).getByRole("button", { name: "New Prototype" }));
  const modal = screen.getByRole("dialog", { name: "New Prototype" });
  expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
  expect(within(modal).queryByRole("combobox")).toBeNull();
  const submit = within(modal).getByRole("button", { name: "Submit prototype" });
  expect(submit).toBeDisabled();
  await userEvent.type(within(modal).getByRole("textbox", { name: "Prototype" }), text);
  return submit;
}

// wantPrototype checks the new Prototype is a Ready prototype Ticket on
// spec, and is the selected Ticket.
async function wantPrototype(w: World, spec: string, id: string, title: string) {
  expect(w.calls).toContain("submitPrototype");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(await w.api.getTicket("acme", id)).toMatchObject({ title, state: "Ready", spec: { id: spec }, labels: ["prototype"] });
  expect(params().get("requirement")).toBe(spec);
  expect(params().get("ticket")).toBe(id);
  expect(await within(detail()).findByRole("heading", { name: title, level: 2 })).toBeInTheDocument();
}

describe("New Prototype", () => {
  it("is on the Requirement view, and submits a Prototype on that Requirement", async () => {
    const { w } = await acme("beta");
    const { spec } = await w.api.submitRequirement("acme", "Dark mode");
    renderApp(w, `/?view=inbox&requirement=${spec.id}`);

    await userEvent.click(await prototype("Requirement", "A theme picker{Enter}Would a picker beat a toggle?"));
    const id = (await w.api.findTickets("acme")).find((t) => t.title === "Prototype: A theme picker")!.id;
    await wantPrototype(w, spec.id, id, "Prototype: A theme picker");
    expect((await w.api.getTicket("acme", id)).description).toBe("A theme picker\nWould a picker beat a toggle?");  });

  it("is in the Map view's header, and submits a Prototype on that Map", async () => {
    const { w } = await acme();
    const { spec } = await w.api.submitMap("acme", "Offline mode");
    renderApp(w, `/?view=inbox&requirement=${spec.id}`);

    const pane = await within(detail()).findByRole("region", { name: "Map" });
    const button = within(pane).getByRole("button", { name: "New Prototype" });
    expect(button.compareDocumentPosition(within(pane).getByRole("list", { name: "Map Tickets" })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await userEvent.click(await prototype("Map", "Try a local cache"));
    const id = (await w.api.findTickets("acme")).find((t) => t.title === "Prototype: Try a local cache")!.id;
    await wantPrototype(w, spec.id, id, "Prototype: Try a local cache");
    const node = await within(within(pane).getByRole("list", { name: "Map Tickets" })).findByText(/Prototype: Try a local cache/);
    expect(node.closest("[role=listitem]")).toHaveTextContent("prototype");
  });

  it("is on the plain Spec view, and submits a Prototype on that Spec", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming", body: "How the app looks." });
    renderApp(w, `/?view=inbox&requirement=${spec}`);

    await userEvent.click(await prototype("Spec", "A theme picker"));
    const id = (await w.api.findTickets("acme")).find((t) => t.title === "Prototype: A theme picker")!.id;
    await wantPrototype(w, spec, id, "Prototype: A theme picker");
  });

  it("closes on Cancel without submitting", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming" });
    renderApp(w, `/?view=inbox&requirement=${spec}`);

    await prototype("Spec", "A theme picker");
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).not.toContain("submitPrototype");
  });

  it("shows why a Prototype was refused, and keeps the text", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming" });
    renderApp(w, `/?view=inbox&requirement=${spec}`);

    const submit = await prototype("Spec", "A theme picker");
    // Core refuses every write to an archived Project.
    await s.human("/api/p/acme/archive_project", {});
    await userEvent.click(submit);
    const modal = screen.getByRole("dialog", { name: "New Prototype" });
    expect(await within(modal).findByText(/Project acme is archived/)).toBeInTheDocument();
    expect(within(modal).getByRole("textbox", { name: "Prototype" })).toHaveValue("A theme picker");
  });
});

describe("the plain Spec view", () => {
  it("shows a Spec that is neither a Requirement nor a Map: its header and its Tickets, each opening in the Ticket panel", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming", body: "How the app looks." });
    await s.ticket("acme", { title: "Dark palette", state: "In Review", spec });
    const light = await s.ticket("acme", { title: "Light palette", state: "Ready", spec });
    renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = await within(detail()).findByRole("region", { name: "Spec" });
    expect(pane).toHaveTextContent(spec);
    expect(pane).toHaveTextContent("Spec · submitted just now");
    expect(within(pane).getByRole("heading", { name: "Theming", level: 2 })).toBeInTheDocument();
    const tickets = within(pane).getByRole("list", { name: "Tickets" });
    expect(within(tickets).getAllByRole("listitem").map((li) => li.textContent)).toEqual([expect.stringMatching(/^\S+ Dark palette In ReviewIn Review$/), `${light} Light palette Ready`]);
    await userEvent.click(within(tickets).getByRole("button", { name: /Light palette/ }));
    expect(params().get("ticket")).toBe(light);
    expect(await within(detail()).findByRole("heading", { name: "Light palette", level: 2 })).toBeInTheDocument();
  });

  it("opens from the Spec named in the Ticket panel", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming" });
    const id = await s.ticket("acme", { title: "Dark palette", state: "Ready", spec });
    renderApp(w, `/?view=inbox&ticket=${id}`);

    await userEvent.click(await within(detail()).findByRole("button", { name: `${spec} Theming` }));
    expect(params().get("requirement")).toBe(spec);
    expect(params().get("ticket")).toBe(id);
    expect(await within(detail()).findByRole("region", { name: "Spec" })).toBeInTheDocument();
  });
});
