// PopupShell is what every modal popup shares: the Modal, a header row
// holding its title and × to close it, what it holds, and an optional
// actions row at the foot. A popup whose content carries its own heading,
// such as a Board popup's pane, is named by label instead of a title. head
// is anything else the header row holds, after the title. A form popup puts
// its submit button in actions with the form's ID in the button's form.
import { useId, type ReactNode } from "react";
import { Modal } from "./Modal";

export type PopupShellProps = (
  | {
      // title is the heading shown in the header row, naming the popup.
      title: ReactNode;
      label?: never;
    }
  | {
      // label names the popup when what it holds shows the heading.
      label: string;
      title?: never;
    }
) & {
  // size is Modal's: narrow for a form, wide for a Board popup.
  size?: "narrow" | "wide";
  head?: ReactNode;
  actions?: ReactNode;
  // uncounted is Modal's: the automatic Focus popup's own shell.
  uncounted?: boolean;
  onClose(): void;
  children: ReactNode;
};

export function PopupShell({ title, label, size, head, actions, uncounted, onClose, children }: PopupShellProps) {
  const titleId = useId();
  return (
    <Modal labelledBy={title === undefined ? undefined : titleId} label={label} size={size} uncounted={uncounted} onClose={onClose}>
      <div className="popup-head">
        {title !== undefined && <h2 id={titleId}>{title}</h2>}
        {head}
        <button type="button" className="popup-close" aria-label="Close" onClick={onClose}>
          ×
        </button>
      </div>
      {children}
      {actions && <div className="modal-actions">{actions}</div>}
    </Modal>
  );
}
