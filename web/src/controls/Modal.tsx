// Modal is the shell every dialog and popup shares: it portals to the body,
// draws the backdrop and closes on a backdrop click or Escape, unless
// something inside already handled that Escape (an open menu calls
// preventDefault on it). The card is the dialog: focus moves in on open, Tab
// stays inside it and focus goes back to the opener on close. "narrow" is the
// 40rem dialog; "wide" is for popups, capped to the viewport, with its content
// scrolling inside it while the page behind stays put.
import { useEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";

export interface ModalProps {
  // labelledBy is the ID of the element inside that names the dialog; label
  // names it directly.
  labelledBy?: string;
  label?: string;
  size?: "narrow" | "wide";
  onClose(): void;
  children: ReactNode;
}

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function Modal({
  labelledBy,
  label,
  size = "narrow",
  onClose,
  children,
}: ModalProps) {
  const card = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  close.current = onClose;

  useEffect(() => {
    const opener = document.activeElement;
    const el = card.current;
    if (el && !el.contains(document.activeElement)) el.focus();
    return () => {
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
    };
  }, []);

  useEffect(() => {
    if (size !== "wide") return;
    const before = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = before;
    };
  }, [size]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented) close.current();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const trap = (e: React.KeyboardEvent) => {
    if (e.key !== "Tab" || !card.current) return;
    const items = Array.from(
      card.current.querySelectorAll<HTMLElement>(FOCUSABLE),
    );
    const first = items[0];
    const last = items[items.length - 1];
    const at = document.activeElement;
    if (!first || !last) {
      e.preventDefault();
      card.current.focus();
    } else if (e.shiftKey && (at === first || at === card.current)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && at === last) {
      e.preventDefault();
      first.focus();
    }
  };

  return createPortal(
    <div
      className="modal"
      onClick={(e) => e.target === e.currentTarget && close.current()}
    >
      <div
        ref={card}
        className={`modal-card${size === "wide" ? " modal-wide" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        aria-label={label}
        tabIndex={-1}
        onKeyDown={trap}
      >
        {children}
      </div>
    </div>,
    document.body,
  );
}
