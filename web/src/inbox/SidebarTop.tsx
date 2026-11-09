// The top row of the inbox sidebar: the split "+ New ▾", primary and full
// width, and "◎ Focus". The row places the ▾'s menu across its width.
import { NewSplit, type NewSplitProps } from "../requirement/NewSplit";

export interface SidebarTopProps {
  // newProps are what + New ▾ needs, all but its variant, which is primary.
  newProps: Omit<NewSplitProps, "variant">;
  onFocusMode(): void;
}

// SidebarTop is the sidebar's top row: + New ▾ and ◎ Focus.
export function SidebarTop({ newProps, onFocusMode }: SidebarTopProps) {
  return (
    <div className="new-row">
      <NewSplit variant="primary" {...newProps} />
      <button className="btn" onClick={onFocusMode}>
        <span aria-hidden="true">◎</span> Focus
      </button>
    </div>
  );
}
