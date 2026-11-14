// + New ▾ in Focus Mode: the real App on a WASM Core world, at ?view=focus.
// The same split as the sidebar's sits in the header beside Exit, plain, on
// every Focus screen; submitting from it closes the modal and leaves Focus
// Mode where it was, with a brief "Requirement added" or "Map added" note.
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { World } from "../api/core/core";
import { newWorld } from "../api/core/core";
import { scenario, type TicketSeed } from "../api/scenario";
import { allClearOn, focus, renderApp } from "./worldTesting";

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
const heading = (name: string) => within(focus()).findByRole("heading", { name });
const newButton = () => within(focus()).findByRole("button", { name: "New requirement" });
const menuButton = () => within(focus()).getByRole("button", { name: "More to create" });
const item = (name: string) => within(screen.getByRole("menu")).getByRole("menuitem", { name });
// headerButtons are the buttons in Focus Mode's header, in order.
const headerButtons = async () =>
  within((await newButton()).closest("header")!)
    .getAllByRole("button")
    .map((b) => b.textContent);
const skip = async () => userEvent.click(await within(focus()).findByRole("button", { name: "Skip" }));

// added opens the modal for kind from + New ▾ and submits it.
async function added(kind: "Requirement" | "Map" | "Small Change") {
  if (kind === "Small Change") {
    await userEvent.click(menuButton());
    await userEvent.click(item("Small Change"));
    await submit("New Small Change", "Submit small change");
  } else if (kind === "Requirement") {
    await userEvent.click(await newButton());
    await submit("New requirement", "Submit requirement");
  } else {
    await userEvent.click(menuButton());
    await userEvent.click(item("Map"));
    await submit("New Map", "Submit map");
  }
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
}

const note = () => within(focus()).queryByRole("status");

// later moves the world's clock and the timers on by ms, once React has
// run the effects that start them.
async function later(w: World, ms: number) {
  await act(() => new Promise((r) => setImmediate(r)));
  w.advance(ms);
  act(() => vi.advanceTimersByTime(ms));
}

// submit types text into the open dialog and submits it.
async function submit(dialog: string, button: string, text = "Offline mode") {
  const modal = screen.getByRole("dialog", { name: dialog });
  await userEvent.type(within(modal).getByRole("textbox"), text);
  await userEvent.click(within(modal).getByRole("button", { name: button }));
  return modal;
}

describe("+ New ▾ in Focus Mode's header", () => {
  it("sits after the Project and N left, just before Exit, which stays at the far end", async () => {
    renderApp(await world(), "/?view=focus&project=acme");
    await heading("Flaky build");
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);
    const plus = await newButton();
    const before = (text: string) => (within(plus.closest("header")!).getByText(text).compareDocumentPosition(plus) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
    expect(before("acme")).toBe(true);
    expect(before("2 left")).toBe(true);
  });

  it("is plain, like Exit, not primary", async () => {
    renderApp(await world());
    expect(await newButton()).not.toHaveClass("primary");
    expect(menuButton()).not.toHaveClass("primary");
  });

  it("shows on Loading…, a Focus Item and You skipped N", async () => {
    const w = await world();
    const release = gateNeedsMe(w);
    renderApp(w);
    expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);

    release();
    await heading("Flaky build");
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);

    await skip();
    await heading("Login page");
    await skip();
    expect(await within(focus()).findByText("You skipped 2, go through them again?")).toBeInTheDocument();
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);
  });

  it("shows on All clear", async () => {
    renderApp(await world(false));
    await within(focus()).findByRole("region", { name: "All clear" });
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);
  });

  it("shows on the Activity Dashboard", async () => {
    const w = await world(false);
    await w.api.setOptions({ focus_clear: "activity_dashboard" });
    renderApp(w);
    expect(await within(focus()).findByRole("region", { name: "Activity Dashboard" })).toBeInTheDocument();
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);
  });

  it("opens New requirement from + New, and New Map from the ▾ menu", async () => {
    renderApp(await world());
    await userEvent.click(await newButton());
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
    renderApp(await world());
    await heading("Flaky build");
    await userEvent.keyboard("n");
    await userEvent.keyboard("N");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  describe("by keyboard", () => {
    const focusMenuButton = async () => {
      await newButton();
      menuButton().focus();
    };

    it("opens with ↓ on the first item, and moves with the arrows, wrapping at either end", async () => {
      renderApp(await world());
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
    });

    it("closes on Esc and returns focus to ▾", async () => {
      renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{Escape}");
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveFocus();
    });

    it("closes when Tab leaves the menu", async () => {
      renderApp(await world());
      await focusMenuButton();
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
      renderApp(w, "/?view=focus&project=acme");
      await heading("Flaky build");
      await skip();
      await heading("Login page");

      if (from === "+ New") await userEvent.click(await newButton());
      else {
        await userEvent.click(menuButton());
        await userEvent.click(item(from));
      }
      await submit(dialog, button);

      expect(w.calls).toContain(call);
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(within(focus()).getByRole("heading", { name: "Login page" })).toBeInTheDocument();
      expect(window.location.search).toBe("?view=focus&project=acme");
      expect(params().get("requirement")).toBeNull();

      // Flaky build is still skipped: skipping Login page leaves only the two.
      await skip();
      expect(await within(focus()).findByText("You skipped 2, go through them again?")).toBeInTheDocument();
    });

    it("leaves a half-written answer on the Focus Item as it was", async () => {
      const w = await world(false);
      const s = scenario(w);
      const id = await s.ticket("acme", { title: "Pick a colour" });
      await s.askRound("acme", id, [{ title: "Which colour?", body: "Red or blue" }]);
      renderApp(w);
      await heading("Which colour?");
      await userEvent.type(within(focus()).getByRole("textbox", { name: "Your answer" }), "Blue");

      await userEvent.click(await newButton());
      await submit("New requirement", "Submit requirement");
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(within(focus()).getByRole("textbox", { name: "Your answer" })).toHaveValue("Blue");
    });

    it("submits from a modal opened while Loading…, once the Projects arrive", async () => {
      const w = await world(false);
      const release = gateNeedsMe(w);
      renderApp(w);
      await userEvent.click(await newButton());
      release();
      await within(focus()).findByRole("region", { name: "All clear" });

      await submit("New requirement", "Submit requirement");
      expect(w.calls).toContain("submitRequirement");
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    });

    it("keeps the modal open with its error when the submit is refused", async () => {
      const w = await world();
      refuseSubmit(w);
      renderApp(w, "/?view=focus&project=acme");
      await heading("Flaky build");
      await userEvent.click(await newButton());
      const modal = await submit("New requirement", "Submit requirement");

      expect(await within(modal).findByText("the Project is archived")).toBeInTheDocument();
      expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
      expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
    });
  });

  describe("which Project", () => {
    it("preselects the Project Focus Mode is filtered to, without asking", async () => {
      const w = await world();
      renderApp(w, "/?view=focus&project=beta");
      await userEvent.click(await newButton());
      expect(within(screen.getByRole("dialog")).queryByRole("combobox")).toBeNull();
      await submit("New requirement", "Submit requirement");
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect((await w.api.findSpecs("beta")).map((s) => s.title)).toContain("Offline mode");
    });

    it("asks which active Project when Focus Mode isn't filtered to one, leaving out the Archived", async () => {
      renderApp(await world());
      await userEvent.click(await newButton());
      const select = within(screen.getByRole("dialog")).getByRole("combobox", { name: "Project" });
      expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["Acme", "Beta"]);
    });
  });

  describe("the added note", () => {
    beforeEach(() => vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true, advanceTimeDelta: 1 }));
    afterEach(() => vi.useRealTimers());

    it.each([["Requirement"], ["Map"], ["Small Change"]] as const)("reads %s added after a submit, and goes after 4 s", async (kind) => {
      const w = await world();
      renderApp(w);
      await heading("Flaky build");
      await added(kind);

      expect(await within(focus()).findByRole("status")).toHaveTextContent(`${kind} added`);
      await later(w, 3900);
      expect(note()).toHaveTextContent(`${kind} added`);
      await later(w, 100);
      expect(note()).toBeNull();
    });

    it("sits where Dealt with elsewhere does, and looks like it", async () => {
      const w = await world();
      renderApp(w);
      await heading("Flaky build");
      await dealtWith(w);
      const elsewhere = await within(focus()).findByRole("status");
      expect(elsewhere).toHaveTextContent("Dealt with elsewhere");
      const placement = (el: HTMLElement) => [el.parentElement, el.previousElementSibling, el.className];
      const elsewherePlacement = placement(elsewhere);

      await added("Requirement");
      const addedNote = await within(focus()).findByText("Requirement added");
      expect(placement(addedNote)).toEqual(elsewherePlacement);
    });

    it("shows on All clear", async () => {
      renderApp(await world(false));
      await within(focus()).findByRole("region", { name: "All clear" });
      await added("Requirement");
      expect(await within(focus()).findByRole("status")).toHaveTextContent("Requirement added");
    });

    it("shows on the Activity Dashboard", async () => {
      const w = await world(false);
      await w.api.setOptions({ focus_clear: "activity_dashboard" });
      renderApp(w);
      await within(focus()).findByRole("region", { name: "Activity Dashboard" });
      await added("Map");
      expect(await within(focus()).findByRole("status")).toHaveTextContent("Map added");
    });

    it("shows on You skipped N", async () => {
      renderApp(await world());
      await heading("Flaky build");
      await skip();
      await heading("Login page");
      await skip();
      await within(focus()).findByText("You skipped 2, go through them again?");
      await added("Requirement");
      expect(await within(focus()).findByRole("status")).toHaveTextContent("Requirement added");
    });

    it("shows on Loading…", async () => {
      const w = await world();
      const release = gateNeedsMe(w, Infinity);
      renderApp(w, "/?view=focus&project=acme");
      await added("Requirement");
      expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
      expect(note()).toHaveTextContent("Requirement added");
      release();
    });

    it("stays when the item on screen changes, until its own 4 s are up", async () => {
      const w = await world();
      renderApp(w);
      await heading("Flaky build");
      await added("Requirement");
      await later(w, 2000);
      await skip();
      await heading("Login page");
      expect(note()).toHaveTextContent("Requirement added");
      await later(w, 2000);
      expect(note()).toBeNull();
    });

    it("replaces Dealt with elsewhere, and starts its own 4 s", async () => {
      const w = await world();
      renderApp(w);
      await heading("Flaky build");
      await dealtWith(w);
      expect(await within(focus()).findByRole("status")).toHaveTextContent("Dealt with elsewhere");
      await later(w, 3000);

      await added("Map");
      expect(within(focus()).getAllByRole("status").map((n) => n.textContent)).toEqual(["Map added"]);
      await later(w, 3000);
      expect(note()).toHaveTextContent("Map added");
      await later(w, 1000);
      expect(note()).toBeNull();
    });

    it("is replaced by Dealt with elsewhere, which starts its own 4 s", async () => {
      const w = await world();
      renderApp(w);
      await heading("Flaky build");
      await added("Requirement");
      await later(w, 3000);

      await dealtWith(w);
      await heading("Login page");
      await within(focus()).findByText("Dealt with elsewhere");
      expect(within(focus()).getAllByRole("status").map((n) => n.textContent)).toEqual(["Dealt with elsewhere"]);
      await later(w, 3000);
      expect(note()).toHaveTextContent("Dealt with elsewhere");
      await later(w, 1000);
      expect(note()).toBeNull();
    });

    it("isn't replaced when the last item goes elsewhere to All clear, which shows no Dealt with elsewhere", async () => {
      const w = await world(false, [{ title: "Flaky build", state: "Failed" }]);
      renderApp(w);
      await heading("Flaky build");
      await added("Requirement");
      await dealtWith(w);
      await within(focus()).findByRole("region", { name: "All clear" });
      expect(note()).toHaveTextContent("Requirement added");
    });

    it("isn't shown when the submit is refused", async () => {
      const w = await world();
      refuseSubmit(w);
      renderApp(w);
      await heading("Flaky build");
      await userEvent.click(await newButton());
      const modal = await submit("New requirement", "Submit requirement");
      await within(modal).findByText("the Project is archived");
      expect(note()).toBeNull();
    });
  });
});
