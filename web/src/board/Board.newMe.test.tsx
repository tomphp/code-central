// Board view tests: + New ▾ and the Needs Me column.
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { failCI } from "../ciTesting";
import { MINUTE, world, renderApp, settled, click, needing, boardLink, board, heads, column, cards, titles, columnsButton, loaded } from "./Board.testutil";

beforeEach(() => localStorage.clear());

describe("+ New ▾ on the Board", () => {
  const url = "/?view=board&project=acme";
  const kinds = [
    ["Requirement", "New requirement", "Submit requirement"],
    ["Map", "New Map", "Submit map"],
    ["Small Change", "New Small Change", "Submit small change"],
  ] as const;
  // create picks kind from + New ▾ and submits some text.
  async function create(kind: string, dialog: string, button: string) {
    await click(within(board()).getByRole("button", { name: "More to create" }));
    await click(within(screen.getByRole("menu")).getByRole("menuitem", { name: kind }));
    const modal = screen.getByRole("dialog", { name: dialog });
    await userEvent.type(within(modal).getByRole("textbox"), "Offline mode");
    await click(within(modal).getByRole("button", { name: button }));
  }

  it.each(kinds)("stays on the Board after a %s, saying so, with nothing opened", async (kind, dialog, button) => {
    await renderApp(await world(), url);
    loaded();
    await create(kind, dialog, button);

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(within(board()).getByRole("status")).toHaveTextContent(`${kind} added`);
    expect(window.location.search).toBe("?view=board&project=acme");
  });

  it("keeps the columns, grouping and folded lanes", async () => {
    const start = "/?view=board&project=acme&group=spec&hide=Done&needs=1";
    await renderApp(await world(), start);
    loaded();
    const login = () => within(within(board()).getByRole("group", { name: "Lane: Login" })).getByRole("button", { name: "Login" });
    await click(login());
    await create("Small Change", "New Small Change", "Submit small change");

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(window.location.search).toBe(start.slice(1));
    expect(login()).toHaveAttribute("aria-expanded", "false");
  });

  describe("the note", () => {
    beforeEach(() => vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true, advanceTimeDelta: 1 }));
    afterEach(() => vi.useRealTimers());

    it("goes after 4 s, and a second submit's note replaces the first and restarts the 4 s", async () => {
      await renderApp(await world(), url);
      loaded();
      await create("Requirement", "New requirement", "Submit requirement");
      expect(within(board()).getByRole("status")).toHaveTextContent("Requirement added");

      act(() => vi.advanceTimersByTime(3000));
      await create("Small Change", "New Small Change", "Submit small change");
      expect(within(board()).getByRole("status")).toHaveTextContent("Small Change added");
      expect(within(board()).getAllByRole("status")).toHaveLength(1);

      act(() => vi.advanceTimersByTime(3000));
      expect(within(board()).getByRole("status")).toHaveTextContent("Small Change added");
      act(() => vi.advanceTimersByTime(1500));
      expect(within(board()).queryByRole("status")).toBeNull();
    });
  });
});

describe("the Board's Needs Me column", () => {
  const needsOn = "/?view=board&project=acme&needs=1";
  const needsCol = () => within(board()).getByRole("region", { name: "Needs Me" });
  const needsTitles = () => within(needsCol()).queryAllByRole("link").map((c) => c.querySelector(".board-card-title")?.textContent);

  it("is off by default: no column, the address untouched, and its checkbox after In Progress", async () => {
    const fake = await world();
    await needing(fake);
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    expect(heads()).not.toContain("Needs Me 2");
    expect(within(board()).queryByRole("region", { name: "Needs Me" })).toBeNull();
    expect(window.location.search).toBe("?view=board&project=acme");
    expect(columnsButton()).toHaveTextContent("Columns · 8/10");
  });

  it("is shown with Columns ▾, kept in the address and the browser, and doesn't change the N/M count", async () => {
    const fake = await world();
    await needing(fake);
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    within(board()).getByRole("link", { name: /Password reset/ });
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Needs Me" }));
    expect(new URLSearchParams(window.location.search).get("needs")).toBe("1");
    expect(heads()).toEqual(["Draft 0", "Ready 1", "In Progress 0", "Needs Me 2", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Accepted 0"]);
    expect(columnsButton()).toHaveTextContent("Columns · 8/10");
    unmount();

    await renderApp(fake, "/?view=board&project=acme");
    within(board()).getByRole("link", { name: /Password reset/ });
    expect(heads()).toContain("Needs Me 2");

    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Needs Me" }));
    expect(new URLSearchParams(window.location.search).get("needs")).toBe("0");
    expect(heads()).not.toContain("Needs Me 2");
  });

  it("follows the address over the browser, and old hide= addresses open without it", async () => {
    localStorage.setItem("code-central.board.needs", "1");
    const fake = await world();
    await needing(fake);
    const { unmount } = await renderApp(fake, "/?view=board&project=acme&needs=0");
    within(board()).getByRole("link", { name: /Password reset/ });
    expect(heads().some((h) => h?.startsWith("Needs Me"))).toBe(false);
    unmount();
    localStorage.clear();
    await renderApp(fake, "/?view=board&project=acme&hide=Done");
    within(board()).getByRole("link", { name: /Password reset/ });
    expect(heads().some((h) => h?.startsWith("Needs Me"))).toBe(false);
  });

  it("holds needing Tickets in Needs Me's order, out of their State columns, with State chip and reason", async () => {
    const fake = await world();
    await needing(fake);
    await renderApp(fake, needsOn);
    within(board()).getByRole("link", { name: /Password reset/ });
    expect(needsTitles()).toEqual(["Sign-up page", "Login form"]);
    expect(cards("Ready").map((c) => c.textContent)).not.toContain("Login form");
    expect(titles("Ready")).toEqual(["Password reset"]);
    const [first, second] = within(needsCol()).getAllByRole("link");
    expect(first).toHaveTextContent("Needs Answer");
    expect(first).toHaveTextContent("Questions");
    expect(second).toHaveTextContent("In Review");
    expect(second).toHaveTextContent("In Review");
    expect(within(column("Ready")).getByRole("link")).not.toHaveTextContent("Questions");
    expect(within(board()).getByRole("heading", { name: "Needs Me 2" })).toBeInTheDocument();
  });

  it("lists a CI item's Tickets, and not Base diverged", async () => {
    const fake = await world();
    const basket = await fake.s.ticket("acme", { title: "Basket", state: "Accepted", spec: fake.ids.spec });
    await failCI(fake.s, "acme", [basket]);
    await renderApp(fake, needsOn);
    loaded();
    expect(needsTitles()).toEqual(["Basket"]);
    expect(within(needsCol()).getByRole("link")).toHaveTextContent("CI failed");
  });

  it("shows a needing Ticket whose State column is hidden, and no other Ticket of it", async () => {
    const fake = await world();
    await fake.s.inReview("acme", fake.ids.form);
    await renderApp(fake, `${needsOn}&hide=Done,Cancelled,Ready,In Review`);
    within(board()).getByRole("link", { name: /Login form/ });
    expect(needsTitles()).toEqual(["Login form"]);
    expect(within(board()).queryByRole("region", { name: "Ready" })).toBeNull();
    expect(within(board()).queryByRole("link", { name: /Dropped/ })).toBeNull();
  });

  it("follows the Projects picker", async () => {
    const fake = await world();
    await fake.s.askRound("acme", fake.ids.signup);
    await fake.s.askRound("beta", fake.ids.beta);
    await renderApp(fake, "/?view=board&needs=1");
    within(board()).getByRole("link", { name: /Beta export/ });
    expect(needsTitles()).toEqual(["Sign-up page", "Beta export"]);
    await userEvent.selectOptions(within(board()).getByRole("combobox", { name: "Projects" }), "acme");
    await settled();
    expect(needsTitles()).toEqual(["Sign-up page"]);
  });

  it("reads Nothing needs you when empty", async () => {
    await renderApp(await world(), needsOn);
    loaded();
    expect(needsCol()).toHaveTextContent("Nothing needs you");
  });

  it("opens a card's Focus popup on click", async () => {
    const fake = await world();
    await needing(fake);
    await renderApp(fake, needsOn);
    within(board()).getByRole("link", { name: /Password reset/ });
    await click(within(needsCol()).getByRole("link", { name: /Sign-up page/ }));
    expect(screen.getByRole("dialog", { name: "Focus popup" })).toBeInTheDocument();
  });

  it("moves a card in and out live, pulsing on joining even with no Ticket update", async () => {
    const fake = await world();
    await renderApp(fake, needsOn);
    loaded();
    expect(needsTitles()).toEqual([]);
    await fake.s.askRound("acme", fake.ids.signup);
    expect(needsTitles()).toEqual(["Sign-up page"]);
    expect(within(needsCol()).getByRole("link")).toHaveAttribute("data-pulsing", "true");
    expect(titles("In Progress")).toEqual([]);

    await fake.s.answerRound("acme", fake.ids.signup);
    await fake.s.pickUpAnswers("acme", fake.ids.signup);
    expect(titles("In Progress")).toEqual(["Sign-up page"]);
    expect(needsTitles()).toEqual([]);
  });

  it("doesn't pulse when the column is just shown", async () => {
    const fake = await world();
    await needing(fake);
    await renderApp(fake, "/?view=board&project=acme");
    within(board()).getByRole("link", { name: /Password reset/ });
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Needs Me" }));
    expect(within(needsCol()).getAllByRole("link").filter((c) => c.hasAttribute("data-pulsing"))).toEqual([]);
  });

  describe("By Spec", () => {
    it("has a cell per lane, a collapsed lane's count, and a lane for a Spec with only a hidden needing Ticket", async () => {
      const fake = await world();
      await fake.s.askRound("acme", fake.ids.signup);
      const archive = await fake.s.spec("acme", { title: "Archive" });
      fake.w.advance(MINUTE);
      const cold = await fake.s.ticket("acme", { title: "Cold storage", state: "Ready", spec: archive });
      await fake.s.inReview("acme", cold);
      await renderApp(fake, `${needsOn}&group=spec&hide=Done,Cancelled,In Review`);
      within(board()).getByRole("group", { name: "Lane: Login" });
      const lanes = within(board()).getAllByRole("group", { name: /^Lane: / });
      expect(lanes.map((l) => l.getAttribute("aria-label"))).toEqual(["Lane: Archive", "Lane: Login"]);
      const login = within(board()).getByRole("group", { name: "Lane: Login" });
      expect(within(within(login).getByRole("region", { name: "Needs Me" })).getAllByRole("link")).toHaveLength(1);
      await click(within(login).getByRole("button", { name: "Login" }));
      expect(within(within(login).getByRole("region", { name: "Needs Me" })).getByText("1")).toBeInTheDocument();
    });
  });

  it("counts needing Tickets in hidden States in the Board link while the browser has it on", async () => {
    localStorage.setItem("code-central.board.needs", "1");
    localStorage.setItem("code-central.board.hide", "Done,Cancelled,Failed");
    const fake = await world();
    await fake.s.ticket("acme", { title: "Broken", state: "Failed", spec: fake.ids.spec });
    await renderApp(fake, "/?view=inbox&project=acme");
    expect(boardLink()).toHaveTextContent("Board 4");
  });
});
