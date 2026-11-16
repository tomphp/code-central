// + New ▾ in Focus Mode: the real App on a WASM Core world, at ?view=focus.
// The same split as the sidebar's sits in the header beside Exit, plain, on
// every Focus screen; submitting from it closes the modal and leaves Focus
// Mode where it was, with a brief "Requirement added" or "Map added" note.
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { World } from "../api/core/core";
import { newWorld } from "../api/core/core";
import type { TicketSeed } from "../api/scenario";
import { allClearOn, focus, renderApp, scenario, settle } from "./worldTesting";

// FLAKY is the Failed Ticket, acme-1 being acme's Spec.
const FLAKY = "acme-2";

// world has Flaky build (acme-2) Failed, then Login page (acme-3) In
// Review, in that Needs Me order, with beta active and old archived. With
// items false it has none of them, and tickets are made instead.
async function world(items = true, tickets: TicketSeed[] = []) {
  const w = await newWorld();
  const s = scenario(w);
  await allClearOn(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  await s.project("old", "Old");
  await w.api.archiveProject("old");
  if (items) {
    await s.ticket("acme", { title: "Flaky build", state: "Failed" });
    await s.ticket("acme", { title: "Login page", state: "In Review" });
  }
  for (const t of tickets) await s.ticket("acme", t);
  return w;
}

// dealtWith takes the Failed Ticket out of Needs Me, as cancelling it
// elsewhere would.
async function dealtWith(w: World) {
  const s = scenario(w);
  await act(() => s.human(`${s.ticketPath("acme", FLAKY)}/cancel`, {}));
}

// gateNeedsMe holds the next count reads of Needs Me back until the
// returned release is called.
function gateNeedsMe(w: World, times = 1) {
  let release!: () => void;
  const gate = new Promise<void>((r) => (release = r));
  const needsMe = w.api.needsMe.bind(w.api);
  const gated = async () => {
    await gate;
    return needsMe();
  };
  const spy = vi.spyOn(w.api, "needsMe");
  if (times === 1) spy.mockImplementationOnce(gated);
  else spy.mockImplementation(gated);
  return release;
}

const refuseSubmit = (w: World) => vi.spyOn(w.api, "submitRequirement").mockRejectedValue(new Error("the Project is archived"));

const params = () => new URLSearchParams(window.location.search);
const heading = (name: string) => within(focus()).getByRole("heading", { name });
const newButton = () => within(focus()).getByRole("button", { name: "New requirement" });
const menuButton = () => within(focus()).getByRole("button", { name: "More to create" });
const item = (name: string) => within(screen.getByRole("menu")).getByRole("menuitem", { name });
// headerButtons are the buttons in Focus Mode's header, in order.
const headerButtons = () =>
  within(newButton().closest("header")!)
    .getAllByRole("button")
    .map((b) => b.textContent);
const skip = async (w: World) => {
  await userEvent.click(within(focus()).getByRole("button", { name: "Skip" }));
  await settle(w);
};

// added opens the modal for kind from + New ▾ and submits it.
async function added(w: World, kind: "Requirement" | "Map" | "Small Change") {
  if (kind === "Small Change") {
    await userEvent.click(menuButton());
    await userEvent.click(item("Small Change"));
    await submit(w, "New Small Change", "Submit small change");
  } else if (kind === "Requirement") {
    await userEvent.click(newButton());
    await submit(w, "New requirement", "Submit requirement");
  } else {
    await userEvent.click(menuButton());
    await userEvent.click(item("Map"));
    await submit(w, "New Map", "Submit map");
  }
  expect(screen.queryByRole("dialog")).toBeNull();
}

const note = () => within(focus()).queryByRole("status");

// later moves the world's clock and the timers on by ms, once React has
// run the effects that start them.
async function later(w: World, ms: number) {
  await act(() => new Promise((r) => setImmediate(r)));
  w.advance(ms);
  act(() => vi.advanceTimersByTime(ms));
}

// submit types text into the open dialog, submits it and settles.
async function submit(w: World, dialog: string, button: string, text = "Offline mode") {
  const modal = screen.getByRole("dialog", { name: dialog });
  await userEvent.type(within(modal).getByRole("textbox"), text);
  await userEvent.click(within(modal).getByRole("button", { name: button }));
  await settle(w);
  return modal;
}

describe("+ New ▾ in Focus Mode's header", () => {
  it("sits after the Project and N left, just before Exit, which stays at the far end", async () => {
    await renderApp(await world(), "/?view=focus&project=acme");
    heading("Flaky build");
    expect(headerButtons()).toEqual(["+ New", "▾", "Exit"]);
    const plus = newButton();
    const before = (text: string) => (within(plus.closest("header")!).getByText(text).compareDocumentPosition(plus) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
    expect(before("acme")).toBe(true);
    expect(before("2 left")).toBe(true);
  });

  it("is plain, like Exit, not primary", async () => {
    await renderApp(await world());
    expect(newButton()).not.toHaveClass("primary");
    expect(menuButton()).not.toHaveClass("primary");
  });

  it("shows on Loading…, a Focus Item and You skipped N", async () => {
    const w = await world();
    const release = gateNeedsMe(w);
    await renderApp(w);
    expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
    expect(headerButtons()).toEqual(["+ New", "▾", "Exit"]);

    release();
    await settle(w);
    heading("Flaky build");
    expect(headerButtons()).toEqual(["+ New", "▾", "Exit"]);

    await skip(w);
    heading("Login page");
    await skip(w);
    expect(within(focus()).getByText("You skipped 2, go through them again?")).toBeInTheDocument();
    expect(headerButtons()).toEqual(["+ New", "▾", "Exit"]);
  });

  it("shows on All clear", async () => {
    await renderApp(await world(false));
    within(focus()).getByRole("region", { name: "All clear" });
    expect(headerButtons()).toEqual(["+ New", "▾", "Exit"]);
  });

  it("shows on the Activity Dashboard", async () => {
    const w = await world(false);
    await w.api.setOptions({ focus_clear: "activity_dashboard" });
    await renderApp(w);
    expect(within(focus()).getByRole("region", { name: "Activity Dashboard" })).toBeInTheDocument();
    expect(headerButtons()).toEqual(["+ New", "▾", "Exit"]);
  });

  it("opens New requirement from + New, and New Map from the ▾ menu", async () => {
    await renderApp(await world());
    await userEvent.click(newButton());
    expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));

    await userEvent.click(menuButton());
    expect(screen.getByRole("menu", { name: "More to create" })).toBeVisible();
    await userEvent.click(item("Map"));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.getByRole("dialog", { name: "New Map" })).toBeVisible();
    expect(within(screen.getByRole("dialog")).queryByRole("checkbox")).toBeNull();
  });

  it("opens no modal on a key: there's no shortcut for + New", async () => {
    await renderApp(await world());
    heading("Flaky build");
    await userEvent.keyboard("n");
    await userEvent.keyboard("N");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  describe("by keyboard", () => {
    const focusMenuButton = () => {
      newButton();
      menuButton().focus();
    };

    it("opens with ↓ on the first item, and moves with the arrows, wrapping at either end", async () => {
      await renderApp(await world());
      focusMenuButton();
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
    });

    it("closes on Esc and returns focus to ▾", async () => {
      await renderApp(await world());
      focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{Escape}");
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveFocus();
    });

    it("closes when Tab leaves the menu", async () => {
      await renderApp(await world());
      focusMenuButton();
      await userEvent.keyboard("{ArrowDown}");
      await userEvent.tab();
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    });
  });

  describe("submitting", () => {
    it.each([
      ["+ New", "New requirement", "Submit requirement", "submitRequirement"],
      ["Map", "New Map", "Submit map", "submitMap"],
      ["Small Change", "New Small Change", "Submit small change", "submitSmallChange"],
    ])("from %s closes the modal and leaves Focus Mode as it was, skipped list and all", async (from, dialog, button, call) => {
      const w = await world();
      await renderApp(w, "/?view=focus&project=acme");
      heading("Flaky build");
      await skip(w);
      heading("Login page");

      if (from === "+ New") await userEvent.click(newButton());
      else {
        await userEvent.click(menuButton());
        await userEvent.click(item(from));
      }
      await submit(w, dialog, button);

      expect(w.calls).toContain(call);
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(within(focus()).getByRole("heading", { name: "Login page" })).toBeInTheDocument();
      expect(window.location.search).toBe("?view=focus&project=acme");
      expect(params().get("requirement")).toBeNull();

      // Flaky build is still skipped: skipping Login page leaves only the two.
      await skip(w);
      expect(within(focus()).getByText("You skipped 2, go through them again?")).toBeInTheDocument();
    });

    it("leaves a half-written answer on the Focus Item as it was", async () => {
      const w = await world(false);
      const s = scenario(w);
      const id = await s.ticket("acme", { title: "Pick a colour" });
      await s.askRound("acme", id, [{ title: "Which colour?", body: "Red or blue" }]);
      await renderApp(w);
      heading("Which colour?");
      await userEvent.type(within(focus()).getByRole("textbox", { name: "Your answer" }), "Blue");

      await userEvent.click(newButton());
      await submit(w, "New requirement", "Submit requirement");
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(within(focus()).getByRole("textbox", { name: "Your answer" })).toHaveValue("Blue");
    });

    it("submits from a modal opened while Loading…, once the Projects arrive", async () => {
      const w = await world(false);
      const release = gateNeedsMe(w);
      await renderApp(w);
      await userEvent.click(newButton());
      release();
      await settle(w);
      within(focus()).getByRole("region", { name: "All clear" });

      await submit(w, "New requirement", "Submit requirement");
      expect(w.calls).toContain("submitRequirement");
      expect(screen.queryByRole("dialog")).toBeNull();
    });

    it("keeps the modal open with its error when the submit is refused", async () => {
      const w = await world();
      refuseSubmit(w);
      await renderApp(w, "/?view=focus&project=acme");
      heading("Flaky build");
      await userEvent.click(newButton());
      const modal = await submit(w, "New requirement", "Submit requirement");

      expect(within(modal).getByText("the Project is archived")).toBeInTheDocument();
      expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
      expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
    });
  });

  describe("which Project", () => {
    it("preselects the Project Focus Mode is filtered to, without asking", async () => {
      const w = await world();
      await renderApp(w, "/?view=focus&project=beta");
      await userEvent.click(newButton());
      expect(within(screen.getByRole("dialog")).queryByRole("combobox")).toBeNull();
      await submit(w, "New requirement", "Submit requirement");
      expect(screen.queryByRole("dialog")).toBeNull();
      expect((await w.api.findSpecs("beta")).map((s) => s.title)).toContain("Offline mode");
    });

    it("asks which active Project when Focus Mode isn't filtered to one, leaving out the Archived", async () => {
      await renderApp(await world());
      await userEvent.click(newButton());
      const select = within(screen.getByRole("dialog")).getByRole("combobox", { name: "Project" });
      expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["Acme", "Beta"]);
    });
  });

  describe("the added note", () => {
    beforeEach(() => vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true, advanceTimeDelta: 1 }));
    afterEach(() => vi.useRealTimers());

    it.each([["Requirement"], ["Map"], ["Small Change"]] as const)("reads %s added after a submit, and goes after 4 s", async (kind) => {
      const w = await world();
      await renderApp(w);
      heading("Flaky build");
      await added(w, kind);

      expect(within(focus()).getByRole("status")).toHaveTextContent(`${kind} added`);
      await later(w, 3900);
      expect(note()).toHaveTextContent(`${kind} added`);
      await later(w, 100);
      expect(note()).toBeNull();
    });

    it("sits where Dealt with elsewhere does, and looks like it", async () => {
      const w = await world();
      await renderApp(w);
      heading("Flaky build");
      await dealtWith(w);
      const elsewhere = within(focus()).getByRole("status");
      expect(elsewhere).toHaveTextContent("Dealt with elsewhere");
      const placement = (el: HTMLElement) => [el.parentElement, el.previousElementSibling, el.className];
      const elsewherePlacement = placement(elsewhere);

      await added(w, "Requirement");
      const addedNote = within(focus()).getByText("Requirement added");
      expect(placement(addedNote)).toEqual(elsewherePlacement);
    });

    it("shows on All clear", async () => {
      const w = await world(false);
      await renderApp(w);
      within(focus()).getByRole("region", { name: "All clear" });
      await added(w, "Requirement");
      expect(within(focus()).getByRole("status")).toHaveTextContent("Requirement added");
    });

    it("shows on the Activity Dashboard", async () => {
      const w = await world(false);
      await w.api.setOptions({ focus_clear: "activity_dashboard" });
      await renderApp(w);
      within(focus()).getByRole("region", { name: "Activity Dashboard" });
      await added(w, "Map");
      expect(within(focus()).getByRole("status")).toHaveTextContent("Map added");
    });

    it("shows on You skipped N", async () => {
      const w = await world();
      await renderApp(w);
      heading("Flaky build");
      await skip(w);
      heading("Login page");
      await skip(w);
      within(focus()).getByText("You skipped 2, go through them again?");
      await added(w, "Requirement");
      expect(within(focus()).getByRole("status")).toHaveTextContent("Requirement added");
    });

    it("shows on Loading…", async () => {
      const w = await world();
      const release = gateNeedsMe(w, Infinity);
      await renderApp(w, "/?view=focus&project=acme");
      await added(w, "Requirement");
      expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
      expect(note()).toHaveTextContent("Requirement added");
      release();
    });

    it("stays when the item on screen changes, until its own 4 s are up", async () => {
      const w = await world();
      await renderApp(w);
      heading("Flaky build");
      await added(w, "Requirement");
      await later(w, 2000);
      await skip(w);
      heading("Login page");
      expect(note()).toHaveTextContent("Requirement added");
      await later(w, 2000);
      expect(note()).toBeNull();
    });

    it("replaces Dealt with elsewhere, and starts its own 4 s", async () => {
      const w = await world();
      await renderApp(w);
      heading("Flaky build");
      await dealtWith(w);
      expect(within(focus()).getByRole("status")).toHaveTextContent("Dealt with elsewhere");
      await later(w, 3000);

      await added(w, "Map");
      expect(within(focus()).getAllByRole("status").map((n) => n.textContent)).toEqual(["Map added"]);
      await later(w, 3000);
      expect(note()).toHaveTextContent("Map added");
      await later(w, 1000);
      expect(note()).toBeNull();
    });

    it("is replaced by Dealt with elsewhere, which starts its own 4 s", async () => {
      const w = await world();
      await renderApp(w);
      heading("Flaky build");
      await added(w, "Requirement");
      await later(w, 3000);

      await dealtWith(w);
      heading("Login page");
      within(focus()).getByText("Dealt with elsewhere");
      expect(within(focus()).getAllByRole("status").map((n) => n.textContent)).toEqual(["Dealt with elsewhere"]);
      await later(w, 3000);
      expect(note()).toHaveTextContent("Dealt with elsewhere");
      await later(w, 1000);
      expect(note()).toBeNull();
    });

    it("isn't replaced when the last item goes elsewhere to All clear, which shows no Dealt with elsewhere", async () => {
      const w = await world(false, [{ title: "Flaky build", state: "Failed" }]);
      await renderApp(w);
      heading("Flaky build");
      await added(w, "Requirement");
      await dealtWith(w);
      within(focus()).getByRole("region", { name: "All clear" });
      expect(note()).toHaveTextContent("Requirement added");
    });

    it("isn't shown when the submit is refused", async () => {
      const w = await world();
      refuseSubmit(w);
      await renderApp(w);
      heading("Flaky build");
      await userEvent.click(newButton());
      const modal = await submit(w, "New requirement", "Submit requirement");
      within(modal).getByText("the Project is archived");
      expect(note()).toBeNull();
    });
  });
});
