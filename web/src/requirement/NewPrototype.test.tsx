// New Prototype tests: the real App on a WASM Core world. "New Prototype" on
// each Spec's view (Requirement, Map and plain Spec), and the plain Spec
// view it sits on.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { renderApp, scenario, settle } from "../worldTesting";

// acme is a world with the Project acme, and the scenario builder on it.
async function acme(...more: string[]) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  for (const key of more) await s.project(key, key[0].toUpperCase() + key.slice(1));
  return { w, s };
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const params = () => new URLSearchParams(window.location.search);

// click is a user's click, settled.
async function click(w: World, el: Element) {
  await userEvent.click(el);
  await settle(w);
}

// prototype asks for a Prototype from the view named view and returns
// the dialog's submit, once the text is typed.
async function prototype(w: World, view: string, text: string) {
  const pane = within(detail()).getByRole("region", { name: view });
  await click(w, within(pane).getByRole("button", { name: "New Prototype" }));
  const modal = screen.getByRole("dialog", { name: "New Prototype" });
  expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
  expect(within(modal).queryByRole("combobox")).toBeNull();
  const submit = within(modal).getByRole("button", { name: "Submit prototype" });
  expect(submit).toBeDisabled();
  await userEvent.type(within(modal).getByRole("textbox", { name: "Prototype" }), text);
  await settle(w);
  return submit;
}

// wantPrototype checks the new Prototype is a Ready prototype Ticket on
// spec, and is the selected Ticket.
async function wantPrototype(w: World, spec: string, id: string, title: string) {
  expect(w.calls).toContain("submitPrototype");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(await w.api.getTicket("acme", id)).toMatchObject({ title, state: "Ready", spec: { id: spec }, labels: ["prototype"] });
  expect(params().get("requirement")).toBe(spec);
  expect(params().get("ticket")).toBe(id);
  expect(within(detail()).getByRole("heading", { name: title, level: 2 })).toBeInTheDocument();
}

describe("New Prototype", () => {
  it("is on the Requirement view, and submits a Prototype on that Requirement", async () => {
    const { w } = await acme("beta");
    const { spec } = await w.api.submitRequirement("acme", "Dark mode");
    await renderApp(w, `/?view=inbox&requirement=${spec.id}`);

    await click(w, await prototype(w, "Requirement", "A theme picker{Enter}Would a picker beat a toggle?"));
    const id = (await w.api.findTickets("acme")).find((t) => t.title === "Prototype: A theme picker")!.id;
    await wantPrototype(w, spec.id, id, "Prototype: A theme picker");
    expect((await w.api.getTicket("acme", id)).description).toBe("A theme picker\nWould a picker beat a toggle?");
  });

  it("is in the Map view's header, and submits a Prototype on that Map", async () => {
    const { w } = await acme();
    const { spec } = await w.api.submitMap("acme", "Offline mode");
    await renderApp(w, `/?view=inbox&requirement=${spec.id}`);

    const pane = within(detail()).getByRole("region", { name: "Map" });
    const button = within(pane).getByRole("button", { name: "New Prototype" });
    expect(button.compareDocumentPosition(within(pane).getByRole("list", { name: "Map Tickets" })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await click(w, await prototype(w, "Map", "Try a local cache"));
    const id = (await w.api.findTickets("acme")).find((t) => t.title === "Prototype: Try a local cache")!.id;
    await wantPrototype(w, spec.id, id, "Prototype: Try a local cache");
    const node = within(within(pane).getByRole("list", { name: "Map Tickets" })).getByText(/Prototype: Try a local cache/);
    expect(node.closest("[role=listitem]")).toHaveTextContent("prototype");
  });

  it("is on the plain Spec view, and submits a Prototype on that Spec", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming", body: "How the app looks." });
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    await click(w, await prototype(w, "Spec", "A theme picker"));
    const id = (await w.api.findTickets("acme")).find((t) => t.title === "Prototype: A theme picker")!.id;
    await wantPrototype(w, spec, id, "Prototype: A theme picker");
  });

  it("closes on Cancel without submitting", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming" });
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    await prototype(w, "Spec", "A theme picker");
    await click(w, within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).not.toContain("submitPrototype");
  });

  it("shows why a Prototype was refused, and keeps the text", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming" });
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const submit = await prototype(w, "Spec", "A theme picker");
    // Core refuses every write to an archived Project.
    await s.human("/api/p/acme/archive_project", {});
    await click(w, submit);
    const modal = screen.getByRole("dialog", { name: "New Prototype" });
    expect(within(modal).getByText(/Project acme is archived/)).toBeInTheDocument();
    expect(within(modal).getByRole("textbox", { name: "Prototype" })).toHaveValue("A theme picker");
  });
});

describe("the plain Spec view", () => {
  it("shows a Spec that is neither a Requirement nor a Map: its header and its Tickets, each opening in the Ticket panel", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming", body: "How the app looks." });
    await s.ticket("acme", { title: "Dark palette", state: "In Review", spec });
    const light = await s.ticket("acme", { title: "Light palette", state: "Ready", spec });
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = within(detail()).getByRole("region", { name: "Spec" });
    expect(pane).toHaveTextContent(spec);
    expect(pane).toHaveTextContent("Spec · submitted just now");
    expect(within(pane).getByRole("heading", { name: "Theming", level: 2 })).toBeInTheDocument();
    const tickets = within(pane).getByRole("list", { name: "Tickets" });
    expect(within(tickets).getAllByRole("listitem").map((li) => li.textContent)).toEqual([expect.stringMatching(/^\S+ Dark palette In Review In Review$/), `${light} Light palette Ready`]);
    await click(w, within(tickets).getByRole("button", { name: /Light palette/ }));
    expect(params().get("ticket")).toBe(light);
    expect(within(detail()).getByRole("heading", { name: "Light palette", level: 2 })).toBeInTheDocument();
  });

  it("opens from the Spec named in the Ticket panel", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Theming" });
    const id = await s.ticket("acme", { title: "Dark palette", state: "Ready", spec });
    await renderApp(w, `/?view=inbox&ticket=${id}`);

    await click(w, within(detail()).getByRole("button", { name: `${spec} Theming` }));
    expect(params().get("requirement")).toBe(spec);
    expect(params().get("ticket")).toBe(id);
    expect(within(detail()).getByRole("region", { name: "Spec" })).toBeInTheDocument();
  });
});
