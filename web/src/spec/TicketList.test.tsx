import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { TicketRow } from "../api/types";
import type { ProjectTicket } from "../live";
import { Needing } from "../needsme/fixture";
import { TicketList } from "./TicketList";

const row = (id: string, title: string, state: TicketRow["state"]) =>
  ({ id, title, state, blocked: false, labels: [], updated: "2026-11-07T09:00:00Z" }) as unknown as TicketRow;

const tickets = [row("acme-7", "Add a login page", "In Progress"), row("acme-8", "Rate-limit sign-in", "Ready")];
const now = new Date("2026-11-07T10:00:00Z");

describe("a Spec's Ticket list", () => {
  it("shows each Ticket's State as a chip, and its Reason badge when it needs the human", () => {
    render(
      <Needing tickets={tickets.map((t) => ({ ...t, project: "acme" }) as ProjectTicket)} reasons={{ "acme-7": "silent_claim" }}>
        <TicketList label="Tickets" tickets={tickets} now={now} select={() => {}} />
      </Needing>,
    );
    const [needs, plain] = within(screen.getByRole("list", { name: "Tickets" })).getAllByRole("listitem");
    expect(within(needs).getByText("In Progress").className).toBe("state-chip st-in-progress");
    expect(within(needs).getByText("Silent Claim").className).toBe("reason reason-silent_claim");
    expect(within(plain).getByText("Ready").className).toBe("state-chip st-ready");
    expect(plain.querySelector(".reason")).toBeNull();
  });

  it("selects a Ticket and marks the selected one", async () => {
    const select = vi.fn();
    render(<TicketList label="Tickets" tickets={tickets} now={now} ticket="acme-8" select={select} />);
    expect(screen.getByRole("button", { name: /Rate-limit sign-in/ })).toHaveAttribute("aria-current", "true");
    await userEvent.click(screen.getByRole("button", { name: /Add a login page/ }));
    expect(select).toHaveBeenCalledWith("acme-7");
  });
});
