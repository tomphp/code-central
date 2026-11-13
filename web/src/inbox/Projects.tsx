// The left pane's Projects: active ones as filters, each with a menu that
// renames (story 92), opens its Project Options page or archives it, and
// Archived ones in a collapsed group at the bottom, each with a menu that
// unarchives it (ticket 39).
import { useEffect, useId, useRef, useState, type FormEvent, type MouseEvent, type ReactNode } from "react";
import type { Api } from "../api/api";
import type { ApiError, ProjectRow } from "../api/types";
import { FactoryStatusDot } from "../factory/FactoryStatus";
import { Button } from "../controls/Button";

export interface ProjectFilterProps {
  api: Api;
  project: ProjectRow;
  now: Date;
  on: boolean;
  onSelect(): void;
  // projectOptions links to the Project's Project Options page.
  projectOptions?: { href: string; onClick(e: MouseEvent): void };
}

// ProjectFilter is one Project's row: the filter button, then its menu.
export function ProjectFilter({ api, project: p, now, on, onSelect, projectOptions }: ProjectFilterProps) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string>();
  // name is the rename box's text while the human is renaming.
  const [name, setName] = useState<string>();
  const ref = useRef<HTMLDivElement>(null);
  const menuId = useId();
  const reasonId = useId();
  const label = `Menu for ${p.display_name}`;

  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("keydown", close);
    document.addEventListener("mousedown", close);
    return () => {
      document.removeEventListener("keydown", close);
      document.removeEventListener("mousedown", close);
    };
  }, [open]);

  // After the action, nothing else: the service's notice refetches the list.
  const run = (action: () => Promise<unknown>) => {
    setOpen(false);
    setError(undefined);
    action().catch((e: ApiError) => setError(e.message));
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api.renameProject(p.key, name ?? "").then(
      () => setName(undefined),
      (e: ApiError) => setError(e.message),
    );
  };
  const blocked = p.unfinished > 0 ? `${p.unfinished} unfinished Ticket${p.unfinished === 1 ? "" : "s"}: finish or cancel them first` : "";

  return (
    <div className="project-row" ref={ref}>
      {!p.archived && <FactoryStatusDot status={p.factory_status} now={now} />}
      <button
        className={["filter", on && "on"].filter(Boolean).join(" ")}
        aria-pressed={on}
        onClick={onSelect}
      >
        {p.display_name} <span className={p.needs_me > 0 ? "count loud" : "count"}>{p.needs_me}</span>
      </button>
      <button
        className="menu-toggle"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={() => setOpen((o) => !o)}
      >
        ⋯
      </button>
      {open && (
        <div className="menu" role="menu" id={menuId} aria-label={label}>
          {p.archived ? (
            <button role="menuitem" onClick={() => run(() => api.unarchiveProject(p.key))}>
              Unarchive
            </button>
          ) : (
            <>
              <button
                role="menuitem"
                onClick={() => {
                  setOpen(false);
                  setError(undefined);
                  setName(p.display_name);
                }}
              >
                Rename
              </button>
              {projectOptions && (
                <a
                  role="menuitem"
                  href={projectOptions.href}
                  onClick={(e) => {
                    setOpen(false);
                    projectOptions.onClick(e);
                  }}
                >
                  Options
                </a>
              )}
              <button
                role="menuitem"
                disabled={!!blocked}
                aria-describedby={blocked ? reasonId : undefined}
                onClick={() => run(() => api.archiveProject(p.key))}
              >
                Archive
              </button>
              {blocked && (
                <p className="menu-note" id={reasonId}>
                  {blocked}
                </p>
              )}
            </>
          )}
        </div>
      )}
      {name !== undefined && (
        <form className="project-rename" onSubmit={save}>
          <input aria-label={`Display name for ${p.display_name}`} value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          <Button type="submit" variant="primary">
            Save
          </Button>
          <Button type="button"  onClick={() => {
              setName(undefined);
              setError(undefined);
            }}>
            Cancel
          </Button>
        </form>
      )}
      {error && (
        <p className="warning project-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

// ArchivedProjects is the collapsed group of Archived Projects.
export function ArchivedProjects({ count, children }: { count: number; children: ReactNode }) {
  if (count === 0) return null;
  return (
    <details className="archived-group">
      <summary>Archived ({count})</summary>
      {children}
    </details>
  );
}
