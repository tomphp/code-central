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
import { FakeApi } from "../api/fake";
import type { NeedsMeReason } from "../api/types";
import { failCI } from "../ciTesting";
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
// heading looks the popup up again each try: a card's Focus popup and the
// automatic one share the name, and one can replace the other mid-wait.
const heading = (name: string) => waitFor(() => within(popup()).getByRole("heading", { name }));
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

describe("Focus Pop-up after a close", () => {
  // roundOn is waiting with Focus Pop-up on and only acme-1's Round in
  // Needs Me.
  function roundOn() {
    const fake = waiting();
    fake.seedOptions({ focus_popup: "on" });
    fake.setNeedsMe(fake.needsMeGroups.filter((g) => g.reason === "needs_answer"));
    return fake;
  }
  // stillClosed sends a notice that changes nothing and waits for the
  // App to read Needs Me again, so a popup that would open has had its
  // chance.
  async function stillClosed(fake: FakeApi) {
    const read = vi.spyOn(fake, "needsMe");
    change(fake, fake.needsMeGroups);
    await waitFor(() => expect(read).toHaveBeenCalled());
    await act(async () => {});
    noPopup();
  }

  it("stays closed for a Round closed unread", async () => {
    const fake = waiting();
    fake.seedOptions({ focus_popup: "on" });
    const [round, failed, inReview] = fake.needsMeGroups;
    fake.setNeedsMe([failed, round, inReview]);
    renderApp(fake, "/?view=inbox");
    await heading("Flaky build");
    await close();
    await stillClosed(fake);
  });

  it("stays closed for a Round closed read", async () => {
    const fake = roundOn();
    renderApp(fake, "/?view=inbox");
    await heading("Which colour?");
    await close();
    await stillClosed(fake);
  });

  it("opens again for a new Round on a Ticket it closed on", async () => {
    const fake = roundOn();
    renderApp(fake, "/?view=inbox");
    await heading("Which colour?");
    await close();

    const t = await fake.getTicket("acme", "acme-1");
    const answered = t.rounds.map((r) => ({ ...r, questions: r.questions.map((q) => ({ ...q, status: "answered" as const, answer: "Yes" })) }));
    const next = { number: 2, claim: 1, posted: fake.now.toISOString(), questions: [{ number: 1, title: "Which shape?", body: "Round or square", status: "open" as const }] };
    fake.updateTicket("acme-1", { rounds: [...answered, next] });
    change(fake, [{ reason: "needs_answer", tickets: [{ id: "acme-1", round: { number: 2, open: 1, questions: 1 } }] }]);
    expect(await heading("Which shape?")).toBeInTheDocument();
  });

  it("opens again for a Ticket it closed on, waiting for a new reason", async () => {
    const fake = popupOn();
    renderApp(fake, "/?view=inbox&project=acme");
    await heading("Flaky build");
    await close();

    fake.updateTicket("acme-2", { state: "In Review" });
    change(fake, [{ reason: "in_review", tickets: [{ id: "acme-2", summary: "Fixed the build" }, { id: "acme-3", summary: "Added the login page" }] }], "acme-2");
    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(within(popup()).getByText("2 left")).toBeInTheDocument();
  });

  it("opens again for a new CI item of a Project it closed on", async () => {
    const fake = new FakeApi();
    fake.seedOptions({ focus_popup: "on" });
    fake.addProject("acme", "Acme");
    fake.addTicket("acme", { title: "Basket", state: "Accepted", branch: "acme-1", commit: "abc" }); // acme-1
    const item = await failCI(fake, "acme", ["acme-1"]);
    renderApp(fake, "/?view=inbox");
    await heading("CI failed at abc1234");
    await close();
    await stillClosed(fake);

    const [row] = fake.needsMeGroups[0].tickets;
    change(fake, [{ reason: "ci_failed", tickets: [{ ...row, title: "CI failed at def5678", ci_item: { ...item, commit: "def5678901" } }] }]);
    expect(await heading("CI failed at def5678")).toBeInTheDocument();
  });
});

describe("Focus Pop-up and Focus Kinds", () => {
  // Each row is a Focus Item or not by its own Project's Focus Kinds, as
  // core marks it: here acme has Answered, CI failed and Base diverged
  // off, beta has nothing off.
  // rowOf is Needs Me's one row, of reason in project: a Ticket's, a CI
  // item's or a Base diverged item's.
  function rowOf(fake: FakeApi, reason: NeedsMeReason, project: string) {
    if (reason === "ci_failed") return { id: "", project, title: "CI failed at aaa1111", ci_item: { reason, commit: "aaa1111222", tickets: [], options: [] } };
    if (reason === "base_diverged") return { id: "", project, title: "Base diverged", base_diverged: { base: "main", local: "aaa1111222", remote: "bbb2222333" } };
    return { id: fake.addTicket(project, { title: "Waiting", state: reason === "answered" ? "Answered" : "Failed" }).id };
  }
  it.each<[string, boolean, NeedsMeReason, string, string?]>([
    ["an Answered Ticket in a Project with Answered off", false, "answered", "acme"],
    ["the same, with that Project chosen", false, "answered", "acme", "acme"],
    ["an Answered Ticket in another Project", true, "answered", "beta"],
    ["a reason left on in that Project", true, "failed", "acme"],
    ["a CI item of a reason off in its Project", false, "ci_failed", "acme"],
    ["a CI item of that reason in another Project", true, "ci_failed", "beta"],
    ["a Base diverged item of a Project with it off", false, "base_diverged", "acme"],
    ["a Base diverged item of another Project", true, "base_diverged", "beta"],
  ])("with %s, opens: %s", async (_, opens, reason, project, chosen) => {
    const fake = new FakeApi();
    fake.seedOptions({ focus_popup: "on" });
    fake.addProject("acme", "Acme");
    fake.addProject("beta", "Beta");
    await fake.setFocusKinds("acme", ["answered", "ci_failed", "base_diverged"]);
    fake.setNeedsMe([{ reason, tickets: [rowOf(fake, reason, project)] }]);
    await settled(fake, `/?view=inbox${chosen ? `&project=${chosen}` : ""}`);
    await act(async () => {});
    if (opens) expect(await screen.findByRole("dialog", { name: "Focus popup" })).toBeInTheDocument();
    else noPopup();
  });

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
