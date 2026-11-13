// Modal is the shell every dialog and popup shares: it portals to the body,
// draws the backdrop and closes on a backdrop click or Escape, unless
// something inside already handled that Escape (an open menu calls
// preventDefault on it). The card is the dialog: focus moves in on open, Tab
// stays inside it and focus goes back to the opener on close. "narrow" is the
// 40rem dialog; "wide" is for popups, capped to the viewport, with its content
// scrolling inside it while the page behind stays put. While any Modal is
// open, unless uncounted, useModalOpen says so: the automatic Focus popup
// waits on it.
import { useEffect, useRef, useSyncExternalStore, type ReactNode } from "react";
import { createPortal } from "react-dom";

export interface ModalProps {
  // labelledBy is the ID of the element inside that names the dialog; label
  // names it directly.
  labelledBy?: string;
  label?: string;
  size?: "narrow" | "wide";
  // uncounted leaves the Modal out of useModalOpen: the automatic Focus
  // popup's own, which doesn't wait on itself.
  uncounted?: boolean;
  onClose(): void;
  children: ReactNode;
}

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function Modal({
  labelledBy,
  label,
  size = "narrow",
  uncounted = false,
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

  useEffect(() => (uncounted ? undefined : opened()), [uncounted]);

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

// openCount is how many counted Modals are open now, and listeners who to
// tell when that changes.
let openCount = 0;
const listeners = new Set<() => void>();

// opened counts a Modal open and returns what counts it closed.
function opened(): () => void {
  openCount++;
  listeners.forEach((l) => l());
  return () => {
    openCount--;
    listeners.forEach((l) => l());
  };
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

// modalOpen is whether any counted Modal is open anywhere in the page.
export function modalOpen(): boolean {
  return openCount > 0;
}

// useModalOpen is modalOpen, rendering again as it changes.
export function useModalOpen(): boolean {
  return useSyncExternalStore(subscribe, modalOpen);
}
