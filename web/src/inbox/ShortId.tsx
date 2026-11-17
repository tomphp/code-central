// The one place the UI draws a Ticket or Spec ID (the Short ID, in the
// glossary): `#452` while one Project is selected, the full ID otherwise.
// Clicking copies the full ID and never follows the row's link.
import { createContext, useContext, useEffect, useRef, useState, type MouseEvent } from "react";

// OneProject says whether one Project is selected, or there is a Sole Project. The Inbox, the Board, Focus
// Mode and the Activity Dashboard provide it, so a pane or row needs no extra prop; elsewhere IDs show in full.
export const OneProject = createContext(false);

// shortOf is an ID's Short ID within its Project: `#452` for acme-452.
export function shortOf(id: string): string {
  return `#${id.slice(id.lastIndexOf("-") + 1)}`;
}

export function ShortId({ id }: { id: string }) {
  const one = useContext(OneProject);
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);

  const copy = (e: MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    // A missing clipboard, or a write that throws or rejects, does nothing more.
    let write: Promise<void> | undefined;
    try {
      write = navigator.clipboard?.writeText(id);
    } catch {
      return;
    }
    write?.then(() => {
      setCopied(true);
      clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1000);
    }, () => {});
  };

  return (
    <span className="tid short-id" role="button" tabIndex={0} title={id} aria-label={id} onClick={copy}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") copy(e as unknown as MouseEvent); }}>
      {copied ? "Copied" : one ? shortOf(id) : id}
    </span>
  );
}
