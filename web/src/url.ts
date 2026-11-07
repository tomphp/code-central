// What the UI shows lives in the URL: ?view=inbox&project=<key>&state=<State>&ticket=<id>.
// view is the layout (only the inbox in v1; a focus queue or board later);
// project and state are the left pane's filters (no state = Needs Me);
// ticket is the selected Ticket's display ID.
import { useCallback, useEffect, useState } from "react";
import { STATES, type State } from "./api/types";

export type View = "inbox";

export interface UiLocation {
  view: View;
  project?: string;
  state?: State;
  ticket?: string;
}

export function readLocation(search: string): UiLocation {
  const q = new URLSearchParams(search);
  const state = q.get("state") as State | null;
  return {
    view: "inbox",
    project: q.get("project") || undefined,
    state: state && STATES.includes(state) ? state : undefined,
    ticket: q.get("ticket") || undefined,
  };
}

export function hrefFor(loc: UiLocation): string {
  const q = new URLSearchParams({ view: loc.view });
  if (loc.project) q.set("project", loc.project);
  if (loc.state) q.set("state", loc.state);
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
