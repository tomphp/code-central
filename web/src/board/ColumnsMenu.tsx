// Columns ▾: the Board view's menu that shows or hides each State's column,
// one checkbox per State in workflow order. Its button reads "Columns",
// with how many of the columns show appended while any is hidden.
import { useEffect, useId, useRef, useState } from "react";
import { STATES, type State } from "../api/types";
import { Button } from "../controls/Button";
import { stateSlug } from "../controls/StateChip";

export interface ColumnsMenuProps {
  hidden: readonly State[];
  onChange(hidden: State[]): void;
}

export function ColumnsMenu({ hidden, onChange }: ColumnsMenuProps) {
  const [open, setOpen] = useState(false);
  const wrap = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const id = useId();

  // A click outside it closes it; Esc closes it and returns focus to its button.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!wrap.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const toggle = (state: State) =>
    onChange(STATES.filter((s) => (s === state ? !hidden.includes(s) : hidden.includes(s))));

  return (
    <div
      className="board-columns"
      ref={wrap}
      onKeyDown={(e) => {
        if (e.key !== "Escape" || !open) return;
        setOpen(false);
        button.current?.focus();
      }}
    >
      <Button ref={button}  aria-expanded={open} aria-controls={id} onClick={() => setOpen((o) => !o)}>
        Columns
        {hidden.length > 0 && <span className="muted"> · {STATES.length - hidden.length}/{STATES.length}</span>} ▾
      </Button>
      <div id={id} role="group" aria-label="Columns" className="board-columns-menu" hidden={!open}>
        {STATES.map((s) => (
          <label key={s}>
            <input type="checkbox" checked={!hidden.includes(s)} onChange={() => toggle(s)} />
            <span className={`board-swatch st-${stateSlug(s)}`} aria-hidden="true" />
            {s}
          </label>
        ))}
      </div>
    </div>
  );
}
