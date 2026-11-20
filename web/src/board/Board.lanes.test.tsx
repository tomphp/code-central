// Board view tests: grouping, the By Spec lanes and the Spec popup.
import { fireEvent, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { newWorld } from "../api/core/core";
import { WORK } from "../api/scenario";
import { scenario } from "../worldTesting";
import { MINUTE, Fake, world, renderApp, click, travel, press, board, heads, cards, card, loaded, noPopup } from "./Board.testutil";

beforeEach(() => localStorage.clear());

describe("the Board's grouping", () => {
  it("is By Ticket by default, and shares its pick with the State lists", async () => {
    const fake = await world();
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    loaded();
    const toggle = within(board()).getByRole("group", { name: "Group the board" });
    expect(within(toggle).getByRole("button", { name: "Ticket" })).toHaveAttribute("aria-pressed", "true");
    await click(within(toggle).getByRole("button", { name: "Spec" }));
    expect(within(toggle).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");
    expect(new URLSearchParams(window.location.search).get("group")).toBe("spec");
    unmount();

    const inbox = await renderApp(fake, "/?view=inbox&project=acme&state=Ready");
    const list = within(screen.getByRole("main", { name: "Tickets" })).getByRole("group", { name: "Group the list" });
    expect(within(list).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");
    await click(within(list).getByRole("button", { name: "Ticket" }));
    inbox.unmount();

    await renderApp(fake, "/?view=board&project=acme");
    const again = within(board()).getByRole("group", { name: "Group the board" });
    expect(within(again).getByRole("button", { name: "Ticket" })).toHaveAttribute("aria-pressed", "true");
  });
});

describe("the Board's By Spec lanes", () => {
  // lanesWorld makes a second Spec, newer than Login, with a Ready and an In Progress Ticket,
  // and a third whose only Ticket is Done.
  async function lanesWorld() {
    const fake = await world();
    const { s, w } = fake;
    const billing = await s.spec("acme", { title: "Billing" }); // acme-7
    w.advance(MINUTE);
    await s.ticket("acme", { title: "Invoice list", state: "Ready", spec: billing }); // acme-8
    w.advance(MINUTE);
    await s.ticket("acme", { title: "Refunds", state: "In Progress", spec: billing }); // acme-9
    w.advance(MINUTE);
    const archive = await s.spec("acme", { title: "Archive" }); // acme-10
    w.advance(MINUTE);
    await s.ticket("acme", { title: "Cold storage", state: "Done", spec: archive }); // acme-11
    return fake;
  }
  const lanes = () => within(board()).getAllByRole("group", { name: /^Lane: / }).map((l) => l.getAttribute("aria-label"));
  const lane = (title: string) => within(board()).getByRole("group", { name: `Lane: ${title}` });
  const bySpec = async (url = "/?view=board&project=acme&group=spec") => renderApp(await lanesWorld(), url);

  it("shows one lane per Spec with a Ticket in a shown column, newest Spec first, and no lanes By Ticket", async () => {
    await bySpec();
    loaded();
    expect(lanes()).toEqual(["Lane: Billing", "Lane: Login"]);
    await click(within(within(board()).getByRole("group", { name: "Group the board" })).getByRole("button", { name: "Ticket" }));
    expect(within(board()).queryAllByRole("group", { name: /^Lane: / })).toEqual([]);
    expect(cards("Ready")).toHaveLength(3);
  });

  it("adds a lane when its Spec's Done Tickets are shown with Columns ▾", async () => {
    await bySpec("/?view=board&project=acme&group=spec&hide=");
    loaded();
    expect(lanes()).toEqual(["Lane: Archive", "Lane: Billing", "Lane: Login"]);
  });

  it("heads each lane with its Lane header, which opens the Spec's popup", async () => {
    await bySpec();
    loaded();
    const row = within(lane("Login")).getByRole("region", { name: "Login" });
    // One line: the count of the Tickets the lane shows, and no State chips.
    expect(within(row).getByText("3 shown")).toBeInTheDocument();
    expect(row.querySelector(".state-chips, .strip")).toBeNull();
    expect(within(row).getByRole("link")).toHaveAttribute("href", "?view=board&project=acme&group=spec&popup=acme-1");
    await click(within(row).getByRole("link"));
    expect(screen.getByRole("dialog", { name: "Spec popup" })).toBeInTheDocument();
    expect(window.location.search).toBe("?view=board&project=acme&group=spec&popup=acme-1");
  });

  it("badges each Lane header with its Spec's kind, and heads a Spec a Factory is on with that", async () => {
    const fake = await lanesWorld();
    await fake.w.api.addLabels("acme", "acme-7", ["long-lived"]);
    await fake.w.api.submitMap("acme", "Plan the year"); // acme-12, its Charting Ticket Ready
    await renderApp(fake, "/?view=board&project=acme&group=spec");
    loaded();
    const head = (title: string) => within(lane(title)).getByRole("region", { name: title });
    expect(within(head("Login")).getByText("Spec")).toHaveClass("kind-badge", "kind-spec");
    expect(within(head("Billing")).getByText("Long-lived")).toHaveClass("kind-badge", "kind-long-lived");
    expect(within(head("Plan the year")).getByText("Map")).toHaveClass("kind-badge", "kind-map");
    // Refunds is In Progress, so a Factory holds its Claim.
    expect(head("Billing")).toHaveTextContent("A Factory is on it · 0 of 2 Done");
  });

  it("puts each Ticket's card in its lane's column, without the Spec line", async () => {
    await bySpec();
    loaded();
    const ready = within(lane("Login")).getByRole("region", { name: "Ready" });
    expect(within(ready).getAllByRole("link").map((c) => c.querySelector(".board-card-title")?.textContent)).toEqual(["Password reset", "Login form"]);
    const billingReady = within(lane("Billing")).getByRole("region", { name: "Ready" });
    expect(within(billingReady).getByRole("link", { name: /Invoice list/ })).not.toHaveTextContent("Billing");
    expect(within(lane("Login")).getByRole("link", { name: /Login form/ }).querySelector(".board-card-spec")).toBeNull();
  });

  it("counts a column's Tickets across all lanes in its head", async () => {
    await bySpec();
    loaded();
    expect(heads()).toContain("Ready 3");
    expect(heads()).toContain("In Progress 2");
    expect(heads()).toContain("Draft 0");
  });

  it("collapses a lane to its header, keeping its per-column counts and the head totals", async () => {
    await bySpec();
    loaded();
    const toggle = within(lane("Login")).getByRole("button", { name: "Login" });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    await click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(within(lane("Login")).queryAllByRole("link", { name: /Login form|Sign-up page|Password reset/ })).toEqual([]);
    expect(within(within(lane("Login")).getByRole("region", { name: "Ready" })).getByText("2")).toBeInTheDocument();
    expect(within(within(lane("Login")).getByRole("region", { name: "In Progress" })).getByText("1")).toBeInTheDocument();
    expect(within(lane("Billing")).getByRole("link", { name: /Invoice list/ })).toBeInTheDocument();
    expect(heads()).toContain("Ready 3");
    await click(toggle);
    expect(within(lane("Login")).getByRole("link", { name: /Login form/ })).toBeInTheDocument();
  });

  it("keeps folded lanes across a card's popup opening and closing", async () => {
    await bySpec();
    loaded();
    await click(within(lane("Billing")).getByRole("button", { name: "Billing" }));
    await click(within(lane("Login")).getByText("Login form"));
    screen.getByRole("dialog", { name: "Ticket popup" });
    await press("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(within(lane("Billing")).getByRole("button", { name: "Billing" })).toHaveAttribute("aria-expanded", "false");
    expect(within(lane("Billing")).queryByRole("link", { name: /Invoice list/ })).toBeNull();
  });

  it("follows a Ticket moving column inside its lane", async () => {
    const fake = await lanesWorld();
    await renderApp(fake, "/?view=board&project=acme&group=spec");
    loaded();
    await fake.s.claimFor("acme", fake.ids.form);
    expect(within(within(lane("Login")).getByRole("region", { name: "In Progress" })).getByRole("link", { name: /Login form/ })).toBeInTheDocument();
    expect(heads()).toContain("In Progress 3");
  });
});

describe("the Board's Spec popup", () => {
  const popup = () => screen.getByRole("dialog", { name: "Spec popup" });
  const lane = (title: string) => within(board()).getByRole("group", { name: `Lane: ${title}` });
  const header = (title: string) => within(within(lane(title)).getByRole("region", { name: title })).getByRole("link");
  const openLane = async (title: string) => {
    await click(header(title));
    return popup();
  };
  const bySpec = "/?view=board&project=acme&group=spec&hide=Done";
  const specPopup = popup;

  // finishedMap makes a Map whose every Ticket is Done: acme-1, its Ticket acme-2.
  async function finishedMap() {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const body = ["Plan the year", "## Destination", "A plan.", "## Decisions so far", "## Not yet specified", "## Out of scope"].join("\n\n");
    const map = await w.api.submitMap("acme", body);
    await s.accepted("acme", map.charting.id);
    await s.human(`${s.ticketPath("acme", map.charting.id)}/land`, { commit: WORK.commit });
    return { w, s, map: map.spec.id };
  }

  it("opens from a Lane header with the Spec's pane and its Tickets, and its chevron only folds", async () => {
    const fake = await world();
    await renderApp(fake, bySpec);
    loaded();
    const chevron = () => within(lane("Login")).getByRole("button", { name: "Login" });
    await click(chevron());
    await noPopup();
    expect(chevron()).toHaveAttribute("aria-expanded", "false");
    await click(chevron());
    await noPopup();
    await openLane("Login");
    expect(within(popup()).getByRole("region", { name: "Spec" })).toBeInTheDocument();
    expect(within(popup()).getByRole("heading", { name: "Login" })).toBeInTheDocument();
    expect(within(popup()).getByRole("button", { name: "Close" })).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&hide=Done&popup=${fake.ids.spec}`);
    // The lane stays as it was behind it.
    expect(chevron()).toHaveAttribute("aria-expanded", "true");
  });

  it("offers New Ticket through the pane's ticketable flag, and submits Ready and Draft", async () => {
    const fake = await world();
    expect((await fake.w.api.getSpec("acme", fake.ids.spec)).ticketable).toBe(true);

    for (const [text, button, state] of [["Rename Start", "Submit ticket", "Ready"], ["Maybe later", "Save as Draft", "Draft"]] as const) {
      // A submitted Ticket is selected, which turns the popup into its Ticket popup.
      const { unmount } = await renderApp(fake, `${bySpec}&popup=${fake.ids.spec}`);
      loaded();
      await click(within(popup()).getByRole("button", { name: "New Ticket" }));
      const modal = screen.getByRole("dialog", { name: "New Ticket" });
      fireEvent.change(within(modal).getByRole("textbox", { name: "Ticket" }), { target: { value: text } });
      await click(within(modal).getByRole("button", { name: button }));
      const made = (await fake.w.api.findTickets("acme")).find((t) => t.title === text)!;
      expect(made.state).toBe(state);
      unmount();
    }
  });

  it("offers no New Ticket for a Long-lived Spec", async () => {
    const fake = await world();
    const longLived = await fake.s.spec("acme", { title: "Chores", labels: ["long-lived"] });
    await renderApp(fake, `/?view=board&project=acme&group=spec&popup=${longLived}`);
    loaded();
    expect(within(popup()).getByRole("region", { name: "Spec" })).toBeInTheDocument();
    expect(within(popup()).queryByRole("button", { name: "New Ticket" })).toBeNull();
  });

  it("leaves a modified click on a Lane header to the browser", async () => {
    const fake = await world();
    await renderApp(fake, bySpec);
    loaded();
    expect(header("Login")).toHaveAttribute("href", `?view=board&project=acme&group=spec&hide=Done&popup=${fake.ids.spec}`);
    fireEvent.click(header("Login"), { metaKey: true });
    fireEvent.click(header("Login"), { button: 1 });
    await noPopup();
    expect(window.location.search).toBe(bySpec.slice(1));
  });

  it("shows a Requirement's pane even when one of its Stage Tickets needs the human", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const { spec, stages } = await w.api.submitRequirement("acme", "Offline mode");
    await s.askRound("acme", stages[0].id);
    await renderApp({ w, s } as Fake, "/?view=board&project=acme&group=spec");
    within(board()).getByRole("group", { name: "Lane: Offline mode" });
    await click(header("Offline mode"));
    expect(within(specPopup()).getByRole("region", { name: "Requirement" })).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&popup=${spec.id}`);
  });

  it("shows a Map's pane", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const { spec } = await w.api.submitMap("acme", "Plan the year");
    await renderApp({ w, s } as Fake, "/?view=board&project=acme&group=spec");
    within(board()).getByRole("group", { name: "Lane: Plan the year" });
    await click(header("Plan the year"));
    const dialog = specPopup();
    expect(within(dialog).getByRole("region", { name: "Map" })).toBeInTheDocument();
    expect(within(dialog).getByRole("heading", { name: "Plan the year", level: 2 })).toBeInTheDocument();
    expect(window.location.search).toContain(`popup=${spec.id}`);
  });

  it("switches to a Ticket's popup when a Ticket is chosen in it, and Back returns to the Spec popup", async () => {
    const fake = await world();
    await renderApp(fake, bySpec);
    loaded();
    await openLane("Login");
    await click(within(within(popup()).getByRole("list", { name: "Tickets" })).getByRole("button", { name: /Login form/ }));
    const ticket = screen.getByRole("dialog", { name: "Ticket popup" });
    expect(within(ticket).getByRole("heading", { name: /Login form/ })).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Spec popup" })).toBeNull();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&hide=Done&popup=${fake.ids.form}`);
    await travel(() => window.history.back());
    expect(specPopup()).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Ticket popup" })).toBeNull();
    // No Ticket shows as selected.
    expect(popup().querySelector("[aria-current]")).toBeNull();
  });

  it("switches to the Spec's popup from a Ticket popup's Spec link", async () => {
    const fake = await world();
    await renderApp(fake, bySpec);
    loaded();
    await click(within(card("Login form")).getByText("Login form"));
    screen.getByRole("dialog", { name: "Ticket popup" });
    await click(screen.getByRole("button", { name: `${fake.ids.spec} Login` }));
    expect(specPopup()).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&hide=Done&popup=${fake.ids.spec}`);
  });

  it("switches to the Map's popup from a Requirement made from it", async () => {
    const fake = await finishedMap();
    const { spec } = await fake.w.api.submitRequirement("acme", "Offline mode", fake.map); // acme-3
    await renderApp(fake as unknown as Fake, `/?view=board&project=acme&group=spec&popup=${spec.id}`);
    await click(within(specPopup()).getByRole("link", { name: /^Map/ }));
    expect(within(specPopup()).getByRole("region", { name: "Map" })).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&popup=${fake.map}`);
  });

  it("switches to the new Requirement's popup after Write the Spec", async () => {
    const fake = await finishedMap();
    await renderApp(fake as unknown as Fake, `/?view=board&project=acme&group=spec&popup=${fake.map}`);
    await click(within(specPopup()).getByRole("button", { name: "Write the Spec" }));
    await click(within(screen.getByRole("dialog", { name: "Write the Spec" })).getByRole("button", { name: "Write the Spec" }));
    expect(window.location.search).toContain("popup=acme-3");
    expect(within(specPopup()).getByRole("region", { name: "Requirement" })).toBeInTheDocument();
  });

  it.each([
    ["×", () => click(within(popup()).getByRole("button", { name: "Close" }))],
    ["Escape", () => press("{Escape}")],
    ["the backdrop", () => click(popup().parentElement!)],
    ["Back", () => travel(() => window.history.back())],
  ])("closes on %s, keeping the grouping, hidden columns and folded lanes", async (_, close) => {
    await renderApp(await world(), bySpec);
    loaded();
    await click(within(lane("Login")).getByRole("button", { name: "Login" }));
    await openLane("Login");
    await close();
    await noPopup();
    expect(window.location.search).toBe(bySpec.slice(1));
    expect(within(lane("Login")).getByRole("button", { name: "Login" })).toHaveAttribute("aria-expanded", "false");
    expect(within(board()).queryByRole("region", { name: "Done" })).toBeNull();
  });
});
