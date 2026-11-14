// Finished work and Archived Projects in the inbox (ticket 39): the
// Archived group in the left pane, each Project row's menu, and the
// finished States' lists. On a WASM Core world.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld } from "../api/core/core";
import { scenario } from "../api/scenario";
import { ApiError } from "../api/types";
import { renderApp } from "../focus/worldTesting";

const HOUR = 3_600_000;

// world is acme with unfinished work, "Done Co" with only finished work and
// the Archived "Old App": the Done Tickets finished 50, 30 and 20 hours ago
// and the Cancelled one 5 hours ago. ids names their Tickets.
async function world() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("done", "Done Co");
  await s.project("old", "Old App");
  const longAgo = await s.ticket("acme", { title: "Shipped long ago", state: "Done" });
  w.advance(20 * HOUR);
  const lastWeek = await s.ticket("old", { title: "Shipped last week", state: "Done" });
  await s.human(`${s.projectPath("old")}/archive_project`, {});
  w.advance(10 * HOUR);
  const yesterday = await s.ticket("done", { title: "Shipped yesterday", state: "Done" });
  w.advance(15 * HOUR);
  const dropped = await s.ticket("done", { title: "Dropped", state: "Cancelled" });
  await s.ticket("acme", { title: "Still open", state: "Ready" });
  await s.ticket("acme", { title: "Sketch", state: "Draft" });
  return { w, s, ids: { longAgo, lastWeek, yesterday, dropped } };
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const list = () => screen.getByRole("main", { name: "Tickets" });
const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const rowIds = () => [...list().querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id);
const archivedGroup = () => within(nav()).getByText(/^Archived/).closest("details")!;

async function openMenu(name: string) {
  await userEvent.click(await within(nav()).findByRole("button", { name: `Menu for ${name}`, hidden: true }));
  return screen.getByRole("menu", { name: `Menu for ${name}` });
}

describe("Archived Projects and finished work", () => {
  it("keeps Archived Projects in a collapsed group at the bottom of the left pane", async () => {
    renderApp((await world()).w, "/");
    const old = await within(nav()).findByRole("button", { name: /^Old App/, hidden: true });
    const group = archivedGroup();
    expect(group).not.toHaveAttribute("open");
    expect(group).toContainElement(old);
    expect(old).not.toBeVisible();
    expect(within(group).getByText("Archived (1)")).toBeInTheDocument();
    // Active Projects stay above, out of the group; the group comes last.
    expect(group).not.toContainElement(within(nav()).getByRole("button", { name: /^Acme/ }));
    expect(nav().lastElementChild).toBe(group);
  });

  it("disables Archive, saying why, while unfinished work remains", async () => {
    const { w } = await world();
    renderApp(w, "/");
    const menu = await openMenu("Acme");
    const archive = within(menu).getByRole("menuitem", { name: "Archive" });
    expect(archive).toBeDisabled();
    expect(archive).toHaveAccessibleDescription("2 unfinished Tickets: finish or cancel them first");
    await userEvent.click(archive);
    expect(w.calls).not.toContain("archiveProject");
  });

  it("archives a finished Project from its menu, and unarchives it", async () => {
    const { w } = await world();
    renderApp(w, "/");
    await userEvent.click(within(await openMenu("Done Co")).getByRole("menuitem", { name: "Archive" }));
    await waitFor(() => expect(archivedGroup()).toContainElement(within(nav()).getByRole("button", { name: /^Done Co/, hidden: true })));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(within(archivedGroup()).getByText("Archived (2)")).toBeInTheDocument();

    await userEvent.click(archivedGroup().querySelector("summary")!);
    const menu = await openMenu("Done Co");
    expect(within(menu).queryByRole("menuitem", { name: "Archive" })).toBeNull();
    await userEvent.click(within(menu).getByRole("menuitem", { name: "Unarchive" }));
    await waitFor(() => expect(archivedGroup()).not.toContainElement(within(nav()).getByRole("button", { name: /^Done Co/ })));
    expect(w.calls).toEqual(expect.arrayContaining(["archiveProject", "unarchiveProject"]));
  });

  it("shows a refused archive's reason", async () => {
    const { w } = await world();
    // Work that arrived after the list was read: the service refuses.
    const api = {
      ...w.api,
      archiveProject: async () => {
        throw new ApiError(409, "action_not_allowed", "Project done still has unfinished work: done-3 (Ready)");
      },
    };
    window.history.replaceState(null, "", "/");
    render(<App api={api} now={w.now} />);
    await userEvent.click(within(await openMenu("Done Co")).getByRole("menuitem", { name: "Archive" }));
    expect(await within(nav()).findByRole("alert")).toHaveTextContent("still has unfinished work: done-3 (Ready)");
  });

  it("lists Done and Cancelled newest first, with plain counts, across every Project", async () => {
    const { w, ids } = await world();
    renderApp(w, "/");
    const done = await within(nav()).findByRole("button", { name: "Done 3" });
    expect(done.querySelector(".count")).not.toHaveClass("loud");
    expect(within(nav()).getByRole("button", { name: "Cancelled 1" })).toBeInTheDocument();

    await userEvent.click(done);
    await waitFor(() => expect(rowIds()).toEqual([ids.yesterday, ids.lastWeek, ids.longAgo]));
    await userEvent.click(within(nav()).getByRole("button", { name: /^Cancelled/ }));
    await waitFor(() => expect(rowIds()).toEqual([ids.dropped]));
  });

  it("opens any Ticket by its URL, an Archived Project's too", async () => {
    const { w, ids } = await world();
    renderApp(w, `/?view=inbox&ticket=${ids.lastWeek}`);
    expect(await within(detail()).findByRole("heading", { name: "Shipped last week" })).toBeInTheDocument();
  });
});
