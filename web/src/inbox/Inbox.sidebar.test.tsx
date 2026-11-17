// Inbox layout tests for the sidebar: the top row (+ New and Focus) and the Admin
// section.
import { screen, within, act, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { Api } from "../api/api";
import { scenario, settle } from "../worldTesting";
import { world, renderApp, list, nav, params } from "./Inbox.testutil";

describe("the sidebar's top row: + New and Focus", () => {
  const menuButton = () => within(nav()).getByRole("button", { name: "More to create" });
  const item = (name: string) => within(screen.getByRole("menu")).getByRole("menuitem", { name });

  it("shows + New, its ▾ and Focus at the top of the sidebar", async () => {
    await renderApp(await world());
    const newButton = within(nav()).getByRole("button", { name: "New requirement" });
    expect(newButton).toHaveTextContent("+ New");
    expect(menuButton()).toHaveAttribute("aria-haspopup", "menu");
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    expect(within(nav()).getByRole("button", { name: "Focus" })).toHaveTextContent(/^Focus$/);
    expect(within(nav()).queryByRole("button", { name: "New Map" })).toBeNull();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("shows + New and its ▾ as primary", async () => {
    await renderApp(await world());
    expect(within(nav()).getByRole("button", { name: "New requirement" })).toHaveClass("primary");
    expect(menuButton()).toHaveClass("primary");
  });

  it("opens the New requirement dialog from + New", async () => {
    await renderApp(await world());
    await userEvent.click(within(nav()).getByRole("button", { name: "New requirement" }));
    expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens a menu of Requirement, Map and Small Change from ▾, each with its hint", async () => {
    await renderApp(await world());
    await userEvent.click(menuButton());
    expect(menuButton()).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("menu", { name: "More to create" })).toBeVisible();
    const items = within(screen.getByRole("menu")).getAllByRole("menuitem");
    expect(items.map((i) => i.textContent)).toEqual([
      expect.stringContaining("Requirement"),
      expect.stringContaining("Map"),
      expect.stringContaining("Small Change"),
    ]);
    expect(item("Requirement")).toHaveAccessibleDescription(
      "Grilled into a Spec and Tickets",
    );
    expect(item("Map")).toHaveAccessibleDescription(
      "Charts the decisions to get somewhere",
    );
    expect(item("Small Change")).toHaveAccessibleDescription("One Ticket, built and landed as it is");
  });

  it("opens Small Change's dialog from the menu, and submits to the new Ticket", async () => {
    const w = await world();
    await renderApp(w, "/?project=acme");
    await userEvent.click(menuButton());
    await userEvent.click(item("Small Change"));

    expect(screen.queryByRole("menu")).toBeNull();
    const modal = screen.getByRole("dialog", { name: "New Small Change" });
    const box = within(modal).getByRole("textbox", { name: "Small Change" });
    expect(box).toHaveAttribute("placeholder", "Describe the change you want. The first line becomes the Ticket's title.");
    expect(within(modal).getByText("Creates a Ready Ticket on the Small Changes Spec. A Factory builds it and it lands on its own.")).toBeVisible();
    const submit = within(modal).getByRole("button", { name: "Submit small change" });
    expect(submit).toBeDisabled();
    await userEvent.type(box, "   ");
    expect(submit).toBeDisabled();
    await userEvent.type(box, "Rename the button");
    await userEvent.click(submit);

    await settle(w);
    expect(w.calls).toContain("submitSmallChange");
    expect(screen.queryByRole("dialog")).toBeNull();
    const created = (await w.api.findTickets("acme")).find((t) => t.title === "Rename the button")!;
    expect(params().get("ticket")).toBe(created.id);
    expect(params().get("requirement")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("asks which Project for a Small Change when several are open and none is filtered", async () => {
    await renderApp(await world());
    await userEvent.click(menuButton());
    await userEvent.click(item("Small Change"));
    expect(within(screen.getByRole("dialog")).getByRole("combobox")).toBeVisible();
  });

  it("keeps the text and shows the error when a Small Change is refused", async () => {
    // Core would take this, so the refusal is the service's, injected.
    const w = await world();
    const refusing: Api = {
      ...w.api,
      submitSmallChange: async () => {
        throw new Error("the Project is archived");
      },
    };
    await renderApp(w, "/?project=acme", refusing);
    await userEvent.click(menuButton());
    await userEvent.click(item("Small Change"));
    const modal = screen.getByRole("dialog");
    await userEvent.type(within(modal).getByRole("textbox"), "Rename the button");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit small change" }));
    await settle(w);
    expect(within(modal).getByText("the Project is archived")).toBeVisible();
    expect(within(modal).getByRole("textbox")).toHaveValue("Rename the button");
  });

  it.each([
    ["Requirement", "New requirement", "Submit requirement", "submitRequirement"],
    ["Map", "New Map", "Submit map", "submitMap"],
  ])("opens %s's dialog from the menu, which stays visible once the menu closes, and submits to the new Spec", async (name, dialog, submit, call) => {
    const w = await world();
    await renderApp(w, "/?project=acme");
    await userEvent.click(menuButton());
    await userEvent.click(item(name));

    expect(screen.queryByRole("menu")).toBeNull();
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    const modal = screen.getByRole("dialog", { name: dialog });
    expect(modal).toBeVisible();
    await userEvent.type(within(modal).getByRole("textbox"), "Offline mode");
    await userEvent.click(within(modal).getByRole("button", { name: submit }));

    await settle(w);
    expect(w.calls).toContain(call);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(params().get("list")).toBe("requirements");
    expect(params().get("requirement")).toBe("acme-6");
    // The note is Focus Mode's: the sidebar navigates instead.
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("closes the menu on Esc and on a click outside it", async () => {
    await renderApp(await world());
    await userEvent.click(menuButton());
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");

    await userEvent.click(menuButton());
    expect(screen.getByRole("menu")).toBeInTheDocument();
    await userEvent.click(list());
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("closes the menu when + New is clicked instead", async () => {
    await renderApp(await world());
    await userEvent.click(menuButton());
    await userEvent.click(within(nav()).getByRole("button", { name: "New requirement" }));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
  });

  it("toggles the menu shut from ▾", async () => {
    await renderApp(await world());
    await userEvent.click(menuButton());
    await userEvent.click(menuButton());
    expect(screen.queryByRole("menu")).toBeNull();
  });

  describe("by keyboard", () => {
    const focusMenuButton = async () => (menuButton()).focus();

    it("opens with ↓ on the first item, and moves with the arrows, wrapping at either end", async () => {
      await renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}");
      expect(menuButton()).toHaveAttribute("aria-expanded", "true");
      expect(item("Requirement")).toHaveFocus();

      await userEvent.keyboard("{ArrowDown}");
      expect(item("Map")).toHaveFocus();
      await userEvent.keyboard("{ArrowDown}");
      expect(item("Small Change")).toHaveFocus();
      await userEvent.keyboard("{ArrowDown}");
      expect(item("Requirement")).toHaveFocus();
      await userEvent.keyboard("{ArrowUp}");
      expect(item("Small Change")).toHaveFocus();
      await userEvent.keyboard("{ArrowUp}");
      expect(item("Map")).toHaveFocus();
    });

    it.each([["{Enter}"], [" "]])("opens with %s on the first item", async (key) => {
      await renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard(key);
      expect(item("Requirement")).toHaveFocus();
    });

    it.each([["{Enter}"], [" "]])("picks the focused item with %s: the menu closes and its dialog opens", async (key) => {
      await renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}");
      await userEvent.keyboard(key);
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
      expect(screen.getByRole("dialog", { name: "New Map" })).toBeVisible();
    });

    it("leaves the arrows to the dialog an item opened", async () => {
      await renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
      const text = within(screen.getByRole("dialog", { name: "New Map" })).getByRole("textbox");
      await userEvent.type(text, "Offline{Enter}mode");
      // fireEvent is false when a handler cancelled the key, as the caret would stay put.
      expect(fireEvent.keyDown(text, { key: "ArrowUp" })).toBe(true);
      expect(fireEvent.keyDown(text, { key: "ArrowDown" })).toBe(true);
      expect(text).toHaveFocus();
    });

    it("closes on Esc and returns focus to ▾", async () => {
      await renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{Escape}");
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
      expect(menuButton()).toHaveFocus();
    });

    it("closes when Tab leaves the menu", async () => {
      await renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}");
      await userEvent.tab();
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    });
  });

  it("enters Focus Mode from Focus", async () => {
    await renderApp(await world());
    await userEvent.click(within(nav()).getByRole("button", { name: "Focus" }));
    expect(params().get("view")).toBe("focus");
  });
});

describe("the Admin section", () => {
  const follows = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_PRECEDING) !== 0;

  it("keeps the title row to the heading, with no Usage or Options links", async () => {
    await renderApp(await world());
    const title = within(nav()).getByRole("heading", { level: 1, name: "Code Central" });
    expect(within(title.parentElement!).queryByRole("link")).toBeNull();
  });

  it("follows View's last State filter and precedes the Archived Projects", async () => {
    const w = await world();
    await scenario(w).project("old", "Old");
    await w.api.archiveProject("old");
    await renderApp(w);
    const admin = within(nav()).getByRole("heading", { level: 2, name: "Admin" });
    const lastState = within(nav()).getByRole("button", { name: /^Cancelled/ });
    const archived = within(nav()).getByText(/^Archived/);
    expect(follows(admin, lastState)).toBe(true);
    expect(follows(archived, admin)).toBe(true);

    const ci = within(nav()).getByRole("link", { name: "CI" });
    const usage = within(nav()).getByRole("link", { name: "Usage" });
    const options = within(nav()).getByRole("link", { name: "Options" });
    expect(follows(ci, admin)).toBe(true);
    expect(follows(usage, ci)).toBe(true);
    expect(follows(options, usage)).toBe(true);
    expect(follows(archived, options)).toBe(true);
  });

  it("links Usage and Options with real addresses that return to the Inbox", async () => {
    await renderApp(await world(), "/?view=inbox&project=acme&state=Ready");
    expect(within(nav()).getByRole("link", { name: "Usage" })).toHaveAttribute("href", "?view=usage&back=view%3Dinbox%26project%3Dacme%26state%3DReady");
    expect(within(nav()).getByRole("link", { name: "Options" })).toHaveAttribute("href", "?view=options&from=inbox&project=acme&state=Ready");
    expect(within(nav()).getByRole("link", { name: "Usage" })).not.toHaveAttribute("aria-current");
  });

  it("opens Usage and Options", async () => {
    const w = await world();
    await renderApp(w);
    await userEvent.click(within(nav()).getByRole("link", { name: "Usage" }));
    await settle(w);
    expect(screen.getByRole("main", { name: "Usage" })).toBeInTheDocument();

    // jsdom fires history.back()'s popstate on a later task, which settle doesn't wait for.
    window.history.replaceState(null, "", "/");
    act(() => window.dispatchEvent(new PopStateEvent("popstate")));
    await settle(w);
    await userEvent.click(within(nav()).getByRole("link", { name: "Options" }));
    await settle(w);
    expect(screen.getByRole("main", { name: "Options" })).toBeInTheDocument();
  });
});
