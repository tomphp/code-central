// What the UI shows lives in the URL:
// ?view=inbox&project=<key>&state=<State>&list=requirements&requirement=<spec id>&ticket=<id>.
// view is the layout (only the inbox in v1; a focus queue or board later);
// project, state and list are the left pane's filters (no state and no
// list = Needs Me; list=requirements is the Requirements view); requirement
// is the selected Requirement's Spec ID and ticket the selected Ticket's
// display ID (within the Requirement, when one is selected).
import { useCallback, useEffect, useState } from "react";
import { STATES, type State } from "./api/types";

export type View = "inbox";
export type List = "requirements";

export interface UiLocation {
  view: View;
  project?: string;
  state?: State;
  list?: List;
  requirement?: string;
  ticket?: string;
}

export function readLocation(search: string): UiLocation {
  const q = new URLSearchParams(search);
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
  const q = new URLSearchParams({ view: loc.view });
  if (loc.project) q.set("project", loc.project);
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
    const next = { ...readLocation(window.location.search), ...patch };
    window.history.pushState(null, "", hrefFor(next));
    setLoc(next);
  }, []);
  return [loc, navigate];
}
