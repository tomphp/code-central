import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TicketCard } from "./TicketCard";

const t = { id: "acme-7", title: "Add a login page", blocked: false, updated: "2026-11-07T09:00:00Z" };
const link = { href: "/acme-7", onClick: () => {} };

describe("TicketCard", () => {
  it("is one link holding the Ticket's ID and title, with nothing optional", () => {
    render(<TicketCard t={t} link={link} />);
    const card = screen.getByRole("link");
    expect(card.className).toBe("board-card");
    expect(card.getAttribute("href")).toBe("/acme-7");
    expect(screen.getByRole("button", { name: "acme-7" })).toBeTruthy();
    expect(screen.getByText("Add a login page").className).toBe("board-card-title");
    expect(card.querySelector(".stage-badge, .board-blocked, .board-card-spec")).toBeNull();
    expect(screen.queryByRole("img", { name: "Needs you" })).toBeNull();
  });

  it("shows its kind, Blocked, the needs-you dot and its Spec", () => {
    render(<TicketCard t={{ ...t, blocked: true }} link={link} badge="Prototype" reason="in_review" spec={{ kind: "Map", title: "Sign-in" }} />);
    const card = screen.getByRole("link");
    expect(card.className).toBe("board-card needs reason-in_review");
    expect(screen.getByText("Prototype").className).toBe("stage-badge");
    expect(screen.getByTitle("Has open Blockers").textContent).toBe("⛔ Blocked");
    expect(screen.getByRole("img", { name: "Needs you" }).className).toBe("dot");
    expect(card.querySelector(".board-card-spec")!.textContent).toBe("Map Sign-in");
  });

  it("pulses when it joins its column", () => {
    render(<TicketCard t={t} link={link} joined />);
    const card = screen.getByRole("link");
    expect(card.getAttribute("data-pulsing")).toBe("true");
    expect(card.className).toBe("board-card dashboard-pulse-1");
  });

  it("pulses when its Ticket is updated", () => {
    const { rerender } = render(<TicketCard t={t} link={link} />);
    expect(screen.getByRole("link").hasAttribute("data-pulsing")).toBe(false);
    rerender(<TicketCard t={{ ...t, updated: "2026-11-07T09:01:00Z" }} link={link} />);
    expect(screen.getByRole("link").getAttribute("data-pulsing")).toBe("true");
  });
});
