// Retro tests: the real App on a WASM Core world. "Retro" in the Ticket
// pane's header, its dialog, and the Retro badge.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";
import { ApiError, type State } from "../api/types";

function renderApp(w: World, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

// acme is a world with the Project acme and a Spec, and the scenario builder.
async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const spec = await s.spec("acme", { title: "Theming" });
  return { w, s, spec };
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const params = () => new URLSearchParams(window.location.search);

// startRetro opens the dialog from the Ticket pane.
async function startRetro() {
  await userEvent.click(await within(detail()).findByRole("button", { name: "Retro" }));
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
    renderApp(w, `/?view=inbox&ticket=${id}`);

    expect(await within(detail()).findByRole("heading", { name: "Dark palette", level: 2 })).toBeInTheDocument();
    const retro = within(detail()).queryByRole("button", { name: "Retro" });
    if (shown) expect(retro).toBeInTheDocument();
    else expect(retro).toBeNull();
  });

  it("is absent on a Done Ticket in an Archived Project", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await s.archive("acme");
    renderApp(w, `/?view=inbox&ticket=${id}`);

    expect(await within(detail()).findByRole("heading", { name: "Dark palette", level: 2 })).toBeInTheDocument();
    expect(within(detail()).queryByRole("button", { name: "Retro" })).toBeNull();
  });

  it("opens a dialog with one optional Note, and submits with an empty note", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    renderApp(w, `/?view=inbox&ticket=${id}`);

    const modal = await startRetro();
    expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
    expect(within(modal).getByRole("textbox", { name: "Note" })).toHaveValue("");
    const submit = within(modal).getByRole("button", { name: "Start retro" });
    expect(submit).toBeEnabled();
    await userEvent.click(submit);

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(w.calls).toContain("submitRetro");
    const retro = (await w.api.findTickets("acme")).find((t) => t.title === "Retro: Dark palette")!;
    expect(await w.api.getTicket("acme", retro.id)).toMatchObject({ state: "Ready", labels: ["retro"], tags: { "retro-of": id } });
    expect(params().get("ticket")).toBe(retro.id);
    expect(await within(detail()).findByRole("heading", { name: "Retro: Dark palette", level: 2 })).toBeInTheDocument();
  });

  it("puts the note in the Retro, and selects it", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Cancelled", spec });
    renderApp(w, `/?view=inbox&ticket=${id}`);

    const modal = await startRetro();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Note" }), "Why did the agent stall?");
    await userEvent.click(within(modal).getByRole("button", { name: "Start retro" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const retro = (await w.api.findTickets("acme")).find((t) => t.title === "Retro: Dark palette")!;
    expect((await w.api.getTicket("acme", retro.id)).description).toContain("Why did the agent stall?");
    expect(params().get("ticket")).toBe(retro.id);
  });

  it("closes on Cancel without submitting", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    renderApp(w, `/?view=inbox&ticket=${id}`);

    const modal = await startRetro();
    await userEvent.click(within(modal).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).not.toContain("submitRetro");
  });

  it("shows why a Retro was refused in the dialog, and keeps the note", async () => {
    const { w, s, spec } = await acme();
    const id = await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    renderApp(w, `/?view=inbox&ticket=${id}`);

    const modal = await startRetro();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Note" }), "Look at CI");
    // An Archived Project hides the button, and its dialog, so the refusal is made up.
    vi.spyOn(w.api, "submitRetro").mockRejectedValue(new ApiError(422, "invalid", "Dark palette isn't finished", "target"));
    await userEvent.click(within(modal).getByRole("button", { name: "Start retro" }));
    expect(await within(modal).findByText("Dark palette isn't finished")).toBeInTheDocument();
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
    renderApp(w, "/?view=inbox");

    const row = (id: string) => screen.findByRole("link", { name: new RegExp(id) });
    expect(within(await row(retro)).getByText("Retro")).toHaveClass("label", "retro-badge");
    expect(within(await row(other)).queryByText("Retro")).toBeNull();
  });
});
