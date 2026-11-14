// A Spec's Ticket list on a WASM Core world: each Ticket's State chip, its
// Reason badge from the Needs Me read, and selecting a Ticket.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld } from "../api/core/core";
import { scenario } from "../api/scenario";
import { STATES, type NeedsMeReason } from "../api/types";
import { Needing } from "../needsme/fixture";
import { TicketList } from "./TicketList";

// world is a Spec with a Ticket waiting on its Questions and a Ready one,
// read back as the Spec's view reads them, with Core's Needs Me reasons.
async function world() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const spec = await s.spec("acme", { title: "Theming" });
  const asking = await s.ticket("acme", { title: "Pick a theme", spec, state: "Needs Answer" });
  const ready = await s.ticket("acme", { title: "Add a dark palette", spec });
  const tickets = (await w.api.findTickets("acme", { state: [...STATES] })).filter((t) => t.spec === spec);
  const reasons: Record<string, NeedsMeReason> = {};
  for (const g of (await w.api.needsMe()).groups) for (const r of g.tickets) reasons[r.id] = g.reason;
  return { w, asking, ready, tickets, reasons };
}

describe("a Spec's Ticket list", () => {
  it("shows each Ticket's State as a chip, and its Reason badge when it needs the human", async () => {
    const { w, asking, ready, tickets, reasons } = await world();
    render(
      <Needing tickets={tickets.map((t) => ({ ...t, project: "acme" }))} reasons={reasons}>
        <TicketList label="Tickets" tickets={tickets} now={w.now()} select={() => {}} />
      </Needing>,
    );
    const item = (id: string) => within(screen.getByRole("list", { name: "Tickets" })).getAllByRole("listitem").find((li) => li.textContent!.startsWith(id))!;
    expect(item(asking)).toHaveTextContent(`${asking} Pick a theme Needs Answer Questions`);
    expect(within(item(asking)).getByText("Needs Answer")).toHaveClass("state-chip", "st-needs-answer");
    expect(within(item(asking)).getByText("Questions")).toHaveClass("reason", "reason-needs_answer");
    expect(within(item(ready)).getByText("Ready")).toHaveClass("state-chip", "st-ready");
    expect(item(ready).querySelector(".reason")).toBeNull();
  });

  it("selects a Ticket and marks the selected one", async () => {
    const { w, asking, ready, tickets } = await world();
    const select = vi.fn();
    render(<TicketList label="Tickets" tickets={tickets} now={w.now()} ticket={ready} select={select} />);
    expect(screen.getByRole("button", { name: /Add a dark palette/ })).toHaveAttribute("aria-current", "true");
    await userEvent.click(screen.getByRole("button", { name: /Pick a theme/ }));
    expect(select).toHaveBeenCalledWith(asking);
  });
});
