// The split "+ New ▾": "+ New" opens New requirement, the main action; its
// "▾" opens a menu of Requirement, Map and Small Change, so every kind is discoverable.
// The menu stays mounted while closed, so a modal opened from one of its
// items outlives it. The ▾ and its menu follow the usual menu-button
// keyboard pattern. The menu is placed against the nearest positioned
// ancestor, which the caller provides.
import { type KeyboardEvent as ReactKeyboardEvent, useEffect, useId, useRef, useState } from "react";
import { Button } from "../controls/Button";
import type { SpecAcknowledgement, TicketAcknowledgement } from "../api/types";
import { NewMap, NewRequirement, NewSmallChange, type NewRequirementProps } from "./NewRequirement";

// NewKind is what + New ▾ can make.
export type NewKind = "Requirement" | "Map" | "Small Change";

// Made is what a submit resolves to, keyed by its kind: a Requirement or
// Map carries its Spec, a Small Change its Ticket.
export type Made = { kind: "Requirement" | "Map"; spec: SpecAcknowledgement } | { kind: "Small Change"; ticket: TicketAcknowledgement };

export interface NewSplitProps extends Omit<NewRequirementProps, "trigger" | "onSubmitted"> {
  // variant styles both halves of the split.
  variant: "primary" | "plain";
  // onSubmitted is told which kind was made, with its acknowledgement.
  onSubmitted(r: Made): void;
}

// NewSplit is + New, its ▾ and the ▾'s menu.
export function NewSplit({ variant, onSubmitted, ...newProps }: NewSplitProps) {
  const [open, setOpen] = useState(false);
  const more = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const moreId = useId();
  const btn = variant === "primary" ? "primary" : "default";
  const made = (kind: "Requirement" | "Map") => (r: { spec: SpecAcknowledgement }) => onSubmitted({ kind, ...r });

  const items = () => Array.from(menu.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []);
  const focusFirst = () => items()[0]?.focus();

  // A click anywhere but the ▾ (which toggles) and the menu closes it,
  // + New included; Esc closes it and returns focus to the ▾.
  useEffect(() => {
    if (!open) return;
    const inside = (t: Node) => more.current?.contains(t) || menu.current?.contains(t);
    const away = (e: MouseEvent) => !inside(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      setOpen(false);
      more.current?.focus();
    };
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);

  // Opening the menu focuses its first item.
  useEffect(() => {
    if (open) focusFirst();
  }, [open]);

  // ↓ on the ▾ opens the menu as Enter and Space do, by their click.
  const onMoreKey = (e: ReactKeyboardEvent) => {
    if (e.key !== "ArrowDown") return;
    e.preventDefault();
    if (open) focusFirst();
    else setOpen(true);
  };

  // ↑ and ↓ move between the items, wrapping; Tab out of the menu closes it.
  // Keys from a modal an item opened bubble here through its portal, and
  // are left alone.
  const onMenuKey = (e: ReactKeyboardEvent) => {
    if (!menu.current?.contains(e.target as Node)) return;
    if (e.key === "Tab") {
      setOpen(false);
      return;
    }
    const step = { ArrowDown: 1, ArrowUp: -1 }[e.key];
    if (!step) return;
    e.preventDefault();
    const all = items();
    const at = all.indexOf(document.activeElement as HTMLElement);
    all[(at + step + all.length) % all.length]?.focus();
  };

  // pick closes the menu and opens the item's modal.
  const pick = (show: () => void) => () => {
    setOpen(false);
    show();
  };

  return (
    <>
      <div className="new-split">
        <NewRequirement
          {...newProps}
          onSubmitted={made("Requirement")}
          trigger={(show) => (
            <Button variant={btn} className="new-main" aria-label="New requirement" onClick={show}>
              + New
            </Button>
          )}
        />
        <Button
          id={moreId}
          ref={more}
          variant={btn}
          className="new-more"
          aria-label="More to create"
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          onKeyDown={onMoreKey}
        >
          ▾
        </Button>
      </div>
      <div role="menu" aria-labelledby={moreId} className="new-menu" ref={menu} hidden={!open} onKeyDown={onMenuKey}>
        <NewRequirement
          {...newProps}
          onSubmitted={made("Requirement")}
          trigger={(show) => <MenuItem icon="+" label="Requirement" hint="Grilled into a Spec and Tickets" onClick={pick(show)} />}
        />
        <NewMap
          {...newProps}
          onSubmitted={made("Map")}
          trigger={(show) => <MenuItem icon="◇" label="Map" hint="Charts the decisions to get somewhere" onClick={pick(show)} />}
        />
        <NewSmallChange
          {...newProps}
          onSubmitted={(ticket) => onSubmitted({ kind: "Small Change", ticket })}
          trigger={(show) => <MenuItem icon="✓" label="Small Change" hint="One Ticket, built and landed as it is" onClick={pick(show)} />}
        />
      </div>
    </>
  );
}

// MenuItem is named by its label and described by its hint.
function MenuItem({ icon, label, hint, onClick }: { icon: string; label: string; hint: string; onClick(): void }) {
  const id = useId();
  return (
    <button role="menuitem" tabIndex={-1} className="new-menu-item" aria-labelledby={`${id}-label`} aria-describedby={`${id}-hint`} onClick={onClick}>
      <span className="new-menu-icon" aria-hidden="true">
        {icon}
      </span>
      <span>
        <span id={`${id}-label`}>{label}</span>
        <span id={`${id}-hint`} className="new-menu-hint">
          {hint}
        </span>
      </span>
    </button>
  );
}
