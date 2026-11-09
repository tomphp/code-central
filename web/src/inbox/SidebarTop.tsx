// The top row of the inbox sidebar: the split "+ New" button and "◎ Focus".
// "+ New" opens New requirement, the main action; its "▾" opens a menu of
// Requirement and Map, so every kind is discoverable. The menu stays
// mounted while closed, so a modal opened from one of its items outlives it.
import { useEffect, useId, useRef, useState } from "react";
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

  // A click anywhere but the ▾ (which toggles) and the menu closes it,
  // + New and Focus included.
  useEffect(() => {
    if (!open) return;
    const inside = (t: Node) => more.current?.contains(t) || menu.current?.contains(t);
    const away = (e: MouseEvent) => !inside(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);

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
        >
          ▾
        </button>
      </div>
      <button className="btn" onClick={onFocusMode}>
        <span aria-hidden="true">◎</span> Focus
      </button>
      <div role="menu" aria-labelledby={moreId} className="new-menu" ref={menu} hidden={!open}>
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
    <button role="menuitem" className="new-menu-item" aria-labelledby={`${id}-label`} aria-describedby={`${id}-hint`} onClick={onClick}>
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
