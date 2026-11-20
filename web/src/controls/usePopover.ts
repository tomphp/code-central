import { useEffect, useRef, useState, type KeyboardEvent } from "react";

// usePopover owns a popover's open state: a mousedown outside its wrapper
// closes it, and Esc closes it and returns focus to its button. Spread
// wrapProps on the wrapper that holds both the button and the popover.
export function usePopover() {
  const [open, setOpen] = useState(false);
  const wrap = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!wrap.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Escape" || !open) return;
    setOpen(false);
    button.current?.focus();
  };
  return { open, setOpen, button, wrapProps: { ref: wrap, onKeyDown } };
}
