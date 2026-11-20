// Usage view tests: opening and leaving Usage, scope and range, and the tabs.
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { Api } from "../api/api";
import { settle } from "../worldTesting";
import { projects, calls, world, renderApp, usage, scope, chooseScope, range, ops, tab, back, openTab } from "./Usage.testutil";
import { openProjectPicker, projectChoices } from "../testPickers";

describe("opening and leaving Usage", () => {
  it("opens from the Inbox's Admin section on All Projects and 7 days, and returns to the Inbox with its filter and Ticket", async () => {
    const { w, ticket } = await projects();
    await renderApp(w, `/?view=inbox&project=acme&state=Ready&ticket=${ticket}`);
    const nav = screen.getByRole("navigation", { name: "Filters" });
    await userEvent.click(within(nav).getByRole("link", { name: "Usage" }));

    expect(usage()).toBeInTheDocument();
    expect(scope()).toHaveAccessibleName("Projects All Projects");
    expect(range()).toHaveValue("7");
    expect(new URLSearchParams(window.location.search).get("project")).toBeNull();

    await userEvent.click(within(usage()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe(`?view=inbox&project=acme&state=Ready&ticket=${ticket}`);
    await settle(world);
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("returns to Focus Mode with its Project when the address names it", async () => {
    await renderApp((await projects()).w, "/?view=usage&back=view%3Dfocus%26project%3Dacme");
    expect(scope()).toHaveTextContent("All Projects");
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus&project=acme");
    expect(screen.getByRole("main", { name: "Focus Mode" })).toBeInTheDocument();
  });

  it("returns to the Inbox when opened straight from the address", async () => {
    await renderApp((await projects()).w, "/?view=usage");
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox");
  });

  it("returns to where the human was on the browser's Back", async () => {
    await renderApp((await projects()).w, "/?view=inbox&project=acme");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Usage" }));
    expect(usage()).toBeInTheDocument();

    await back();
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
    expect(window.location.search).toBe("?view=inbox&project=acme");
  });
});

describe("scope and range", () => {
  it("lists All Projects and every Project, Archived ones under their heading", async () => {
    await renderApp((await projects()).w, "/?view=usage");
    await settle(world);
    const listbox = await openProjectPicker(usage());
    expect(projectChoices(listbox)).toEqual(["All Projects", "Acme", "Beta", "Old"]);
    expect(within(within(listbox).getByRole("group", { name: "Archived" })).getAllByRole("option")).toHaveLength(1);
    await userEvent.keyboard("{Escape}");
    expect(
      within(range())
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Last 7 days", "Last 30 days", "Last 90 days"]);
  });

  it("puts a chosen Project and range in the address, and a reload keeps them", async () => {
    const { w, s, ticket } = await projects();
    // Twenty days ago: only in the 30 and 90 day ranges.
    await calls(s, "acme", "claim", 1, { ticket: "acme-99" });
    w.advance(20 * 24 * 60 * 60_000);
    await calls(s, "acme", "get_ticket", 1, { ticket });
    await calls(s, "beta", "find_tickets", 1, {});
    const { unmount } = await renderApp(w, "/?state=Ready");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Usage" }));
    await openTab("Code Central calls");
    await settle(world);
    expect(ops()).toEqual(["find_tickets", "get_ticket"]);

    await settle(world);
    await chooseScope("Acme");
    expect(window.location.search).toBe("?view=usage&project=acme&tab=calls&back=view%3Dinbox%26state%3DReady");
    await settle(world);
    expect(ops()).toEqual(["get_ticket"]);

    await userEvent.selectOptions(range(), "Last 30 days");
    expect(window.location.search).toBe("?view=usage&project=acme&days=30&tab=calls&back=view%3Dinbox%26state%3DReady");
    await settle(world);
    expect(ops()).toEqual(["claim", "get_ticket"]);

    unmount();
    await renderApp(w, window.location.search);
    expect(scope()).toHaveTextContent("Acme");
    expect(range()).toHaveValue("30");
    expect(tab("Code Central calls")).toHaveAttribute("aria-selected", "true");
    await settle(world);
    expect(ops()).toEqual(["claim", "get_ticket"]);
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&state=Ready");
  });

  it("goes back to All Projects and 7 days, leaving them out of the address", async () => {
    await renderApp((await projects()).w, "/?view=usage&project=acme&days=90");
    expect(scope()).toHaveTextContent("Acme");
    await chooseScope("All Projects");
    await userEvent.selectOptions(range(), "Last 7 days");
    expect(window.location.search).toBe("?view=usage");
  });

  it("loads when opened and when the scope or range changes, never on a live notice", async () => {
    const { w, s } = await projects();
    await renderApp(w, "/?view=usage");
    const reads = () => w.calls.filter((c) => c === "usage").length;
    await settle(world);
    expect(reads()).toBe(1);

    await calls(s, "acme", "find_tickets", 1, {});
    await act(() => s.ticket("acme", { title: "Another" }));
    act(() => w.dropStreams());
    await settle(w);
    expect(reads()).toBe(1);
    expect(within(usage()).getByText(/No token usage/)).toBeInTheDocument();

    await userEvent.selectOptions(range(), "Last 30 days");
    await settle(w);
    expect(reads()).toBe(2);
    await openTab("Code Central calls");
    expect(ops()).toEqual(["find_tickets"]);
  });
});

describe("tabs", () => {
  async function withUsage() {
    const { w, s, ticket } = await projects();
    await calls(s, "acme", "get_ticket", 2, { ticket });
    await calls(s, "beta", "find_tickets", 1, {});
    return w;
  }
  const tabs = () => within(usage()).getAllByRole("tab");
  const selected = () => within(usage()).getByRole("tab", { selected: true });

  it("opens on Token usage, the tabs in order, with no tab in the address", async () => {
    await renderApp(await withUsage(), "/?view=usage");
    expect(tabs().map((t) => t.textContent)).toEqual(["Token usage", "Context Size", "Code Central calls", "Storage"]);
    expect(selected()).toHaveTextContent("Token usage");
    expect(within(usage()).getByRole("tabpanel")).toHaveAccessibleName("Token usage");
    expect(window.location.search).toBe("?view=usage");
  });

  it("opens on Token usage for a tab it doesn't know, and writes it with no tab", async () => {
    await renderApp(await withUsage(), "/?view=usage&tab=nonsense");
    expect(selected()).toHaveTextContent("Token usage");
    expect(window.location.search).toBe("?view=usage");
  });

  it("puts the open tab in the address, leaving Token usage out, and a reload keeps it", async () => {
    const w = await withUsage();
    const { unmount } = await renderApp(w, "/?view=usage");
    await openTab("Context Size");
    expect(window.location.search).toBe("?view=usage&tab=context");
    await openTab("Code Central calls");
    expect(window.location.search).toBe("?view=usage&tab=calls");
    await settle(world);
    expect(ops()).toEqual(["get_ticket", "find_tickets"]);

    unmount();
    await renderApp(w, window.location.search);
    expect(selected()).toHaveTextContent("Code Central calls");
    await settle(world);
    expect(ops()).toEqual(["get_ticket", "find_tickets"]);

    await openTab("Token usage");
    expect(window.location.search).toBe("?view=usage");
  });

  it("moves between the tabs with the arrow keys", async () => {
    await renderApp(await withUsage(), "/?view=usage");
    tab("Token usage").focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(selected()).toHaveTextContent("Context Size");
    expect(window.location.search).toBe("?view=usage&tab=context");
    await userEvent.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(selected()).toHaveTextContent("Storage");
  });

  it("goes back through the tabs on the browser's Back", async () => {
    await renderApp(await withUsage(), "/?view=inbox");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Usage" }));
    await openTab("Code Central calls");
    await openTab("Context Size");

    await back();
    expect(selected()).toHaveTextContent("Code Central calls");
    await back();
    expect(selected()).toHaveTextContent("Token usage");
    await back();
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("keeps the scope and range across a switch, without reading Usage again", async () => {
    const w = await withUsage();
    await renderApp(w, "/?view=usage&project=acme&days=30");
    const reads = () => w.calls.filter((c) => c === "usage").length;
    await settle(world);
    expect(reads()).toBe(1);
    await openTab("Code Central calls");
    await settle(world);
    expect(ops()).toEqual(["get_ticket"]);
    await openTab("Context Size");
    await openTab("Token usage");

    expect(window.location.search).toBe("?view=usage&project=acme&days=30");
    expect(scope()).toHaveTextContent("Acme");
    expect(range()).toHaveValue("30");
    expect(reads()).toBe(1);
  });

  it("shows the error banner below the tab strip", async () => {
    const w = await withUsage();
    const down: Api = {
      ...w.api,
      usage: async () => {
        throw new Error("Usage is down");
      },
    };
    await renderApp(w, "/?view=usage&tab=calls", down);
    const alert = within(usage()).getByRole("alert");
    expect(alert).toHaveTextContent("Usage is down");
    expect(within(usage()).getByRole("tablist").compareDocumentPosition(alert) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(selected()).toHaveTextContent("Code Central calls");
  });
});
