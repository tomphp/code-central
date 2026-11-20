// The Project picker of a full-width view: a menu like Columns ▾ that lists
// All Projects, the current Projects, then the Archived ones under a heading,
// each with its Factory Status mark and Project ribbon, and the chosen Project
// when it isn't known. It is a single-select listbox.
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import type { ProjectRow } from "../api/types";
import { FactoryStatusMark } from "../factory/FactoryStatus";
import { Button } from "./Button";
import { Caret } from "./Icon";
import { ProjectRibbon } from "./ProjectRibbon";
import { usePopover } from "./usePopover";

export interface ProjectPickerProps {
  projects: ProjectRow[];
  // value is the chosen Project's key, undefined for All Projects.
  value?: string;
  onChange(key: string | undefined): void;
  // withoutAll drops the All Projects choice, while there is a Sole Project.
  withoutAll?: boolean;
}

// PickerEntry is one option: a Project (none for All Projects) or a bare key.
interface PickerEntry {
  key: string | undefined;
  name: string;
  project?: ProjectRow;
}

const byName = (a: ProjectRow, b: ProjectRow) =>
  a.display_name.localeCompare(b.display_name, undefined, { sensitivity: "base" }) || a.key.localeCompare(b.key);

// pickerEntries orders the picker's options: All Projects, the current
// Projects, the Archived ones, then the chosen key when no Project has it.
function pickerEntries(projects: ProjectRow[], value: string | undefined, withoutAll: boolean | undefined) {
  const entry = (p: ProjectRow): PickerEntry => ({ key: p.key, name: p.display_name, project: p });
  return {
    all: withoutAll ? [] : [{ key: undefined, name: "All Projects" }],
    current: projects.filter((p) => !p.archived).sort(byName).map(entry),
    archived: projects.filter((p) => p.archived).sort(byName).map(entry),
    unknown: value && !projects.some((p) => p.key === value) ? [{ key: value, name: value }] : [],
  };
}

export function ProjectPicker({ projects, value, onChange, withoutAll }: ProjectPickerProps) {
  const { open, setOpen, button, wrapProps } = usePopover();
  const labelId = useId();
  const buttonId = useId();
  const listId = useId();
  const list = useRef<HTMLDivElement>(null);
  const groups = pickerEntries(projects, value, withoutAll);
  const entries = [...groups.all, ...groups.current, ...groups.archived, ...groups.unknown];
  const chosen = entries.findIndex((e) => e.key === value);
  const [active, setActive] = useState(0);
  const optionId = (i: number) => `${listId}-${i}`;

  // Opening moves focus into the list, on the chosen entry.
  useEffect(() => {
    if (!open) return;
    setActive(Math.max(chosen, 0));
    list.current?.focus();
    // chosen is read when the menu opens, not as it changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- opening is the only trigger
  }, [open]);

  const choose = (e: PickerEntry) => {
    setOpen(false);
    button.current?.focus();
    if (e.key !== value) onChange(e.key);
  };
  const onKeyDown = (e: KeyboardEvent) => {
    const move = (i: number) => {
      e.preventDefault();
      setActive(Math.min(Math.max(i, 0), entries.length - 1));
    };
    if (e.key === "ArrowDown") move(active + 1);
    else if (e.key === "ArrowUp") move(active - 1);
    else if (e.key === "Home") move(0);
    else if (e.key === "End") move(entries.length - 1);
    else if (e.key === "Enter" && entries[active]) {
      e.preventDefault();
      choose(entries[active]);
    } else if (e.key === "Tab") setOpen(false);
  };

  const mark = (e: PickerEntry) => (
    <>
      {e.project && !e.project.archived && <FactoryStatusMark status={e.project.factory_status} />}
      {e.project && <ProjectRibbon projectKey={e.project.key} />}
      {e.name}
    </>
  );
  const option = (e: PickerEntry) => {
    const i = entries.indexOf(e);
    return (
      <div
        key={e.key ?? ""}
        id={optionId(i)}
        role="option"
        aria-selected={i === chosen}
        className={i === active ? "active" : undefined}
        onClick={() => choose(e)}
      >
        {mark(e)}
        <span className="check" aria-hidden="true">
          {i === chosen ? "✓" : ""}
        </span>
      </div>
    );
  };

  return (
    <div className="view-picker">
      <span className="view-picker-label" id={labelId}>
        Projects
      </span>
      <div className="popover-wrap" {...wrapProps}>
        <Button
          ref={button}
          id={buttonId}
          aria-haspopup="listbox"
          aria-expanded={open}
          aria-controls={open ? listId : undefined}
          aria-labelledby={`${labelId} ${buttonId}`}
          onClick={() => setOpen((o) => !o)}
        >
          <span className="project-choice">{chosen >= 0 ? mark(entries[chosen]) : "All Projects"}</span> <Caret />
        </Button>
        {open && (
          <div
            id={listId}
            ref={list}
            role="listbox"
            tabIndex={-1}
            aria-label="Projects"
            aria-activedescendant={optionId(active)}
            className="popover-menu popover-left"
            onKeyDown={onKeyDown}
          >
            {[...groups.all, ...groups.current].map(option)}
            {groups.archived.length > 0 && (
              <div role="group" aria-labelledby={`${listId}-archived`}>
                <div className="popover-heading" id={`${listId}-archived`}>
                  Archived
                </div>
                {groups.archived.map(option)}
              </div>
            )}
            {groups.unknown.map(option)}
          </div>
        )}
      </div>
    </div>
  );
}
