// Columns ▾: the Board view's menu that shows or hides each State's column,
// one checkbox per State in workflow order. Its button reads "Columns",
// with how many of the State columns show appended while any is hidden. A
// Needs Me checkbox follows In Progress, for the Needs Me column.
import { useId } from "react";
import { STATES, type State } from "../api/types";
import { Button } from "../controls/Button";
import { Caret } from "../controls/Icon";
import { stateSlug } from "../controls/StateChip";
import { usePopover } from "../controls/usePopover";

export interface ColumnsMenuProps {
  hidden: readonly State[];
  onChange(hidden: State[]): void;
  // needsMe is whether the Needs Me column shows.
  needsMe: boolean;
  onNeedsMe(on: boolean): void;
}

export function ColumnsMenu({ hidden, onChange, needsMe, onNeedsMe }: ColumnsMenuProps) {
  const { open, setOpen, button, wrapProps } = usePopover();
  const id = useId();

  const toggle = (state: State) =>
    onChange(STATES.filter((s) => (s === state ? !hidden.includes(s) : hidden.includes(s))));

  return (
    <div className="popover-wrap" {...wrapProps}>
      <Button ref={button} aria-expanded={open} aria-controls={id} onClick={() => setOpen((o) => !o)}>
        Columns
        {hidden.length > 0 && <span className="muted"> · {STATES.length - hidden.length}/{STATES.length}</span>} <Caret />
      </Button>
      <div id={id} role="group" aria-label="Columns" className="popover-menu" hidden={!open}>
        {STATES.flatMap((s) => [
          <label key={s}>
            <input type="checkbox" checked={!hidden.includes(s)} onChange={() => toggle(s)} />
            <span className={`board-swatch st-${stateSlug(s)}`} aria-hidden="true" />
            {s}
          </label>,
          ...(s === "In Progress"
            ? [
                <label key="needs-me">
                  <input type="checkbox" checked={needsMe} onChange={() => onNeedsMe(!needsMe)} />
                  <span className="dot" aria-hidden="true" />
                  Needs Me
                </label>,
              ]
            : []),
        ])}
      </div>
    </div>
  );
}
