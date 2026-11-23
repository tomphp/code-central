import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { Modal } from "./Modal";

function Harness({ onClose = () => {}, size }: { onClose?: () => void; size?: "wide" }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Open</button>
      {open && (
        <Modal
          label="Demo"
          size={size}
          onClose={() => {
            onClose();
            setOpen(false);
          }}
        >
          <button>First</button>
          <select aria-label="Pick" onKeyDown={(e) => e.key === "Escape" && e.preventDefault()}>
            <option>a</option>
          </select>
          <button>Last</button>
        </Modal>
      )}
    </>
  );
}

describe("Modal", () => {
  it("is a named modal dialog, and closes on Escape and on the backdrop", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    await user.click(screen.getByText("Open"));
    expect(screen.getByRole("dialog", { name: "Demo" })).toHaveAttribute("aria-modal", "true");
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
    await user.click(screen.getByText("Open"));
    await user.click(screen.getByRole("dialog").parentElement!);
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("stays open when a press inside the card is released on the backdrop", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    await user.click(screen.getByText("Open"));
    const card = screen.getByRole("dialog");
    const backdrop = card.parentElement!;
    fireEvent.mouseDown(card);
    fireEvent.mouseUp(backdrop);
    fireEvent.click(backdrop);
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("stays open when a press on the backdrop is released inside the card", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    await user.click(screen.getByText("Open"));
    const card = screen.getByRole("dialog");
    const backdrop = card.parentElement!;
    fireEvent.mouseDown(backdrop);
    fireEvent.mouseUp(card);
    fireEvent.click(backdrop);
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("closes on a press and release both on the backdrop", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    await user.click(screen.getByText("Open"));
    const backdrop = screen.getByRole("dialog").parentElement!;
    fireEvent.mouseDown(backdrop);
    fireEvent.mouseUp(backdrop);
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not close on an Escape something inside handled", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    await user.click(screen.getByText("Open"));
    screen.getByLabelText("Pick").focus();
    await user.keyboard("{Escape}");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("moves focus in, traps Tab and gives focus back to the opener", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const opener = screen.getByText("Open");
    await user.click(opener);
    expect(screen.getByRole("dialog")).toHaveFocus();
    await user.tab();
    expect(screen.getByText("First")).toHaveFocus();
    await user.tab();
    await user.tab();
    expect(screen.getByText("Last")).toHaveFocus();
    await user.tab();
    expect(screen.getByText("First")).toHaveFocus();
    await user.tab({ shift: true });
    expect(screen.getByText("Last")).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(opener).toHaveFocus();
  });

  it("locks page scroll while a wide one is open", async () => {
    const user = userEvent.setup();
    render(<Harness size="wide" />);
    await user.click(screen.getByText("Open"));
    expect(document.body.style.overflow).toBe("hidden");
    await user.keyboard("{Escape}");
    expect(document.body.style.overflow).toBe("");
  });
});
