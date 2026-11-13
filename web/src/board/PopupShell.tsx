// PopupShell is what every Board popup shares: the wide modal shell, named
// by the popup's kind, with a header row holding × to close it. The ID and
// title the popup shows come from the pane inside; head is anything else
// the header row holds, at its start.
import type { ReactNode } from "react";
import { Modal } from "../controls/Modal";

export interface PopupShellProps {
  label: string;
  onClose(): void;
  head?: ReactNode;
  // uncounted is Modal's: the automatic Focus popup's own shell.
  uncounted?: boolean;
  children: ReactNode;
}

export function PopupShell({ label, onClose, head, uncounted, children }: PopupShellProps) {
  return (
    <Modal label={label} size="wide" uncounted={uncounted} onClose={onClose}>
      <div className="popup-head">
        {head}
        <button type="button" className="popup-close" aria-label="Close" onClick={onClose}>
          ×
        </button>
      </div>
      {children}
    </Modal>
  );
}
