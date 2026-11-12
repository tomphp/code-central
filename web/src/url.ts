// What the UI shows lives in the URL:
// ?view=inbox&project=<key>&state=<State>&list=requirements&requirement=<spec id>&ticket=<id>,
// or ?view=focus&project=<key> for Focus Mode, which keeps only project,
// or ?view=options&from=<inbox|focus>&... for the Options view, which keeps
// the parameters of the view it came from, to return to it,
// or ?view=project-options&project=<key> for a Project's Project Options page,
// or ?view=usage&project=<key>&days=<30|90>&session_project=<key>&session=<id>&agent=<id>&back=<query>
// for Usage, where project is Usage's scope (none = All Projects), days its
// range (none = 7), session_project, session and agent the session open
// on its line chart (agent none for a main session), and back the query
// of the view it returns to (none = the Inbox),
// or ?view=ci&project=<key>&back=<query> for the CI view, where project is
// its scope (none = All Projects) and back as Usage's.
// group=spec is the Inbox's By Spec grouping of a State list (none = By
// Ticket); it stays in the URL across lists and Projects.
// view is the layout (anything else is the inbox); project, state and
// list are the left pane's filters (no state and no list = Needs Me;
// list=requirements is the Requirements view, list=tickets the All Tickets
// view); requirement is the selected
// Spec's ID (a Requirement, a Map or a plain Spec) and ticket the selected
// Ticket's display ID (within the Spec, when one is selected).
import { useCallback, useEffect, useState, type MouseEvent } from "react";
import { STATES, USAGE_DAYS, type State, type UsageDays, type SessionUsageFilter } from "./api/types";

export type View = "inbox" | "focus" | "options" | "project-options" | "usage" | "ci";
// A ReturnView is a view the Options view, Usage and the CI view return to.
export type ReturnView = Exclude<View, "options" | "project-options" | "usage" | "ci">;
export type List = "requirements" | "tickets";

export interface UiLocation {
  view: View;
  // from is the view the Options view returns to; returnViewOf reads it.
  from?: ReturnView;
  project?: string;
  state?: State;
  list?: List;
  requirement?: string;
  ticket?: string;
  // group is the Inbox's grouping of a State list; none is By Ticket.
  group?: "spec";
  // days is Usage's range; none is the default, 7.
  days?: UsageDays;
  // session is the session Usage has open on its line chart.
  session?: SessionUsageFilter;
  // back is where Usage and the CI view return to, a ReturnView with its
  // parameters.
  back?: UiLocation;
}

// The view a bare address opens, and where Usage or the CI view opened
// from it returns.
const INBOX = "?view=inbox";

export function readLocation(search: string): UiLocation {
  const q = new URLSearchParams(search);
  if (q.get("view") === "options") {
    const from = returnViewOf({ view: "options", from: q.get("from") as ReturnView });
    q.set("view", from);
    return { ...readLocation(`?${q}`), view: "options", from };
  }
  const view = q.get("view");
  if (view === "ci") return { view, project: q.get("project") || undefined, back: readBack(q) };
  if (view === "usage") {
    const days = Number(q.get("days")) as UsageDays;
    const [sessionProject, session, agent] = [q.get("session_project"), q.get("session"), q.get("agent")];
    return {
      view,
      project: q.get("project") || undefined,
      days: USAGE_DAYS.includes(days) && days !== USAGE_DAYS[0] ? days : undefined,
      session: sessionProject && session ? { project: sessionProject, session, ...(agent ? { agent } : {}) } : undefined,
      back: readBack(q),
    };
  }
  if (view === "focus" || view === "project-options") return { view, project: q.get("project") || undefined };
  const state = q.get("state") as State | null;
  return {
    view: "inbox",
    project: q.get("project") || undefined,
    state: state && STATES.includes(state) ? state : undefined,
    list: q.get("list") === "requirements" || q.get("list") === "tickets" ? (q.get("list") as List) : undefined,
    requirement: q.get("requirement") || undefined,
    ticket: q.get("ticket") || undefined,
    group: q.get("group") === "spec" ? "spec" : undefined,
  };
}

// readBack is the view an address's back returns to: the Inbox unless it
// names the Inbox or Focus Mode.
function readBack(q: URLSearchParams): UiLocation {
  const back = readLocation(`?${q.get("back") ?? ""}`);
  return back.view === "inbox" || back.view === "focus" ? back : readLocation(INBOX);
}

// Link is an in-app link: its href, and a click that navigates in place.
export interface Link {
  href: string;
  onClick(e: MouseEvent): void;
}

export function hrefFor(loc: UiLocation): string {
  if (loc.view === "options") {
    const from = returnViewOf(loc);
    const back = new URLSearchParams(hrefFor({ ...loc, view: from }));
    back.delete("view");
    return `?${new URLSearchParams({ view: "options", from })}${back.size ? `&${back}` : ""}`;
  }
  const q = new URLSearchParams({ view: loc.view });
  if (loc.project) q.set("project", loc.project);
  if (loc.view === "usage") {
    if (loc.days && loc.days !== USAGE_DAYS[0]) q.set("days", String(loc.days));
    if (loc.session) {
      q.set("session_project", loc.session.project);
      q.set("session", loc.session.session);
      if (loc.session.agent) q.set("agent", loc.session.agent);
    }
    return withBack(q, loc);
  }
  if (loc.view === "ci") return withBack(q, loc);
  if (loc.view !== "inbox") return `?${q}`;
  if (loc.state) q.set("state", loc.state);
  if (loc.list) q.set("list", loc.list);
  if (loc.requirement) q.set("requirement", loc.requirement);
  if (loc.ticket) q.set("ticket", loc.ticket);
  if (loc.group) q.set("group", loc.group);
  return `?${q}`;
}

// withBack is q's address with loc's back, left out for the Inbox.
function withBack(q: URLSearchParams, loc: UiLocation): string {
  const back = loc.back ? hrefFor(loc.back) : INBOX;
  if (back !== INBOX) q.set("back", back.slice(1));
  return `?${q}`;
}

// returnViewOf is the view to return to from loc: from the Options view,
// the one it came from, the Inbox unless Focus Mode; from any other, itself.
export function returnViewOf(loc: UiLocation): ReturnView {
  const view = loc.view === "options" ? loc.from : loc.view;
  return view === "focus" ? "focus" : "inbox";
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

// clickTo is a link's click handler: a plain click navigates in place,
// any other is left to the browser.
export function clickTo(navigate: (patch: Partial<UiLocation>) => void, patch: Partial<UiLocation>) {
  return (e: MouseEvent) => {
    if (!opensInPlace(e)) return;
    e.preventDefault();
    navigate(patch);
  };
}
