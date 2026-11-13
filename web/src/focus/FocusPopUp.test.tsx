// Focus Pop-up: the real App against FakeApi. With the Option on, the
// Focus popup opens by itself in Queue mode over the Inbox, the Board, the
// Activity Dashboard, the CI view and the Usage view while Focus Items in
// the view's scope wait, and again when a new one arrives; never over Focus
// Mode, Options or Project Options. Closing it holds for the Focus Items
// there then, for the tab's life.
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
