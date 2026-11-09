// What the UI shows lives in the URL:
// ?view=inbox&project=<key>&state=<State>&list=requirements&requirement=<spec id>&ticket=<id>,
// or ?view=focus&project=<key> for Focus Mode, which keeps only project,
// or ?view=options&from=<inbox|focus>&... for the Options view, which keeps
// the parameters of the view it came from, to return to it.
// view is the layout (anything else is the inbox); project, state and
// list are the left pane's filters (no state and no list = Needs Me;
// list=requirements is the Requirements view); requirement is the selected
// Requirement's Spec ID and ticket the selected Ticket's display ID (within
// the Requirement, when one is selected).
import { useCallback, useEffect, useState, type MouseEvent } from "react";
import { STATES, type State } from "./api/types";

export type View = "inbox" | "focus" | "options";
export type List = "requirements";

export interface UiLocation {
  view: View;
  // from is the view the Options view returns to: the Inbox unless Focus Mode.
  from?: "inbox" | "focus";
  project?: string;
  state?: State;
  list?: List;
  requirement?: string;
  ticket?: string;
}

export function readLocation(search: string): UiLocation {
  const q = new URLSearchParams(search);
  if (q.get("view") === "options") {
    const from = q.get("from") === "focus" ? "focus" : "inbox";
    q.set("view", from);
    return { ...readLocation(`?${q}`), view: "options", from };
  }
  if (q.get("view") === "focus") return { view: "focus", project: q.get("project") || undefined };
  const state = q.get("state") as State | null;
  return {
    view: "inbox",
    project: q.get("project") || undefined,
    state: state && STATES.includes(state) ? state : undefined,
    list: q.get("list") === "requirements" ? "requirements" : undefined,
    requirement: q.get("requirement") || undefined,
    ticket: q.get("ticket") || undefined,
  };
}

export function hrefFor(loc: UiLocation): string {
  if (loc.view === "options") {
    const from = loc.from ?? "inbox";
    const back = new URLSearchParams(hrefFor({ ...loc, view: from }));
    back.delete("view");
    return `?${new URLSearchParams({ view: "options", from })}${back.size ? `&${back}` : ""}`;
  }
  const q = new URLSearchParams({ view: loc.view });
  if (loc.project) q.set("project", loc.project);
  if (loc.view === "focus") return `?${q}`;
  if (loc.state) q.set("state", loc.state);
  if (loc.list) q.set("list", loc.list);
  if (loc.requirement) q.set("requirement", loc.requirement);
  if (loc.ticket) q.set("ticket", loc.ticket);
  return `?${q}`;
}

// useUiLocation reads the URL and returns a navigate that pushes a change
// onto the history, so Back returns to the previous filter or Ticket.
export function useUiLocation(): [UiLocation, (patch: Partial<UiLocation>) => void] {
  const [loc, setLoc] = useState(() => readLocation(window.location.search));
  useEffect(() => {
    const onPop = () => setLoc(readLocation(window.location.search));
    window.addEventListener("popstate", onPop);
    // Normalise a bare or partial URL to the canonical one.
    window.history.replaceState(null, "", hrefFor(readLocation(window.location.search)));
    return () => window.removeEventListener("popstate", onPop);
  }, []);
  const navigate = useCallback((patch: Partial<UiLocation>) => {
    const href = hrefFor({ ...readLocation(window.location.search), ...patch });
    window.history.pushState(null, "", href);
    setLoc(readLocation(href));
  }, []);
  return [loc, navigate];
}

// opensInPlace reports whether a click on a link is a plain one, for the
// UI to navigate itself; a modified or middle click is left to the
// browser, to open a tab.
export function opensInPlace(e: MouseEvent): boolean {
  return !(e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0);
}
