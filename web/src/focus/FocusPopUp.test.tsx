// Focus Pop-up: the real App against FakeApi. With the Option on, the
// Focus popup opens by itself in Queue mode over the Inbox, the Board, the
// Activity Dashboard, the CI view and the Usage view while Focus Items in
// the view's scope wait, and again when a new one arrives; never over Focus
// Mode, Options or Project Options. Closing it holds for the Focus Items
// there then, for the tab's life. It never interrupts: it waits while
// another popup is open or a text field has focus. It opens for, shows and
// counts only Focus Items: rows of a reason on in their Project's Focus
// Kinds.
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { FakeApi } from "../api/fake";
import { change, renderApp, waiting, waitingTickets, without } from "./focusTesting";

// popupOn is waitingTickets with Focus Pop-up on: acme-2 Failed, then acme-3
// In Review, then beta-1 In Review.
function popupOn() {
  const fake = waitingTickets();
  fake.seedOptions({ focus_popup: "on" });
  fake.addProject("beta", "Beta");
  fake.addTicket("beta", { title: "Beta search", state: "In Review" }); // beta-1
  fake.setNeedsMe(fake.needsMeGroups.map((g) => (g.reason === "in_review" ? { ...g, tickets: [...g.tickets, { id: "beta-1", summary: "Added search" }] } : g)));
  return fake;
}

// arrive adds a Ticket with a silent Claim at the front of Needs Me, as a
// new Focus Item through the live feed, and returns its ID.
function arrive(fake: FakeApi, project = "acme"): string {
  const t = fake.addTicket(project, { title: "Quiet session", state: "In Progress" });
  change(fake, [{ reason: "silent_claim", tickets: [{ id: t.id }] }, ...fake.needsMeGroups], t.id);
  return t.id;
}

// go navigates in place, as a link in a view does.
function go(url: string) {
  act(() => {
    window.history.pushState(null, "", url);
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
}

const popup = () => screen.getByRole("dialog", { name: "Focus popup" });
const heading = async (name: string) => within(await screen.findByRole("dialog", { name: "Focus popup" })).findByRole("heading", { name });
const close = async () => userEvent.click(within(popup()).getByRole("button", { name: "Close" }));
const noPopup = () => expect(screen.queryByRole("dialog", { name: "Focus popup" })).toBeNull();

// settled renders the App at url and waits for its Needs Me to load, so a
// popup that would open has had its chance.
async function settled(fake: FakeApi, url: string) {
  document.title = "";
  renderApp(fake, url);
  await waitFor(() => expect(document.title).toMatch(/^\(\d+\)/));
}

describe("Focus Pop-up", () => {
  it.each(["inbox", "board", "activity", "ci", "usage"])("opens the Focus popup over the %s view while Focus Items wait", async (view) => {
    renderApp(popupOn(), `/?view=${view}`);
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
  });

  it.each(["focus", "options", "project-options&project=acme"])("never opens over the %s view", async (view) => {
    await settled(popupOn(), `/?view=${view}`);
    noPopup();
  });

  it("is scoped to the view's Project", async () => {
    renderApp(popupOn(), "/?view=board&project=beta");
    expect(await heading("Beta search")).toBeInTheDocument();
    expect(within(popup()).getByText("1 left")).toBeInTheDocument();
  });

  it("goes through the Focus Items with Skip, closes once only skipped ones are left, and doesn't reopen for them", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=inbox&project=acme");
    await heading("Flaky build");
    await userEvent.click(await within(popup()).findByRole("button", { name: "Skip" }));
    await heading("Login page");
    await userEvent.click(await within(popup()).findByRole("button", { name: "Skip" }));
    await waitFor(noPopup);
    go("/?view=board&project=acme");
    await screen.findByRole("main", { name: "Board" });
    noPopup();
    expect(fake.actions()).toEqual([]);
  });

  it("closes once nothing in scope is left", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=activity&project=beta");
    await heading("Beta search");
    change(fake, fake.needsMeGroups.filter((g) => g.reason !== "in_review"), "beta-1");
    await waitFor(noPopup);
  });

  it("stays closed across views once closed, until a new Focus Item arrives through the live feed", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=inbox");
    await heading("Flaky build");
    await close();
    noPopup();
    go("/?view=board");
    await screen.findByRole("main", { name: "Board" });
    go("/?view=ci");
    await screen.findByRole("main", { name: "CI" });
    noPopup();

    arrive(fake);
    expect(await heading("Quiet session")).toBeInTheDocument();
    expect(within(popup()).getByText("4 left")).toBeInTheDocument();
  });

  it("once open for a new Focus Item, goes on through the rest in scope", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=activity");
    await heading("Flaky build");
    await close();
    const id = arrive(fake);
    await heading("Quiet session");
    change(fake, without(fake, id), id);
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
  });

  it("answers a Round's Questions one at a time, and doesn't reopen for the rest of the Round once closed", async () => {
    const fake = waiting();
    fake.seedOptions({ focus_popup: "on" });
    renderApp(fake, "/?view=board");
    expect(await heading("Which colour?")).toBeInTheDocument();
    await userEvent.click(within(popup()).getByRole("button", { name: "Answer" }));
    expect(await heading("Which size?")).toBeInTheDocument();
    expect(within(popup()).getByText("Question 2 of 2 · Round 1")).toBeInTheDocument();
    await close();
    go("/?view=inbox");
    await screen.findByRole("complementary", { name: "Ticket" });
    noPopup();
    expect((await fake.getTicket("acme", "acme-1")).rounds[0].questions.map((q) => q.status)).toEqual(["answered", "open"]);
  });

  it("holds a close only for the Focus Items in scope then", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=inbox&project=beta");
    await heading("Beta search");
    await close();
    go("/?view=inbox");
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(within(popup()).getByText("3 left")).toBeInTheDocument();
  });

  it("doesn't open in one Project's view for a new Focus Item in another", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=board&project=beta");
    await heading("Beta search");
    await close();
    arrive(fake, "acme");
    await waitFor(() => expect(document.title).toBe("(4) Code Central"));
    noPopup();
  });

  it("stays open, where it was, through a failed read of the Options", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=inbox");
    await heading("Flaky build");
    await userEvent.click(await within(popup()).findByRole("button", { name: "Skip" }));
    await heading("Login page");
    const read = vi.spyOn(fake, "options").mockRejectedValue(new Error("service down"));
    act(() => fake.emit({ type: "options.changed" }));
    await waitFor(() => expect(read).toHaveBeenCalled());
    await act(async () => {});
    expect(within(popup()).getByRole("heading", { name: "Login page" })).toBeInTheDocument();
  });

  it("forgets what was closed on a reload", async () => {
    const fake = popupOn();
    const { unmount } = renderApp(fake, "/?view=usage");
    await heading("Flaky build");
    await close();
    unmount();
    renderApp(fake, "/?view=usage");
    expect(await heading("Flaky build")).toBeInTheDocument();
  });

  it("leaves the address and history alone, and Back doesn't close it", async () => {
    window.history.replaceState(null, "", "/?view=board");
    window.history.pushState(null, "", "/?view=activity");
    renderApp(popupOn(), "/?view=activity");
    await heading("Flaky build");
    const length = window.history.length;
    expect(window.location.search).toBe("?view=activity");
    act(() => window.history.back());
    await waitFor(() => expect(window.location.search).toBe("?view=board"));
    await screen.findByRole("main", { name: "Board" });
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(window.history.length).toBe(length);
  });

  it("lands on the Focus Item's Ticket in the Inbox from Open in Inbox, and stays closed there", async () => {
    renderApp(popupOn(), "/?view=ci");
    const item = within(await screen.findByRole("dialog", { name: "Focus popup" }));
    await userEvent.click(await item.findByRole("link", { name: "Open in Inbox" }));
    expect(window.location.search).toBe("?view=inbox&ticket=acme-2");
    const detail = await screen.findByRole("complementary", { name: "Ticket" });
    expect(await within(detail).findByText("Flaky build")).toBeInTheDocument();
    noPopup();
  });

  it.each(["inbox", "board", "activity", "ci", "usage", "focus", "options", "project-options&project=acme"])(
    "leaves the %s view alone while it's off",
    async (view) => {
      await settled(waitingTickets(), `/?view=${view}`);
      noPopup();
    },
  );
});

describe("Focus Pop-up and Focus Kinds", () => {
  // projectRead waits for the Projects to be read again after a saved
  // change to Focus Kinds, and for the App to render what it read.
  async function projectRead(fake: FakeApi, change: () => Promise<unknown>) {
    const read = vi.spyOn(fake, "listProjects");
    await act(change);
    await waitFor(() => expect(read).toHaveBeenCalled());
    await act(async () => {});
    read.mockRestore();
  }

  it("doesn't open for items of a reason turned off in their Project, though Needs Me counts them", async () => {
    const fake = popupOn();
    await fake.setFocusKinds("acme", ["failed", "in_review"]);
    await settled(fake, "/?view=inbox&project=acme");
    expect(document.title).toBe("(3) Code Central");
    noPopup();
  });

  it("leaves them out and doesn't count them when another item opens it, while another Project's still show", async () => {
    const fake = popupOn();
    await fake.setFocusKinds("acme", ["in_review"]);
    renderApp(fake, "/?view=board");
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(within(popup()).getByText("2 left")).toBeInTheDocument();
    await userEvent.click(await within(popup()).findByRole("button", { name: "Skip" }));
    expect(await heading("Beta search")).toBeInTheDocument();
    expect(popup()).not.toHaveTextContent("Login page");
  });

  it("closes as cleared once only items of reasons turned off are left", async () => {
    const fake = popupOn();
    await fake.setFocusKinds("acme", ["in_review"]);
    renderApp(fake, "/?view=activity&project=acme");
    await heading("Flaky build");
    change(fake, without(fake, "acme-2"), "acme-2");
    await waitFor(noPopup);
  });

  it("closes as cleared when the reason of the only item left is turned off", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=activity&project=beta");
    await heading("Beta search");
    await projectRead(fake, () => fake.setFocusKinds("beta", ["in_review"]));
    await waitFor(noPopup);
  });

  it("opens for items a reason turned back on brings back, unless they were dismissed in this tab", async () => {
    const fake = popupOn();
    await fake.setFocusKinds("acme", ["in_review"]);
    renderApp(fake, "/?view=inbox&project=acme");
    await heading("Flaky build");
    expect(within(popup()).getByText("1 left")).toBeInTheDocument();
    await close();

    await projectRead(fake, () => fake.setFocusKinds("acme", []));
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(within(popup()).getByText("2 left")).toBeInTheDocument();
    await close();

    await projectRead(fake, () => fake.setFocusKinds("acme", ["in_review"]));
    await projectRead(fake, () => fake.setFocusKinds("acme", []));
    noPopup();
  });
});

describe("Focus Pop-up never interrupts", () => {
  const board = () => screen.getByRole("main", { name: "Board" });
  const dialogs = () => screen.queryAllByRole("dialog").map((d) => d.getAttribute("aria-label"));
  // dismissed renders the App at url, waits for the Focus popup and
  // closes it, so only a new Focus Item opens it again.
  async function dismissed(fake: FakeApi, url: string, first = "Flaky build") {
    renderApp(fake, url);
    await heading(first);
    await close();
    noPopup();
  }

  it("still opens a card's Focus popup, and waits for it to close before opening for a new Focus Item", async () => {
    const fake = popupOn();
    await dismissed(fake, "/?view=board&project=acme");
    await userEvent.click(within(board()).getByText("Login page"));
    expect(await heading("Login page")).toBeInTheDocument();
    expect(window.location.search).toBe("?view=board&project=acme&popup=acme-3");
    expect(popup()).not.toHaveTextContent(/\d+ left/);

    arrive(fake);
    await waitFor(() => expect(document.title).toBe("(4) Code Central"));
    await act(async () => {});
    expect(dialogs()).toEqual(["Focus popup"]);
    expect(popup()).not.toHaveTextContent("Quiet session");

    await close();
    expect(await heading("Quiet session")).toBeInTheDocument();
    expect(within(popup()).getByText(/\d+ left/)).toBeInTheDocument();
  });

  it("waits for a card's Focus popup loaded from the address, then opens for the Focus Items left once it closes by itself", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=board&project=acme&popup=acme-2");
    await heading("Flaky build");
    expect(dialogs()).toEqual(["Focus popup"]);
    expect(popup()).not.toHaveTextContent(/\d+ left/);

    change(fake, without(fake, "acme-2"), "acme-2");
    await waitFor(() => expect(window.location.search).toBe("?view=board&project=acme"));
    expect(await heading("Login page")).toBeInTheDocument();
    expect(within(popup()).getByText("1 left")).toBeInTheDocument();
  });

  it("doesn't open for the Focus Items a card's Focus popup showed once it's closed", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=board&project=beta&popup=beta-1");
    await heading("Beta search");
    await close();
    await waitFor(() => expect(window.location.search).toBe("?view=board&project=beta"));
    await act(async () => {});
    noPopup();
    go("/?view=inbox&project=beta");
    await screen.findByRole("complementary", { name: "Ticket" });
    noPopup();
  });

  it("doesn't open for the Focus Items a card's Focus popup showed once Back closes it", async () => {
    window.history.replaceState(null, "", "/?view=board&project=beta");
    window.history.pushState(null, "", "/?view=board&project=beta&popup=beta-1");
    renderApp(popupOn(), "/?view=board&project=beta&popup=beta-1");
    await heading("Beta search");
    act(() => window.history.back());
    await waitFor(() => expect(window.location.search).toBe("?view=board&project=beta"));
    await act(async () => {});
    noPopup();
  });

  it("waits while another popup is open, and opens once it closes", async () => {
    const fake = popupOn();
    await dismissed(fake, "/?view=board&project=acme");
    // acme-1 doesn't need the human: its card opens its Ticket popup.
    await userEvent.click(within(board()).getByText("Pick a colour"));
    await screen.findByRole("dialog", { name: "Ticket popup" });

    arrive(fake);
    await waitFor(() => expect(document.title).toBe("(4) Code Central"));
    await act(async () => {});
    expect(dialogs()).toEqual(["Ticket popup"]);

    await userEvent.keyboard("{Escape}");
    expect(await heading("Quiet session")).toBeInTheDocument();
    expect(dialogs()).toEqual(["Focus popup"]);
  });

  it("waits while a text field has focus, and opens once it loses it", async () => {
    const fake = popupOn();
    await dismissed(fake, "/?view=inbox&list=tickets");
    const box = await screen.findByRole("searchbox", { name: "Ticket ID" });
    await userEvent.click(box);
    await userEvent.keyboard("4");

    arrive(fake);
    await waitFor(() => expect(document.title).toBe("(4) Code Central"));
    noPopup();
    expect(box).toHaveFocus();

    act(() => box.blur());
    expect(await heading("Quiet session")).toBeInTheDocument();
  });

  it("doesn't open on load while a text field has focus", async () => {
    const field = document.body.appendChild(document.createElement("textarea"));
    field.focus();
    try {
      await settled(popupOn(), "/?view=activity");
      noPopup();
      act(() => field.blur());
      expect(await heading("Flaky build")).toBeInTheDocument();
    } finally {
      field.remove();
    }
  });
});
