// The Sole Project in the UI (Spec code-central-699): with exactly one
// Project that isn't Archived and none named in the URL, every view is
// scoped to it, IDs show short and All Projects isn't offered, all without
// the address changing. On a WASM Core world.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { openProjectPicker, projectChoices, projectPicker } from "./testPickers";
import { describe, expect, it, vi } from "vitest";
import { newWorld, type World } from "./api/core/core";
import { renderApp, scenario, settle } from "./focus/worldTesting";

const beat = { factory: "code-central-factory", host: "laptop", interval_seconds: 30 } as const;

// world is acme (one Ready Ticket) and the Archived "Old App" (a Done one).
async function world(...more: string[]) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("old", "Old App");
  const old = await s.ticket("old", { title: "Shipped", state: "Done" });
  await w.api.archiveProject("old");
  const mine = await s.ticket("acme", { title: "Mine", state: "Ready" });
  for (const key of more) await s.project(key, key.toUpperCase());
  return { w, s, mine, old };
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const address = () => window.location.search;

describe("the Inbox with a Sole Project", () => {
  it("scopes to it with Short IDs, its filter on and no All Projects, keeping its Project Options link", async () => {
    const { w, mine } = await world();
    await renderApp(w, "/?view=inbox&list=tickets");
    const short = `#${mine.slice(mine.lastIndexOf("-") + 1)}`;
    expect(screen.getByText(short)).toBeInTheDocument();
    expect(screen.queryByText(mine)).toBeNull();
    expect(within(nav()).queryByRole("button", { name: /^All Projects/ })).toBeNull();
    expect(within(nav()).getByRole("button", { name: /^Acme/ })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(within(nav()).getByRole("button", { name: "Menu for Acme" }));
    expect(screen.getByRole("menuitem", { name: "Options" })).toHaveAttribute("href", "?view=project-options&project=acme");
  });

  it("copies the full ID from a Short ID", async () => {
    const { w, mine } = await world();
    await renderApp(w, "/?view=inbox&list=tickets");
    const write = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText: write }, configurable: true });
    await userEvent.click(screen.getByRole("button", { name: mine }));
    expect(write).toHaveBeenCalledWith(mine);
  });

  it("leaves the address alone", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=inbox");
    expect(screen.getByRole("button", { name: /^Acme/ })).toBeInTheDocument();
    expect(address()).toBe("?view=inbox");
  });

  it("lets a named Archived Project win, with the Sole Project's filter still the way back", async () => {
    const { w, old } = await world();
    await renderApp(w, "/?view=inbox&project=old&list=tickets&finished=1");
    expect(screen.getByText(`#${old.slice(old.lastIndexOf("-") + 1)}`)).toBeInTheDocument();
    expect(within(nav()).queryByRole("button", { name: /^All Projects/ })).toBeNull();
    await userEvent.click(within(nav()).getByRole("button", { name: /^Acme/ }));
    expect(address()).toContain("project=acme");
  });

  it("behaves as before with two active Projects", async () => {
    const { w, mine } = await world("beta");
    await renderApp(w, "/?view=inbox&list=tickets");
    expect(screen.getByText(mine)).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: /^All Projects/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("follows archiving and unarchiving through the live feed", async () => {
    const { w, mine } = await world("beta");
    await renderApp(w, "/?view=inbox&list=tickets");
    expect(screen.getByText(mine)).toBeInTheDocument();
    await w.api.archiveProject("beta");
    await settle(w);
    expect(within(nav()).queryByRole("button", { name: /^All Projects/ })).toBeNull();
    expect(screen.getByRole("button", { name: mine })).toHaveTextContent(/^#/);
    await w.api.unarchiveProject("beta");
    await settle(w);
    expect(within(nav()).getByRole("button", { name: /^All Projects/ })).toBeInTheDocument();
    expect(screen.getByText(mine)).toBeInTheDocument();
    expect(address()).toBe("?view=inbox&list=tickets");
  });
});

describe("the full-width views with a Sole Project", () => {
  it.each(["board", "ci", "activity", "usage"])("%s: the picker shows the Sole Project, lists the Archived one and has no All Projects", async (view) => {
    const { w } = await world();
    await renderApp(w, `/?view=${view}`);
    const main = screen.getByRole("main");
    expect(projectPicker(main)).toHaveTextContent("Acme");
    expect(projectChoices(await openProjectPicker(main))).toEqual(["Acme", "Old App"]);
    expect(address()).toBe(`?view=${view}`);
  });

  it("offers All Projects with two active Projects", async () => {
    const { w } = await world("beta");
    await renderApp(w, "/?view=board");
    expect(projectChoices(await openProjectPicker(screen.getByRole("main")))[0]).toBe("All Projects");
  });

  it("the Board scopes to it with Short IDs on its cards", async () => {
    const { w, s, mine } = await world();
    await s.project("other", "Other");
    await w.api.archiveProject("other");
    await renderApp(w, "/?view=board");
    expect(screen.getByText(`#${mine.slice(mine.lastIndexOf("-") + 1)}`)).toBeInTheDocument();
  });

  it("keeps the address clean through the Activity, Board, CI and Usage links and Back", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=inbox");
    for (const name of [/^Activity/, /^Board/]) {
      await userEvent.click(within(nav()).getByRole("link", { name }));
      await settle(w);
      expect(address()).not.toContain("project=");
      await userEvent.click(screen.getByRole("button", { name: /^Back|Inbox/ }));
      await settle(w);
      expect(address()).not.toContain("project=");
    }
  });

  it("holds CI and Usage back from fetching All Projects until the Project list arrives", async () => {
    for (const view of ["ci", "usage"]) {
      const { w } = await world();
      const real = w.api.listProjects.bind(w.api);
      let release!: () => void;
      const gate = new Promise<void>((r) => (release = r));
      vi.spyOn(w.api, "listProjects").mockImplementation(async () => (await gate, real()));
      await renderApp(w, `/?view=${view}`);
      expect(w.calls).not.toContain(view === "ci" ? "ciOverview" : "usage");
      release();
      await settle(w);
      expect(w.calls).toContain(view === "ci" ? "ciOverview" : "usage");
      vi.restoreAllMocks();
    }
  });

  it("fetches as today when the Project list read fails", async () => {
    const { w } = await world();
    vi.spyOn(w.api, "listProjects").mockRejectedValue(new Error("down"));
    await renderApp(w, "/?view=ci");
    expect(w.calls).toContain("ciOverview");
    vi.restoreAllMocks();
  });
});

describe("Focus Mode, Project Options, + New and Chat with a Sole Project", () => {
  it("scopes Focus Mode to it", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=focus");
    const main = screen.getByRole("main", { name: "Focus Mode" });
    expect(within(main).getByText("Acme")).toBeInTheDocument();
    expect(address()).toBe("?view=focus");
  });

  it("shows its Project Options when none is named", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=project-options");
    expect(screen.getByRole("heading", { name: /Acme/ })).toBeInTheDocument();
  });

  it("defaults + New to it without asking which Project", async () => {
    const { w } = await world("beta");
    await w.api.archiveProject("beta");
    await renderApp(w, "/?view=inbox");
    await userEvent.click(within(nav()).getByRole("button", { name: "New requirement" }));
    expect(within(screen.getByRole("dialog")).queryByRole("combobox")).toBeNull();
  });

  it("shows the Chat dock for it with an online Factory that supports Chat, and not without", async () => {
    const a = await world();
    await renderApp(a.w, "/?view=inbox");
    expect(screen.getByRole("button", { name: /^Acme/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Chat" })).toBeNull();
  });

  it("shows the Chat dock with a Chat-capable Factory", async () => {
    const { w, s } = await world();
    await s.reportHeartbeat("acme", { ...beat, state: "online", chat: true });
    await renderApp(w, "/?view=inbox");
    expect(screen.getByRole("button", { name: "Chat" })).toBeInTheDocument();
  });
});

export type { World };
