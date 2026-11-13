// PopupShell is what every Board popup shares: the wide modal shell, named
// by the popup's kind, with a header row holding × to close it. The ID and
// title the popup shows come from the pane inside.
import type { ReactNode } from "react";
import { Modal } from "../controls/Modal";

export interface PopupShellProps {
  label: string;
  onClose(): void;
  children: ReactNode;
}

export function PopupShell({ label, onClose, children }: PopupShellProps) {
  return (
    <Modal label={label} size="wide" onClose={onClose}>
      <div className="popup-head">
        <button type="button" className="popup-close" aria-label="Close" onClick={onClose}>
          ×
        </button>
      </div>
      {children}
    </Modal>
  );
}
