// Retro tests: the real App on a WASM Core world. "Retro" in the Ticket
// pane's header, its dialog, and the Retro badge.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { ApiError, type State } from "../api/types";
import { renderApp, scenario, settle } from "../worldTesting";

// acme is a world with the Project acme and a Spec, and the scenario builder.
async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  const spec = await s.spec("acme", { title: "Theming" });
  return { w, s, spec };
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const params = () => new URLSearchParams(window.location.search);

// click is a user's click, settled.
async function click(w: World, el: Element) {
  await userEvent.click(el);
  await settle(w);
}

// type is a user's typing, settled.
async function type(w: World, el: Element, text: string) {
  await userEvent.type(el, text);
  await settle(w);
}

// startRetro opens the dialog from the Ticket pane.
async function startRetro(w: World) {
  await click(w, within(detail()).getByRole("button", { name: "Retro" }));
  return screen.getByRole("dialog", { name: "Retro" });
}

describe("Retro on a Ticket", () => {
  it.each<[State, boolean]>([
    ["Done", true],
    ["Cancelled", true],
    ["Draft", false],
    ["Ready", false],
    ["In Review", false],
  ])("a %s Ticket's header has the Retro button: %s", async (state, shown) => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state, spec });
    await renderApp(w, `/?view=inbox&ticket=${id}`);

    expect(within(detail()).getByRole("heading", { name: "Dark palette", level: 2 })).toBeInTheDocument();
    const retro = within(detail()).queryByRole("button", { name: "Retro" });
    if (shown) expect(retro).toBeInTheDocument();
    else expect(retro).toBeNull();
  });

  it("is absent on a Done Ticket in an Archived Project", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await s.archive("acme");
    await renderApp(w, `/?view=inbox&ticket=${id}`);

    expect(within(detail()).getByRole("heading", { name: "Dark palette", level: 2 })).toBeInTheDocument();
    expect(within(detail()).queryByRole("button", { name: "Retro" })).toBeNull();
  });

  it("opens a dialog with one optional Note, and submits with an empty note", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await renderApp(w, `/?view=inbox&ticket=${id}`);

    const modal = await startRetro(w);
    expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
    expect(within(modal).getByRole("textbox", { name: "Note" })).toHaveValue("");
    const submit = within(modal).getByRole("button", { name: "Start retro" });
    expect(submit).toBeEnabled();
    await click(w, submit);

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).toContain("submitRetro");
    const retro = (await w.api.findTickets("acme")).find((t) => t.title === "Retro: Dark palette")!;
    expect(await w.api.getTicket("acme", retro.id)).toMatchObject({ state: "Ready", labels: ["retro"], tags: { "retro-of": id } });
    expect(params().get("ticket")).toBe(retro.id);
    expect(within(detail()).getByRole("heading", { name: "Retro: Dark palette", level: 2 })).toBeInTheDocument();
  });

  it("puts the note in the Retro, and selects it", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Cancelled", spec });
    await renderApp(w, `/?view=inbox&ticket=${id}`);

    const modal = await startRetro(w);
    await type(w, within(modal).getByRole("textbox", { name: "Note" }), "Why did the agent stall?");
    await click(w, within(modal).getByRole("button", { name: "Start retro" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    const retro = (await w.api.findTickets("acme")).find((t) => t.title === "Retro: Dark palette")!;
    expect((await w.api.getTicket("acme", retro.id)).description).toContain("Why did the agent stall?");
    expect(params().get("ticket")).toBe(retro.id);
  });

  it("closes on Cancel without submitting", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await renderApp(w, `/?view=inbox&ticket=${id}`);

    const modal = await startRetro(w);
    await click(w, within(modal).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).not.toContain("submitRetro");
  });

  it("shows why a Retro was refused in the dialog, and keeps the note", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await renderApp(w, `/?view=inbox&ticket=${id}`);

    const modal = await startRetro(w);
    await type(w, within(modal).getByRole("textbox", { name: "Note" }), "Look at CI");
    // An Archived Project hides the button, and its dialog, so the refusal is made up.
    vi.spyOn(w.api, "submitRetro").mockRejectedValue(new ApiError(422, "invalid", "Dark palette isn't finished", "target"));
    await click(w, within(modal).getByRole("button", { name: "Start retro" }));
    expect(within(modal).getByText("Dark palette isn't finished")).toBeInTheDocument();
    expect(within(modal).getByRole("textbox", { name: "Note" })).toHaveValue("Look at CI");
  });
});

describe("the Retro badge", () => {
  it("marks a Retro's row in Needs Me, and no other", async () => {
    const { w, s, spec } = await acme();
    const done = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    const other = await s.ticket("acme", { title: "Light palette", state: "Ready", spec });
    const retro = (await w.api.submitRetro("acme", done)).id;
    await s.inReview("acme", retro);
    await s.inReview("acme", other);
    await renderApp(w, "/?view=inbox");

    const row = (id: string) => screen.getByRole("link", { name: new RegExp(id) });
    expect(within(row(retro)).getByText("Retro")).toHaveClass("label", "retro-badge");
    expect(within(row(other)).queryByText("Retro")).toBeNull();
  });
});

// finish has every Ticket on the Spec cancelled, so the Spec is Finished.
async function finish(w: World, spec: string) {
  for (const t of (await w.api.getSpec("acme", spec)).tickets!) await w.api.act("acme", t.id, "cancel", {});
  expect((await w.api.getSpec("acme", spec)).finished).toBe(true);
}

const specRetro = (view: string) => within(detail()).getByRole("region", { name: view });

describe("Retro on a Spec", () => {
  it("is beside New Prototype on a Finished plain Spec, and submits a Retro on it with the note", async () => {
    const { w, s, spec } = await acme();
    await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = specRetro("Spec");
    const retro = within(pane).getByRole("button", { name: "Retro" });
    expect(within(pane).getByRole("button", { name: "New Prototype" }).nextElementSibling).toBe(retro);
    await click(w, retro);
    const modal = screen.getByRole("dialog", { name: "Retro" });
    await type(w, within(modal).getByRole("textbox", { name: "Note" }), "Was the split right?");
    await click(w, within(modal).getByRole("button", { name: "Start retro" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).toContain("submitRetro");
    const made = (await w.api.findTickets("acme")).find((t) => t.title === "Retro: Theming")!;
    expect(await w.api.getTicket("acme", made.id)).toMatchObject({ state: "Ready", labels: ["retro"], tags: { "retro-of": spec } });
    expect((await w.api.getTicket("acme", made.id)).description).toContain("Was the split right?");
    expect(params().get("ticket")).toBe(made.id);
    expect(within(detail()).getByRole("heading", { name: "Retro: Theming", level: 2 })).toBeInTheDocument();
  });

  it("submits with an empty note", async () => {
    const { w, s, spec } = await acme();
    await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    await click(w, within(specRetro("Spec")).getByRole("button", { name: "Retro" }));
    await click(w, within(screen.getByRole("dialog", { name: "Retro" })).getByRole("button", { name: "Start retro" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    const made = (await w.api.findTickets("acme")).find((t) => t.title === "Retro: Theming")!;
    expect(params().get("ticket")).toBe(made.id);
  });

  it("is on a Finished Map's view", async () => {
    const { w } = await acme();
    const { spec } = await w.api.submitMap("acme", "Offline mode");
    await finish(w, spec.id);
    await renderApp(w, `/?view=inbox&requirement=${spec.id}`);

    await click(w, within(specRetro("Map")).getByRole("button", { name: "Retro" }));
    await click(w, within(screen.getByRole("dialog", { name: "Retro" })).getByRole("button", { name: "Start retro" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    const made = (await w.api.findTickets("acme")).find((t) => t.title === "Retro: Offline mode")!;
    expect(await w.api.getTicket("acme", made.id)).toMatchObject({ tags: { "retro-of": spec.id } });
  });

  it("is on a Finished Requirement's view", async () => {
    const { w } = await acme();
    const { spec } = await w.api.submitRequirement("acme", "Dark mode");
    await finish(w, spec.id);
    await renderApp(w, `/?view=inbox&requirement=${spec.id}`);

    await click(w, within(specRetro("Requirement")).getByRole("button", { name: "Retro" }));
    await click(w, within(screen.getByRole("dialog", { name: "Retro" })).getByRole("button", { name: "Start retro" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).toContain("submitRetro");
  });

  it("is absent on an unfinished Spec, Map and Requirement, and on a Finished Long-lived Spec", async () => {
    const { w, s, spec } = await acme();
    await s.ticket("acme", { title: "Dark palette", state: "Ready", spec });
    const longLived = await s.spec("acme", { title: "Small changes", labels: ["long-lived"] });
    await s.ticket("acme", { title: "Rename", state: "Done", spec: longLived });
    const map = (await w.api.submitMap("acme", "Offline mode")).spec.id;
    const req = (await w.api.submitRequirement("acme", "Dark mode")).spec.id;
    expect((await w.api.getSpec("acme", longLived)).finished).toBe(true);

    for (const [id, view] of [[spec, "Spec"], [longLived, "Spec"], [map, "Map"], [req, "Requirement"]] as const) {
      const { unmount } = await renderApp(w, `/?view=inbox&requirement=${id}`);
      const pane = specRetro(view);
      expect(within(pane).getByRole("button", { name: "New Prototype" })).toBeInTheDocument();
      expect(within(pane).queryByRole("button", { name: "Retro" })).toBeNull();
      unmount();
    }
  });

  it("is absent on a Finished Spec in an Archived Project", async () => {
    const { w, s, spec } = await acme();
    await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await s.archive("acme");
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = specRetro("Spec");
    expect(within(pane).getByRole("button", { name: "New Prototype" })).toBeInTheDocument();
    expect(within(pane).queryByRole("button", { name: "Retro" })).toBeNull();
  });
});

describe("the Retros list", () => {
  it("lists a Spec's Retros as Ticket lines that select them, and a Ticket's too", async () => {
    const { w, s, spec } = await acme();
    const done = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    const onSpec = (await w.api.submitRetro("acme", spec)).id;
    const onTicket = (await w.api.submitRetro("acme", done)).id;
    await renderApp(w, `/?view=inbox&requirement=${spec}&ticket=${done}`);

    const specList = within(specRetro("Spec")).getByRole("list", { name: "Retros" });
    expect(within(specList).getAllByRole("listitem").map((li) => li.textContent)).toEqual([`${onSpec} Retro: Theming Ready`]);
    const ticketList = within(screen.getByRole("article", { name: "Ticket details" })).getByRole("list", { name: "Retros" });
    expect(within(ticketList).getAllByRole("listitem").map((li) => li.textContent)).toEqual([`${onTicket} Retro: Dark palette Ready`]);

    await click(w, within(ticketList).getByRole("button", { name: /Retro: Dark palette/ }));
    expect(params().get("ticket")).toBe(onTicket);
    expect(within(detail()).getByRole("heading", { name: "Retro: Dark palette", level: 2 })).toBeInTheDocument();
  });

  it("is on a view only when the Spec has Retros", async () => {
    const { w, spec } = await acme();
    await renderApp(w, `/?view=inbox&requirement=${spec}`);
    specRetro("Spec");
    expect(within(detail()).queryByRole("list", { name: "Retros" })).toBeNull();
  });

  it("selects a Retro from a Map's and a Requirement's list", async () => {
    const { w } = await acme();
    const map = (await w.api.submitMap("acme", "Offline mode")).spec.id;
    const req = (await w.api.submitRequirement("acme", "Dark mode")).spec.id;
    for (const [id, view] of [[map, "Map"], [req, "Requirement"]] as const) {
      await finish(w, id);
      const retro = (await w.api.submitRetro("acme", id)).id;
      const { unmount } = await renderApp(w, `/?view=inbox&requirement=${id}`);
      const list = within(specRetro(view)).getByRole("list", { name: "Retros" });
      await click(w, within(list).getByRole("button", { name: /Retro: / }));
      expect(params().get("ticket")).toBe(retro);
      unmount();
    }
  });
});
