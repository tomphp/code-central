// The Auto-accept section of Project Options (code-central-545): one
// checkbox per Auto-accept kind, all off to begin with, saved through
// set_auto_accept with one Save; it follows changes made elsewhere and is
// read-only on an Archived Project. On the WASM Core.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";

function renderApp(w: World) {
  window.history.replaceState(null, "", "/?view=project-options&project=acme");
  return render(<App api={w.api} now={w.now} />);
}

const KINDS = ["Grilling outcomes", "Spec recommendation", "Tickets Stage", "Implementation results", "Merge results"];

const section = async () =>
  within(await screen.findByRole("main", { name: "Project Options" })).findByRole("group", { name: "Auto-accept" });
const box = (g: HTMLElement, name: string) => within(g).getByRole("checkbox", { name });
const fresh = async (opts: { archived?: boolean } = {}) => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  if (opts.archived) await s.archive("acme");
  return w;
};

describe("the Auto-accept section", () => {
  it("shows a checkbox per kind, all off, and says Prototypes and Map Tickets are never auto-accepted", async () => {
    renderApp(await fresh());

    const g = await section();
    expect(within(g).getAllByRole("checkbox").map((c) => c.closest("label")!.textContent!.trim())).toEqual(KINDS);
    for (const k of KINDS) expect(box(g, k)).not.toBeChecked();
    expect(g).toHaveTextContent("Prototypes and Map Tickets are never auto-accepted");
    expect(within(g).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("is in the section menu", async () => {
    renderApp(await fresh());
    await section();
    const menu = screen.getByRole("navigation", { name: "Sections" });
    expect(within(menu).getByRole("link", { name: "Auto-accept" })).toBeInTheDocument();
  });

  it("reflects the Project's setting", async () => {
    const w = await fresh();
    await w.api.setAutoAccept("acme", { spec: true, merge: true });
    renderApp(w);

    const g = await section();
    expect(box(g, "Spec recommendation")).toBeChecked();
    expect(box(g, "Merge results")).toBeChecked();
    expect(box(g, "Grilling outcomes")).not.toBeChecked();
  });

  it("saves the kinds ticked through set_auto_accept", async () => {
    const w = await fresh();
    renderApp(w);
    const g = await section();

    await userEvent.click(box(g, "Implementation results"));
    await userEvent.click(box(g, "Tickets Stage"));
    await userEvent.click(within(g).getByRole("button", { name: "Save" }));

    expect(await within(g).findByRole("status")).toHaveTextContent("Saved");
    expect(w.calls).toContain("setAutoAccept");
    expect((await w.api.listProjects())[0].auto_accept).toEqual({
      grilling: false,
      spec: false,
      tickets: true,
      implementation: true,
      merge: false,
    });
  });

  it("turns a kind off again", async () => {
    const w = await fresh();
    await w.api.setAutoAccept("acme", { implementation: true });
    renderApp(w);
    const g = await section();

    await userEvent.click(box(g, "Implementation results"));
    await userEvent.click(within(g).getByRole("button", { name: "Save" }));

    await waitFor(async () => expect((await w.api.listProjects())[0].auto_accept.implementation).toBe(false));
    expect(box(g, "Implementation results")).not.toBeChecked();
  });

  it("shows a change made elsewhere, as another tab would", async () => {
    const w = await fresh();
    renderApp(w);
    const g = await section();

    await w.api.setAutoAccept("acme", { grilling: true });
    await waitFor(() => expect(box(g, "Grilling outcomes")).toBeChecked());
  });

  it("is read-only on an Archived Project", async () => {
    renderApp(await fresh({ archived: true }));

    const g = await section();
    for (const k of KINDS) expect(box(g, k)).toBeDisabled();
  });
});
