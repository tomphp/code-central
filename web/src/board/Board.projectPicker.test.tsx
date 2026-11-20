// The Project picker as the Board shows it: its order, marks, keyboard and
// closing. The other views choose through the same control.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { newWorld } from "../api/core/core";
import { openProjectPicker, projectChoices, projectPicker } from "../testPickers";
import { scenario } from "../worldTesting";
import { board, renderApp, type Fake } from "./Board.testutil";

beforeEach(() => localStorage.clear());

// several has three current Projects, named out of Key order and in mixed
// case, one with a Factory online, and an Archived one.
async function several(): Promise<Fake> {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("a", "banana");
  await s.project("b", "Apple");
  await s.project("c", "cherry");
  await s.project("d", "Old");
  await s.reportHeartbeat("b", { factory: "code-central-factory", host: "laptop", state: "online", interval_seconds: 30 });
  await w.api.archiveProject("d");
  return { w, s, ids: {} as Fake["ids"] };
}

const listbox = () => screen.getByRole("listbox", { name: "Projects" });
const selected = () => within(listbox()).getByRole("option", { selected: true });
const active = () => listbox().getAttribute("aria-activedescendant");
const activeName = () => document.getElementById(active() ?? "")?.textContent?.replace("✓", "");

describe("the Project picker's list", () => {
  it("lists All Projects, the current Projects by display name, then the Archived ones under their heading", async () => {
    await renderApp(await several(), "/?view=board");
    const list = await openProjectPicker(board());
    expect(projectChoices(list)).toEqual(["All Projects", "Apple", "banana", "cherry", "Old"]);
    const archived = within(list).getByRole("group", { name: "Archived" });
    expect(within(archived).getAllByRole("option")).toHaveLength(1);
    expect(within(archived).getByRole("option")).toHaveTextContent("Old");
  });

  it("breaks a tie in display name by Key", async () => {
    const f = await several();
    await f.s.project("a2", "Apple");
    await f.s.project("a1", "Apple");
    await renderApp(f, "/?view=board");
    expect(projectChoices(await openProjectPicker(board())).slice(0, 5)).toEqual(["All Projects", "Apple", "Apple", "Apple", "banana"]);
    // The Apples are a1, a2 then b: the second one chosen is a2.
    const apples = within(listbox()).getAllByRole("option").slice(1, 4);
    await userEvent.click(apples[1]);
    expect(window.location.search).toBe("?view=board&project=a2");
  });

  it("leaves All Projects out with a Sole Project", async () => {
    const w = await newWorld();
    await scenario(w).project("solo", "Solo");
    await renderApp({ w, s: scenario(w), ids: {} as Fake["ids"] }, "/?view=board");
    expect(projectChoices(await openProjectPicker(board()))).toEqual(["Solo"]);
  });

  it("shows a Project from the address that isn't known last, as a bare entry", async () => {
    await renderApp(await several(), "/?view=board&project=ghost");
    expect(projectPicker(board())).toHaveTextContent("ghost");
    const list = await openProjectPicker(board());
    expect(projectChoices(list)).toEqual(["All Projects", "Apple", "banana", "cherry", "Old", "ghost"]);
    const bare = within(list).getByRole("option", { name: "ghost" });
    expect(within(bare).queryByRole("img")).toBeNull();
  });

  it("selects the chosen option, and the closed button names it", async () => {
    await renderApp(await several(), "/?view=board&project=c");
    expect(projectPicker(board())).toHaveAccessibleName(/^Projects.*cherry$/);
    await openProjectPicker(board());
    expect(selected()).toHaveTextContent("cherry");
  });

  it("labels each current Project's mark with its Factory Status and gives no Archived one a mark", async () => {
    await renderApp(await several(), "/?view=board");
    const list = await openProjectPicker(board());
    const marks = (name: string) => within(within(list).getAllByRole("option").find((o) => o.textContent?.startsWith(name))!).queryAllByRole("img");
    expect(marks("All Projects")).toHaveLength(0);
    expect(marks("Apple")[0]).toHaveAccessibleName("Factory Status: Online");
    expect(marks("banana")[0]).toHaveAccessibleName("Factory Status: Offline");
    expect(marks("Old")).toHaveLength(0);
  });

  it("shows the chosen Project's mark on the closed button, and none on All Projects", async () => {
    await renderApp(await several(), "/?view=board&project=b");
    expect(within(projectPicker(board())).getByRole("img", { name: "Factory Status: Online" })).toBeInTheDocument();
  });

  it("follows a Factory Status change", async () => {
    const f = await several();
    await renderApp(f, "/?view=board&project=a");
    expect(within(projectPicker(board())).getByRole("img")).toHaveAccessibleName("Factory Status: Offline");
    await f.s.reportHeartbeat("a", { factory: "code-central-factory", host: "laptop", state: "online", interval_seconds: 30 });
    await import("../worldTesting").then((m) => m.settle(f.w));
    expect(within(projectPicker(board())).getByRole("img")).toHaveAccessibleName("Factory Status: Online");
  });
});

describe("the Project picker's keys", () => {
  it("opens on the chosen entry and moves with Up and Down, Home and End, skipping the heading", async () => {
    await renderApp(await several(), "/?view=board&project=c");
    await openProjectPicker(board());
    expect(listbox()).toHaveFocus();
    expect(activeName()).toBe("cherry");
    await userEvent.keyboard("{ArrowDown}");
    expect(activeName()).toBe("Old");
    await userEvent.keyboard("{ArrowDown}");
    expect(activeName()).toBe("Old");
    await userEvent.keyboard("{Home}");
    expect(activeName()).toBe("All Projects");
    await userEvent.keyboard("{ArrowUp}");
    expect(activeName()).toBe("All Projects");
    await userEvent.keyboard("{End}");
    expect(activeName()).toBe("Old");
    await userEvent.keyboard("{ArrowUp}");
    expect(activeName()).toBe("cherry");
  });

  it("chooses with Enter, closes and returns focus to the button", async () => {
    await renderApp(await several(), "/?view=board");
    await openProjectPicker(board());
    await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
    expect(window.location.search).toBe("?view=board&project=a");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(projectPicker(board())).toHaveFocus();
    expect(projectPicker(board())).toHaveTextContent("banana");
  });

  it("closes without a change when the chosen entry is chosen again", async () => {
    await renderApp(await several(), "/?view=board&project=a");
    await openProjectPicker(board());
    await userEvent.keyboard("{Enter}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(window.location.search).toBe("?view=board&project=a");
  });

  it("closes on Esc without choosing, returning focus to the button", async () => {
    await renderApp(await several(), "/?view=board&project=a");
    await openProjectPicker(board());
    await userEvent.keyboard("{ArrowDown}{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(projectPicker(board())).toHaveFocus();
    expect(window.location.search).toBe("?view=board&project=a");
  });

  it("closes on a click outside without choosing", async () => {
    await renderApp(await several(), "/?view=board&project=a");
    await openProjectPicker(board());
    await userEvent.click(document.body);
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(window.location.search).toBe("?view=board&project=a");
  });
});
