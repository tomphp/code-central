// The top row of the inbox sidebar: the split "+ New" button and "◎ Focus".
// "+ New" opens New requirement, the main action; its "▾" opens a menu of
// Requirement and Map, so every kind is discoverable. The menu stays
// mounted while closed, so a modal opened from one of its items outlives it.
// The ▾ and its menu follow the usual menu-button keyboard pattern.
import { type KeyboardEvent as ReactKeyboardEvent, useEffect, useId, useRef, useState } from "react";
import { NewMap, NewRequirement, type NewRequirementProps } from "../requirement/NewRequirement";

export interface SidebarTopProps {
  // newProps are what New requirement and New Map need, all but the
  // trigger, which SidebarTop renders.
  newProps: Omit<NewRequirementProps, "trigger">;
  onFocusMode(): void;
}

// SidebarTop is the sidebar's top row: + New, its ▾ menu and ◎ Focus.
export function SidebarTop({ newProps, onFocusMode }: SidebarTopProps) {
  const [open, setOpen] = useState(false);
  const more = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const moreId = useId();

  const items = () => Array.from(menu.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []);
  const focusFirst = () => items()[0]?.focus();

  // A click anywhere but the ▾ (which toggles) and the menu closes it,
  // + New and Focus included; Esc closes it and returns focus to the ▾.
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
    <div className="new-row">
      <div className="new-split">
        <NewRequirement
          {...newProps}
          trigger={(show) => (
            <button className="btn primary new-main" aria-label="New requirement" onClick={show}>
              + New
            </button>
          )}
        />
        <button
          id={moreId}
          ref={more}
          className="btn primary new-more"
          aria-label="More to create"
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          onKeyDown={onMoreKey}
        >
          ▾
        </button>
      </div>
      <button className="btn" onClick={onFocusMode}>
        <span aria-hidden="true">◎</span> Focus
      </button>
      <div role="menu" aria-labelledby={moreId} className="new-menu" ref={menu} hidden={!open} onKeyDown={onMenuKey}>
        <NewRequirement {...newProps} trigger={(show) => <MenuItem icon="+" label="Requirement" hint="Grilled into a Spec and Tickets" onClick={pick(show)} />} />
        <NewMap {...newProps} trigger={(show) => <MenuItem icon="◇" label="Map" hint="Charts the decisions to get somewhere" onClick={pick(show)} />} />
      </div>
    </div>
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
