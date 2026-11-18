// CI items (code-central-432): CI failed, CI cancelled and CI unavailable
// are one Needs Me item per pushed commit, with no Ticket of its own,
// listing the Tickets waiting on it across Specs. On the WASM Core: the
// Inbox row, its list and badge; the Focus Item with the list and the
// failure options, which act on every listed Ticket; and a listed
// Ticket's own view, which links the item.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import { rowDetail } from "./format";
import { allClear, focus, renderApp, scenario, settle } from "./worldTesting";

const COMMIT = "abc1234567";

// world has two Accepted Tickets, one on each of two long-lived Specs
// (whose Tickets land one by one), under Merge + Push with CI Tracking,
// both pushed as abc1234567, whose one run of ci.yml failed: one CI failed
// item listing both. Their IDs are the ones the writes returned.
async function world() {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  const first = await s.spec("acme", { title: "Login", labels: ["long-lived"] });
  const shop = await s.spec("acme", { title: "Shop", labels: ["long-lived"] });
  const login = await s.ticket("acme", { title: "Login page", state: "Accepted", spec: first });
  const basket = await s.ticket("acme", { title: "Basket", state: "Accepted", spec: shop });
  await s.pushed("acme", login, COMMIT);
  await s.pushed("acme", basket, COMMIT);
  await s.ci("acme", login, "failed", COMMIT);
  return { w, s, login, basket, first, shop };
}

describe("a CI item in Needs Me", () => {
  it("is one row per pushed commit with no Ticket, listing the Tickets waiting on it across Specs", async () => {
    const { w, login, basket, first, shop } = await world();
    const item = (await w.api.getTicket("acme", login)).ci_item!;
    expect(item).toMatchObject({ reason: "ci_failed", commit: COMMIT, options: ["rerun_ci", "land_anyway", "new_fix_ticket"] });
    expect(item.tickets).toEqual([
      { id: login, title: "Login page", spec: first },
      { id: basket, title: "Basket", spec: shop },
    ]);
    const row = (await w.api.needsMe()).groups[0].tickets[0];
    expect(rowDetail(w.now(), row)).toBe(`${login}, ${basket} · ci.yml failed`);

    await renderApp(w, "/?view=inbox");
    const group = within(screen.getByRole("main", { name: "Tickets" })).getByRole("region", { name: "CI failed" });
    expect(within(group).getByRole("heading", { level: 3 })).toHaveTextContent("CI failed 1");
    // The row opens on the first Ticket it lists, whose panel shows the item.
    const link = within(group).getByRole("link", { name: /CI failed at abc1234/ });
    expect(link).toHaveAttribute("href", expect.stringContaining(`ticket=${login}`));
    expect(within(link).getByText("acme")).toHaveClass("tid");
    expect(within(link).getByText(`${login}, ${basket} · ci.yml failed`)).toBeInTheDocument();
    expect(within(link).getByText("CI failed", { selector: ".ci-badge" })).toBeInTheDocument();
  });

  it("is one Focus Item whose panel lists the Tickets and whose options act on all of them", async () => {
    const { w, login, basket } = await world();
    await renderApp(w);
    const focusItem = within(focus()).getByRole("article", { name: "Focus Item" });
    expect(within(focusItem).getByRole("heading", { level: 2 })).toHaveTextContent("CI failed at abc1234");
    expect(within(focusItem).getByText("Focus: CI failed")).toBeInTheDocument();
    const panel = within(focusItem).getByRole("region", { name: "CI failed" });
    expect(within(panel).getByText(/listing 2 Tickets/)).toHaveTextContent(`Also waiting: ${basket}`);
    expect(within(panel).getByRole("link", { name: basket })).toHaveAttribute("href", expect.stringContaining(`ticket=${basket}`));
    const options = within(panel).getByRole("group", { name: "Failure options" });
    expect(within(options).queryByRole("button", { name: /Send back to fix/ })).toBeNull();
    await userEvent.click(within(options).getByRole("button", { name: "Land anyway" }));
    await settle(w);
    for (const id of [login, basket]) {
      expect((await w.api.getTicket("acme", id)).land_anyway).toBe(true);
    }
    expect((await w.api.needsMe()).count).toBe(0);
    expect(within(focus()).getByRole("region", { name: "All clear" })).toBeInTheDocument();
  });

  it("is linked from each listed Ticket's own view, which shows its CI result", async () => {
    const { w, login, basket } = await world();
    await renderApp(w, `/?view=inbox&ticket=${basket}`);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(within(pane).getByText("CI failed", { selector: ".reason" })).toBeInTheDocument();
    const panel = within(pane).getByRole("region", { name: "CI failed" });
    expect(within(panel).getByText(/listing 2 Tickets/, { selector: ".ci-item" })).toHaveTextContent(
      `At abc1234, listing 2 Tickets waiting on the commit. Your choice acts on all of them. Also waiting: ${login}.`,
    );
    expect(within(panel).getByRole("link", { name: login })).toHaveAttribute("href", expect.stringContaining(`ticket=${login}`));
    expect(within(within(panel).getByRole("region", { name: "CI" })).getByRole("heading", { level: 3 })).toHaveTextContent("CI failed abc1234");
  });

  it("offers Send back to fix only when it lists one Ticket", async () => {
    const { w, s, login, basket } = await world();
    await s.human(`${s.ticketPath("acme", basket)}/cancel`, {});
    await s.ci("acme", login, "failed", COMMIT, 2);
    const item = (await w.api.getTicket("acme", login)).ci_item!;
    expect(item.tickets.map((t) => t.id)).toEqual([login]);
    expect(item.options).toEqual(["rerun_ci", "land_anyway", "send_back", "new_fix_ticket"]);
  });

  it("shows a CI unavailable item's cause and its Tickets, with no options", async () => {
    const w = await newWorld();
    await allClear(w);
    const s = scenario(w);
    await s.project("acme", "Acme");
    const spec = await s.spec("acme", { labels: ["long-lived"] });
    const login = await s.ticket("acme", { title: "Login page", state: "Accepted", spec });
    const basket = await s.ticket("acme", { title: "Basket", state: "Accepted", spec });
    await s.pushed("acme", login, COMMIT);
    await s.pushed("acme", basket, COMMIT);
    await s.reportCIWatcherStatus("acme", { state: "watching" });
    w.advance(12 * 60_000);
    const row = (await w.api.needsMe()).groups[0].tickets[0];
    expect(rowDetail(w.now(), row)).toBe(`${login}, ${basket} · No CI Runs reported for abc1234 pushed 12m ago`);

    await renderApp(w);
    const focusItem = within(focus()).getByRole("article", { name: "Focus Item" });
    const panel = within(focusItem).getByRole("region", { name: "CI unavailable" });
    expect(within(panel).getByText(/listing 2 Tickets/)).toHaveTextContent(`Also waiting: ${basket}`);
    expect(within(panel).queryByRole("group", { name: "Failure options" })).toBeNull();
  });
});

// Auto-fix's lines in a Ticket's CI section (Spec code-central-1022): Go
// derives which auto fix Ticket is fixing, covering or has stopped, and the
// UI formats it with a link to each fix Ticket.
describe("Auto-fix in a Ticket's CI section", () => {
  const LATER = "def4567890";
  const FIX_PUSH = "ghi7890123";

  async function autoFixWorld(limit: number) {
    const { w, s, login, basket } = await world();
    await s.human(`${s.projectPath("acme")}/set_auto_fix`, { on: true, fix_instructions: "Fix it.", limit });
    // Turning Auto-fix on acts on the item already waiting.
    const fixing = (await w.api.getTicket("acme", login)).ci_auto_fix!.fixing!;
    return { w, s, login, basket, fix: fixing.fix_ticket };
  }

  const ciSection = () => within(screen.getByRole("complementary", { name: "Ticket" })).getByRole("region", { name: "CI" });

  it("shows the auto fix Ticket fixing it, as attempt n of N, linked", async () => {
    const { w, login, fix } = await autoFixWorld(3);
    await renderApp(w, `/?view=inbox&ticket=${login}`);
    const line = within(ciSection()).getByText(/Auto-fix is fixing it/);
    expect(line).toHaveTextContent(`Auto-fix is fixing it: ${fix} (attempt 1 of 3).`);
    expect(within(line).getByRole("link", { name: fix })).toHaveAttribute("href", expect.stringContaining(`ticket=${fix}`));
  });

  it("shows the auto fix Ticket covering a later commit, with the commit it was opened for", async () => {
    const { w, s, basket, fix } = await autoFixWorld(3);
    await s.pushed("acme", basket, LATER);
    await s.ci("acme", basket, "failed", LATER, 2);
    await renderApp(w, `/?view=inbox&ticket=${basket}`);
    const line = within(ciSection()).getByText(/Auto-fix covers it/);
    expect(line).toHaveTextContent(`Auto-fix covers it: ${fix}, opened for abc1234.`);
    expect(within(line).getByRole("link", { name: fix })).toHaveAttribute("href", expect.stringContaining(`ticket=${fix}`));
  });

  it("shows the stopped line with each attempt linked, once the limit is spent", async () => {
    const { w, s, login, fix } = await autoFixWorld(1);
    await s.accepted("acme", fix);
    await s.pushed("acme", fix, FIX_PUSH);
    await s.pushed("acme", login, FIX_PUSH);
    await s.ci("acme", fix, "failed", FIX_PUSH, 2);
    await renderApp(w, `/?view=inbox&ticket=${login}`);
    const line = within(ciSection()).getByText(/Auto-fix stopped/);
    expect(line).toHaveTextContent(`Auto-fix stopped after 1 attempt: ${fix}.`);
    expect(within(line).getByRole("link", { name: fix })).toHaveAttribute("href", expect.stringContaining(`ticket=${fix}`));
  });

  it("shows no line while Auto-fix is off", async () => {
    const { w, login } = await world();
    await renderApp(w, `/?view=inbox&ticket=${login}`);
    expect(within(ciSection()).queryByText(/Auto-fix/)).toBeNull();
  });
});
