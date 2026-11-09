// + New ▾ in Focus Mode: the real App against FakeApi, at ?view=focus. The
// same split as the sidebar's sits in the header beside Exit, plain, on
// every Focus screen; submitting from it closes the modal and leaves Focus
// Mode where it was, with a brief "Requirement added" or "Map added" note.
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FakeApi } from "../api/fake";
import { change, focus, renderApp, without } from "./focusTesting";

// world has acme-1 Failed, then acme-2 In Review, in that Needs Me order,
// with beta active and old archived.
function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addProject("old", "Old", { archived: true });
  fake.addTicket("acme", { title: "Flaky build", state: "Failed" }); // acme-1
  fake.addTicket("acme", { title: "Login page", state: "In Review" }); // acme-2
  fake.setNeedsMe([
    { reason: "failed", tickets: [{ id: "acme-1" }] },
    { reason: "in_review", tickets: [{ id: "acme-2", summary: "Added the login page" }] },
  ]);
  return fake;
}

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
async function added(kind: "Requirement" | "Map") {
  if (kind === "Requirement") {
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

// later moves FakeApi's clock and the timers on by ms, once React has
// run the effects that start them.
async function later(fake: FakeApi, ms: number) {
  await act(() => new Promise((r) => setImmediate(r)));
  fake.now = new Date(fake.now.getTime() + ms);
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
    renderApp(world(), "/?view=focus&project=acme");
    await heading("Flaky build");
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);
    const plus = await newButton();
    const before = (text: string) => (within(plus.closest("header")!).getByText(text).compareDocumentPosition(plus) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
    expect(before("acme")).toBe(true);
    expect(before("2 left")).toBe(true);
  });

  it("is plain, like Exit, not primary", async () => {
    renderApp(world());
    expect(await newButton()).not.toHaveClass("primary");
    expect(menuButton()).not.toHaveClass("primary");
  });

  it("shows on Loading…, a Focus Item and You skipped N", async () => {
    const fake = world();
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const needsMe = fake.needsMe.bind(fake);
    vi.spyOn(fake, "needsMe").mockImplementationOnce(async () => {
      await gate;
      return needsMe();
    });
    renderApp(fake);
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
    const fake = world();
    fake.setNeedsMe([]);
    renderApp(fake);
    await within(focus()).findByRole("region", { name: "All clear" });
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);
  });

  it("shows on the Activity Dashboard", async () => {
    const fake = world();
    fake.setNeedsMe([]);
    await fake.setOptions({ focus_clear: "activity_dashboard" });
    renderApp(fake);
    expect(await within(focus()).findByRole("region", { name: "Activity Dashboard" })).toBeInTheDocument();
    expect(await headerButtons()).toEqual(["+ New", "▾", "Exit"]);
  });

  it("opens New requirement from + New, and New Map from the ▾ menu", async () => {
    renderApp(world());
    await userEvent.click(await newButton());
    expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));

    await userEvent.click(menuButton());
    expect(screen.getByRole("menu", { name: "More to create" })).toBeVisible();
    await userEvent.click(item("Map"));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.getByRole("dialog", { name: "New Map" })).toBeVisible();
  });

  it("opens no modal on a key: there's no shortcut for + New", async () => {
    renderApp(world());
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
      renderApp(world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}");
      expect(menuButton()).toHaveAttribute("aria-expanded", "true");
      expect(item("Requirement")).toHaveFocus();
      await userEvent.keyboard("{ArrowDown}");
      expect(item("Map")).toHaveFocus();
      await userEvent.keyboard("{ArrowDown}");
      expect(item("Requirement")).toHaveFocus();
      await userEvent.keyboard("{ArrowUp}");
      expect(item("Map")).toHaveFocus();
    });

    it("closes on Esc and returns focus to ▾", async () => {
      renderApp(world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{Escape}");
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveFocus();
    });

    it("closes when Tab leaves the menu", async () => {
      renderApp(world());
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
    ])("from %s closes the modal and leaves Focus Mode as it was, skipped list and all", async (from, dialog, button, call) => {
      const fake = world();
      renderApp(fake, "/?view=focus&project=acme");
      await heading("Flaky build");
      await skip();
      await heading("Login page");

      if (from === "+ New") await userEvent.click(await newButton());
      else {
        await userEvent.click(menuButton());
        await userEvent.click(item(from));
      }
      await submit(dialog, button);

      expect(fake.calls).toContain(call);
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(within(focus()).getByRole("heading", { name: "Login page" })).toBeInTheDocument();
      expect(window.location.search).toBe("?view=focus&project=acme");
      expect(params().get("requirement")).toBeNull();

      // Flaky build is still skipped: skipping Login page leaves only the two.
      await skip();
      expect(await within(focus()).findByText("You skipped 2, go through them again?")).toBeInTheDocument();
    });

    it("leaves a half-written answer on the Focus Item as it was", async () => {
      const fake = world();
      fake.addTicket("acme", {
        title: "Pick a colour",
        state: "Needs Answer",
        rounds: [{ number: 1, claim: 1, posted: fake.now.toISOString(), questions: [{ number: 1, title: "Which colour?", body: "Red or blue", status: "open" }] }],
      }); // acme-3
      fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: "acme-3", round: { number: 1, open: 1, questions: 1 } }] }]);
      renderApp(fake);
      await heading("Which colour?");
      await userEvent.type(within(focus()).getByRole("textbox", { name: "Your answer" }), "Blue");

      await userEvent.click(await newButton());
      await submit("New requirement", "Submit requirement");
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(within(focus()).getByRole("textbox", { name: "Your answer" })).toHaveValue("Blue");
    });

    it("submits from a modal opened while Loading…, once the Projects arrive", async () => {
      const fake = world();
      fake.setNeedsMe([]);
      let release!: () => void;
      const gate = new Promise<void>((r) => (release = r));
      const needsMe = fake.needsMe.bind(fake);
      vi.spyOn(fake, "needsMe").mockImplementationOnce(async () => {
        await gate;
        return needsMe();
      });
      renderApp(fake);
      await userEvent.click(await newButton());
      release();
      await within(focus()).findByRole("region", { name: "All clear" });

      await submit("New requirement", "Submit requirement");
      expect(fake.calls).toContain("submitRequirement");
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    });

    it("keeps the modal open with its error when the submit is refused", async () => {
      const fake = world();
      fake.submitRequirement = async () => {
        throw new Error("the Project is archived");
      };
      renderApp(fake, "/?view=focus&project=acme");
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
      const fake = world();
      renderApp(fake, "/?view=focus&project=beta");
      await userEvent.click(await newButton());
      expect(within(screen.getByRole("dialog")).queryByRole("combobox")).toBeNull();
      await submit("New requirement", "Submit requirement");
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect((await fake.findSpecs("beta")).map((s) => s.title)).toContain("Offline mode");
    });

    it("asks which active Project when Focus Mode isn't filtered to one, leaving out the Archived", async () => {
      renderApp(world());
      await userEvent.click(await newButton());
      const select = within(screen.getByRole("dialog")).getByRole("combobox", { name: "Project" });
      expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["Acme", "Beta"]);
    });
  });

  describe("the added note", () => {
    beforeEach(() => vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true, advanceTimeDelta: 1 }));
    afterEach(() => vi.useRealTimers());

    it.each([["Requirement"], ["Map"]] as const)("reads %s added after a submit, and goes after 4 s", async (kind) => {
      const fake = world();
      renderApp(fake);
      await heading("Flaky build");
      await added(kind);

      expect(await within(focus()).findByRole("status")).toHaveTextContent(`${kind} added`);
      await later(fake, 3900);
      expect(note()).toHaveTextContent(`${kind} added`);
      await later(fake, 100);
      expect(note()).toBeNull();
    });

    it("sits where Dealt with elsewhere does, and looks like it", async () => {
      const fake = world();
      renderApp(fake);
      await heading("Flaky build");
      change(fake, without(fake, "acme-1"), "acme-1");
      const elsewhere = await within(focus()).findByRole("status");
      expect(elsewhere).toHaveTextContent("Dealt with elsewhere");
      const placement = (el: HTMLElement) => [el.parentElement, el.previousElementSibling, el.className];
      const elsewherePlacement = placement(elsewhere);

      await added("Requirement");
      const addedNote = await within(focus()).findByText("Requirement added");
      expect(placement(addedNote)).toEqual(elsewherePlacement);
    });

    it("shows on All clear", async () => {
      const fake = world();
      fake.setNeedsMe([]);
      renderApp(fake);
      await within(focus()).findByRole("region", { name: "All clear" });
      await added("Requirement");
      expect(await within(focus()).findByRole("status")).toHaveTextContent("Requirement added");
    });

    it("shows on the Activity Dashboard", async () => {
      const fake = world();
      fake.setNeedsMe([]);
      await fake.setOptions({ focus_clear: "activity_dashboard" });
      renderApp(fake);
      await within(focus()).findByRole("region", { name: "Activity Dashboard" });
      await added("Map");
      expect(await within(focus()).findByRole("status")).toHaveTextContent("Map added");
    });

    it("shows on You skipped N", async () => {
      renderApp(world());
      await heading("Flaky build");
      await skip();
      await heading("Login page");
      await skip();
      await within(focus()).findByText("You skipped 2, go through them again?");
      await added("Requirement");
      expect(await within(focus()).findByRole("status")).toHaveTextContent("Requirement added");
    });

    it("shows on Loading…", async () => {
      const fake = world();
      let release!: () => void;
      const gate = new Promise<void>((r) => (release = r));
      const needsMe = fake.needsMe.bind(fake);
      vi.spyOn(fake, "needsMe").mockImplementation(async () => {
        await gate;
        return needsMe();
      });
      renderApp(fake, "/?view=focus&project=acme");
      await added("Requirement");
      expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
      expect(note()).toHaveTextContent("Requirement added");
      release();
    });

    it("stays when the item on screen changes, until its own 4 s are up", async () => {
      const fake = world();
      renderApp(fake);
      await heading("Flaky build");
      await added("Requirement");
      await later(fake, 2000);
      await skip();
      await heading("Login page");
      expect(note()).toHaveTextContent("Requirement added");
      await later(fake, 2000);
      expect(note()).toBeNull();
    });

    it("replaces Dealt with elsewhere, and starts its own 4 s", async () => {
      const fake = world();
      renderApp(fake);
      await heading("Flaky build");
      change(fake, without(fake, "acme-1"), "acme-1");
      expect(await within(focus()).findByRole("status")).toHaveTextContent("Dealt with elsewhere");
      await later(fake, 3000);

      await added("Map");
      expect(within(focus()).getAllByRole("status").map((n) => n.textContent)).toEqual(["Map added"]);
      await later(fake, 3000);
      expect(note()).toHaveTextContent("Map added");
      await later(fake, 1000);
      expect(note()).toBeNull();
    });

    it("is replaced by Dealt with elsewhere, which starts its own 4 s", async () => {
      const fake = world();
      renderApp(fake);
      await heading("Flaky build");
      await added("Requirement");
      await later(fake, 3000);

      change(fake, without(fake, "acme-1"), "acme-1");
      await heading("Login page");
      await within(focus()).findByText("Dealt with elsewhere");
      expect(within(focus()).getAllByRole("status").map((n) => n.textContent)).toEqual(["Dealt with elsewhere"]);
      await later(fake, 3000);
      expect(note()).toHaveTextContent("Dealt with elsewhere");
      await later(fake, 1000);
      expect(note()).toBeNull();
    });

    it("isn't replaced when the last item goes elsewhere to All clear, which shows no Dealt with elsewhere", async () => {
      const fake = world();
      fake.setNeedsMe(fake.needsMeGroups.filter((g) => g.reason === "failed"));
      renderApp(fake);
      await heading("Flaky build");
      await added("Requirement");
      change(fake, [], "acme-1");
      await within(focus()).findByRole("region", { name: "All clear" });
      expect(note()).toHaveTextContent("Requirement added");
    });

    it("isn't shown when the submit is refused", async () => {
      const fake = world();
      fake.submitRequirement = async () => {
        throw new Error("the Project is archived");
      };
      renderApp(fake);
      await heading("Flaky build");
      await userEvent.click(await newButton());
      const modal = await submit("New requirement", "Submit requirement");
      await within(modal).findByText("the Project is archived");
      expect(note()).toBeNull();
    });
  });
});
