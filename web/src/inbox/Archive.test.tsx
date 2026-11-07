// Finished work and Archived Projects in the inbox (ticket 39): the
// Archived group in the left pane, each Project row's menu, and the
// finished States' lists. Against FakeApi.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { ApiError } from "../api/types";

const hoursAgo = (fake: FakeApi, h: number) => new Date(fake.now.getTime() - h * 3_600_000).toISOString();

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("done", "Done Co");
  fake.addProject("old", "Old App", { archived: true });
  fake.addTicket("acme", { title: "Still open", state: "Ready" }); // acme-1
  fake.addTicket("acme", { title: "Sketch", state: "Draft" }); // acme-2
  fake.addTicket("acme", { title: "Shipped long ago", state: "Done", updated: hoursAgo(fake, 50) }); // acme-3
  fake.addTicket("done", { title: "Shipped yesterday", state: "Done", updated: hoursAgo(fake, 20) }); // done-1
  fake.addTicket("done", { title: "Dropped", state: "Cancelled", updated: hoursAgo(fake, 5) }); // done-2
  fake.addTicket("old", { title: "Shipped last week", state: "Done", updated: hoursAgo(fake, 30) }); // old-1
  return fake;
}

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
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
    renderApp(world());
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
    const fake = world();
    renderApp(fake);
    const menu = await openMenu("Acme");
    const archive = within(menu).getByRole("menuitem", { name: "Archive" });
    expect(archive).toBeDisabled();
    expect(archive).toHaveAccessibleDescription("2 unfinished Tickets: finish or cancel them first");
    await userEvent.click(archive);
    expect(fake.calls).not.toContain("archiveProject");
  });

  it("archives a finished Project from its menu, and unarchives it", async () => {
    const fake = world();
    renderApp(fake);
    await userEvent.click(within(await openMenu("Done Co")).getByRole("menuitem", { name: "Archive" }));
    await waitFor(() => expect(archivedGroup()).toContainElement(within(nav()).getByRole("button", { name: /^Done Co/, hidden: true })));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(within(archivedGroup()).getByText("Archived (2)")).toBeInTheDocument();

    await userEvent.click(archivedGroup().querySelector("summary")!);
    const menu = await openMenu("Done Co");
    expect(within(menu).queryByRole("menuitem", { name: "Archive" })).toBeNull();
    await userEvent.click(within(menu).getByRole("menuitem", { name: "Unarchive" }));
    await waitFor(() => expect(archivedGroup()).not.toContainElement(within(nav()).getByRole("button", { name: /^Done Co/ })));
    expect(fake.calls).toEqual(expect.arrayContaining(["archiveProject", "unarchiveProject"]));
  });

  it("shows a refused archive's reason", async () => {
    const fake = world();
    renderApp(fake);
    // Work that arrived after the list was read: the service refuses.
    fake.archiveProject = async () => {
      throw new ApiError(409, "action_not_allowed", "Project done still has unfinished work: done-3 (Ready)");
    };
    await userEvent.click(within(await openMenu("Done Co")).getByRole("menuitem", { name: "Archive" }));
    expect(await within(nav()).findByRole("alert")).toHaveTextContent("still has unfinished work: done-3 (Ready)");
  });

  it("lists Done and Cancelled newest first, with plain counts, across every Project", async () => {
    renderApp(world());
    const done = await within(nav()).findByRole("button", { name: "Done 3" });
    expect(done.querySelector(".count")).not.toHaveClass("loud");
    expect(within(nav()).getByRole("button", { name: "Cancelled 1" })).toBeInTheDocument();

    await userEvent.click(done);
    await waitFor(() => expect(rowIds()).toEqual(["done-1", "old-1", "acme-3"]));
    await userEvent.click(within(nav()).getByRole("button", { name: /^Cancelled/ }));
    await waitFor(() => expect(rowIds()).toEqual(["done-2"]));
  });

  it("opens any Ticket by its URL, an Archived Project's too", async () => {
    renderApp(world(), "/?view=inbox&ticket=old-1");
    expect(await within(detail()).findByRole("heading", { name: "Shipped last week" })).toBeInTheDocument();
  });
});
