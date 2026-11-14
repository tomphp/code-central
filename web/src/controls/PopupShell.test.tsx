import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PopupShell } from "./PopupShell";

describe("PopupShell", () => {
  it("shows its title as the heading that names it", () => {
    render(
      <PopupShell title="New Map" onClose={() => {}}>
        <p>Body</p>
      </PopupShell>,
    );
    const popup = screen.getByRole("dialog", { name: "New Map" });
    expect(within(popup).getByRole("heading", { name: "New Map" })).toBeInTheDocument();
    expect(within(popup).getByText("Body")).toBeInTheDocument();
  });

  it("is named by its label when what it holds carries the heading", () => {
    render(
      <PopupShell label="Ticket popup" onClose={() => {}}>
        <h2>acme-1 Sign-in</h2>
      </PopupShell>,
    );
    const popup = screen.getByRole("dialog", { name: "Ticket popup" });
    expect(within(popup).getAllByRole("heading").map((h) => h.textContent)).toEqual(["acme-1 Sign-in"]);
  });

  it("closes on ×", async () => {
    const onClose = vi.fn();
    render(
      <PopupShell title="New Map" onClose={onClose}>
        <p>Body</p>
      </PopupShell>,
    );
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("shows what else its head holds", () => {
    render(
      <PopupShell label="Focus popup" head={<span>3 left</span>} onClose={() => {}}>
        <p>Body</p>
      </PopupShell>,
    );
    expect(within(screen.getByRole("dialog")).getByText("3 left")).toBeInTheDocument();
  });

  it("puts its actions after what it holds", () => {
    render(
      <PopupShell title="New Map" actions={<button>Submit map</button>} onClose={() => {}}>
        <p>Body</p>
      </PopupShell>,
    );
    const popup = screen.getByRole("dialog");
    const submit = within(popup).getByRole("button", { name: "Submit map" });
    const body = within(popup).getByText("Body");
    expect(body.compareDocumentPosition(submit) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("has no actions unless given some", () => {
    render(
      <PopupShell title="New Map" onClose={() => {}}>
        <p>Body</p>
      </PopupShell>,
    );
    expect(within(screen.getByRole("dialog")).getAllByRole("button").map((b) => b.getAttribute("aria-label"))).toEqual(["Close"]);
  });

  it("holds the page still behind it when wide", () => {
    const { unmount } = render(
      <PopupShell label="Spec popup" size="wide" onClose={() => {}}>
        <p>Body</p>
      </PopupShell>,
    );
    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("");
  });

  it("is narrow unless asked, leaving the page to scroll", () => {
    render(
      <PopupShell title="New Map" onClose={() => {}}>
        <p>Body</p>
      </PopupShell>,
    );
    expect(document.body.style.overflow).toBe("");
  });
});
