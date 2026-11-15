// What the UI shows lives in the URL:
// ?view=inbox&project=<key>&state=<State>&list=requirements&requirement=<spec id>&ticket=<id>,
// or ?view=focus&project=<key> for Focus Mode, which keeps only project,
// or ?view=options&from=<inbox|focus>&... for the Options view, which keeps
// the parameters of the view it came from, to return to it,
// or ?view=project-options&project=<key> for a Project's Project Options page,
// or ?view=usage&project=<key>&days=<30|90>&tab=<context|calls>&session_project=<key>&session=<id>&agent=<id>&back=<query>
// for Usage, where project is Usage's scope (none = All Projects), days its
// range (none = 7), tab its open tab (none = Token usage, so tokens is left
// out and an unknown tab reads as none), session_project, session and agent the
// session open in its Session popup (agent none for a main session), and back
// the query of the view it returns to (none = the Inbox),
// or ?view=ci&project=<key>&back=<query> for the CI view, where project is
// its scope (none = All Projects) and back as Usage's,
// or ?view=activity&project=<key>&back=<query> for the Activity Dashboard
// in a view of its own, the same,
// or ?view=board&project=<key>&group=spec&hide=<States>&needs=<1|0>&popup=<id>&back=<query>
// for the Board view, where hide is the States whose columns it hides
// (comma-separated; none = the browser's pick, an empty hide= = none
// hidden), needs=1 or needs=0 shows or hides its Needs Me column (none =
// the browser's pick, off if none), popup the ID of the Spec or Ticket whose popup is open, group
// and back as the Inbox's and Usage's.
// group=spec is the Inbox's By Spec grouping of a State list (none = By
// Ticket); it stays in the URL across lists and Projects.
// The Inbox also takes open=<id>, a Spec or Ticket an ID Link asked it to
// select, whose kind isn't known yet; the App replaces it with requirement=
// or ticket= once it is, and no other view keeps it.
// view is the layout (anything else is the inbox); project, state and
// list are the left pane's filters (no state and no list = Needs Me;
// list=requirements is the Requirements view, list=tickets the All Tickets
// view); requirement is the selected
// Spec's ID (a Requirement, a Map or a plain Spec) and ticket the selected
// Ticket's display ID (within the Spec, when one is selected).
import { useCallback, useEffect, useState, type MouseEvent } from "react";
import { STATES, USAGE_DAYS, type State, type UsageDays, type SessionUsageFilter } from "./api/types";

export type View = "inbox" | "focus" | "options" | "project-options" | "usage" | "ci" | "activity" | "board";
// A ReturnView is a view the Options view, Usage, the CI view, the Activity view and the Board return to.
export type ReturnView = Exclude<View, "options" | "project-options" | "usage" | "ci" | "activity" | "board">;
export type List = "requirements" | "tickets";
// USAGE_TABS are Usage's tabs, in order; the first is the default.
export const USAGE_TABS = ["tokens", "context", "calls"] as const;
export type UsageTab = (typeof USAGE_TABS)[number];

export interface UiLocation {
  view: View;
  // from is the view the Options view returns to; returnViewOf reads it.
  from?: ReturnView;
  project?: string;
  state?: State;
  list?: List;
  requirement?: string;
  ticket?: string;
  // open is the ID of a Spec or Ticket the Inbox is to select, once its
  // kind is known.
  open?: string;
  // group is the Inbox's grouping of a State list, and the Board's; none is By Ticket.
  group?: "spec";
  // hide is the States whose columns the Board hides, in workflow order;
  // none is the browser's pick, and empty is every column shown.
  hide?: State[];
  // needs is whether the Board shows its Needs Me column; none is the
  // browser's pick.
  needs?: boolean;
  // popup is the ID of the Spec or Ticket whose popup the Board has open.
  popup?: string;
  // finished shows Done and Cancelled Tickets in the All Tickets view.
  finished?: true;
  // days is Usage's range; none is the default, 7.
  days?: UsageDays;
  // tab is Usage's open tab; none is the default, Token usage.
  tab?: UsageTab;
  // session is the session Usage has open in its Session popup.
  session?: SessionUsageFilter;
  // back is where Usage, the CI view, the Activity view and the Board return to, a ReturnView
  // with its parameters.
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
  if (view === "ci" || view === "activity") return { view, project: q.get("project") || undefined, back: readBack(q) };
  if (view === "board") {
    const hide = q.get("hide")?.split(",");
    return {
      view,
      project: q.get("project") || undefined,
      group: q.get("group") === "spec" ? "spec" : undefined,
      hide: hide && STATES.filter((s) => hide.includes(s)),
      needs: q.get("needs") === "1" ? true : q.get("needs") === "0" ? false : undefined,
      popup: q.get("popup") || undefined,
      back: readBack(q),
    };
  }
  if (view === "usage") {
    const days = Number(q.get("days")) as UsageDays;
    const tab = q.get("tab") as UsageTab;
    const [sessionProject, session, agent] = [q.get("session_project"), q.get("session"), q.get("agent")];
    return {
      view,
      project: q.get("project") || undefined,
      days: USAGE_DAYS.includes(days) && days !== USAGE_DAYS[0] ? days : undefined,
      tab: USAGE_TABS.includes(tab) && tab !== USAGE_TABS[0] ? tab : undefined,
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
    open: q.get("open") || undefined,
    group: q.get("group") === "spec" ? "spec" : undefined,
    finished: q.get("finished") === "1" ? true : undefined,
  };
}

// readBack is the view an address's back returns to: the Inbox unless it
// names the Inbox, Focus Mode or the Activity view (the CI view's Back from
// the dashboard's CI Running block).
function readBack(q: URLSearchParams): UiLocation {
  const back = readLocation(`?${q.get("back") ?? ""}`);
  return back.view === "inbox" || back.view === "focus" || back.view === "activity" ? back : readLocation(INBOX);
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
    if (loc.tab && loc.tab !== USAGE_TABS[0]) q.set("tab", loc.tab);
    if (loc.session) {
      q.set("session_project", loc.session.project);
      q.set("session", loc.session.session);
      if (loc.session.agent) q.set("agent", loc.session.agent);
    }
    return withBack(q, loc);
  }
  if (loc.view === "ci" || loc.view === "activity") return withBack(q, loc);
  if (loc.view === "board") {
    if (loc.group) q.set("group", loc.group);
    if (loc.hide) q.set("hide", loc.hide.join(","));
    if (loc.needs !== undefined) q.set("needs", loc.needs ? "1" : "0");
    if (loc.popup) q.set("popup", loc.popup);
    return withBack(q, loc);
  }
  if (loc.view !== "inbox") return `?${q}`;
  if (loc.state) q.set("state", loc.state);
  if (loc.list) q.set("list", loc.list);
  if (loc.requirement) q.set("requirement", loc.requirement);
  if (loc.ticket) q.set("ticket", loc.ticket);
  if (loc.open) q.set("open", loc.open);
  if (loc.group) q.set("group", loc.group);
  if (loc.finished) q.set("finished", "1");
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

// ID_LINK is a Spec's or Ticket's full ID: a Project Key, "-" and a positive number.
const ID_LINK = /^[a-z0-9][a-z0-9._-]*-[1-9][0-9]*$/;

// isIdLink reports whether a Markdown link target is exactly a full ID.
export function isIdLink(target: string): boolean {
  return ID_LINK.test(target);
}

// idLinkPatch is the change to loc that opens the Spec or Ticket id in
// place: on the Board its popup, replacing any open; in any other view the
// Inbox with it to select, in its Project when loc is scoped to one.
export function idLinkPatch(loc: UiLocation, id: string): Partial<UiLocation> {
  if (loc.view === "board") return { popup: id };
  return {
    view: "inbox",
    project: loc.project && id.slice(0, id.lastIndexOf("-")),
    open: id,
    requirement: undefined,
    ticket: undefined,
  };
}

// useUiLocation reads the URL and returns a navigate that pushes a change
// onto the history, so Back returns to the previous filter or Ticket, and a
// replace that changes the current entry instead.
export function useUiLocation(): [UiLocation, (patch: Partial<UiLocation>) => void, (patch: Partial<UiLocation>) => void] {
  const [loc, setLoc] = useState(() => readLocation(window.location.search));
  useEffect(() => {
    const onPop = () => setLoc(readLocation(window.location.search));
    window.addEventListener("popstate", onPop);
    // Normalise a bare or partial URL to the canonical one.
    window.history.replaceState(null, "", hrefFor(readLocation(window.location.search)));
    return () => window.removeEventListener("popstate", onPop);
  }, []);
  const go = useCallback((how: "pushState" | "replaceState", patch: Partial<UiLocation>) => {
    const href = hrefFor({ ...readLocation(window.location.search), ...patch });
    window.history[how](null, "", href);
    setLoc(readLocation(href));
  }, []);
  const navigate = useCallback((patch: Partial<UiLocation>) => go("pushState", patch), [go]);
  const replace = useCallback((patch: Partial<UiLocation>) => go("replaceState", patch), [go]);
  return [loc, navigate, replace];
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
