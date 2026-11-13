import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { LatestEvent, LinkView } from "../api/types";
import { TicketLine, TicketName } from "./TicketLine";

const now = new Date("2026-11-07T09:00:00Z");
const t = { id: "acme-7", title: "Add a login page", state: "Ready" as const };
const link = { href: "/acme-7", onClick: () => {}, "data-id": "acme-7" };
const issue: LinkView = { integration: "GitHub", id: "#12", url: "https://x/12", external_state: "open", detached: false, outbound: false };
const event: LatestEvent = { kind: "tool_call", text: "Running tests", time: "2026-11-07T08:55:00Z" };

describe("TicketLine", () => {
  it("is one link holding the Ticket's ID and title", () => {
    render(<TicketLine t={t} link={link} />);
    const row = screen.getByRole("link");
    expect(row.className).toBe("row");
    expect(row.getAttribute("href")).toBe("/acme-7");
    expect(screen.getByRole("button", { name: "acme-7" })).toBeTruthy();
    expect(screen.getByText("Add a login page").className).toBe("row-title");
  });

  it("leaves every optional part out by default", () => {
    render(<TicketLine t={t} link={link} />);
    const row = screen.getByRole("link");
    expect(row.querySelector(".stage-badge, .state-chip, .link-badge, .waiting-badge, .row-detail")).toBeNull();
    expect(screen.queryByRole("img", { name: "Needs you" })).toBeNull();
  });

  it("shows the Stage badge, State chip, Link and Waiting badges, added badges and needs-you dot, in that order", () => {
    const linked = { ...t, link: issue, waiting_for_spec: true };
    render(
      <TicketLine t={linked} link={link} badge="Spec" chip needsYou>
        <span className="extra">Extra</span>
      </TicketLine>,
    );
    const top = screen.getByRole("link").firstElementChild!;
    expect([...top.children].map((c) => c.className)).toEqual([
      "tid short-id",
      "stage-badge",
      "row-title",
      "state-chip st-ready",
      "label link-badge",
      "label waiting-badge",
      "extra",
      "dot needs-you",
    ]);
    expect(screen.getByText("Spec")).toBeTruthy();
    expect(screen.getByText("Ready")).toBeTruthy();
    expect(screen.getByText("GitHub #12")).toBeTruthy();
    expect(screen.getByRole("img", { name: "Needs you" })).toBeTruthy();
  });

  it("shows an In Progress Ticket's latest Session Event on its second line", () => {
    const working = { ...t, state: "In Progress" as const, latest_event: event };
    render(<TicketLine t={working} link={link} now={now} />);
    expect(screen.getByText("Running tests · 5m ago").className).toBe("latest-event row-detail");
  });

  it("shows the list's detail in place of the latest event", () => {
    const working = { ...t, state: "In Progress" as const, latest_event: event };
    render(<TicketLine t={working} link={link} now={now} detail="Of Spec acme-1" />);
    expect(screen.getByText("Of Spec acme-1").className).toBe("row-detail");
    expect(screen.queryByText(/Running tests/)).toBeNull();
  });

  it("dims a quiet row", () => {
    render(<TicketLine t={t} link={link} quiet />);
    expect(screen.getByRole("link").className).toBe("row quiet");
  });
});

describe("TicketName", () => {
  it("is the Ticket's ID and title inline", () => {
    const { container } = render(<TicketName t={t} />);
    expect(container.textContent).toBe("acme-7 Add a login page");
    expect(screen.getByRole("button", { name: "acme-7" })).toBeTruthy();
  });
});
