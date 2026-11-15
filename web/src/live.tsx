// Live data: the UI reads through <Live>, which opens its only change feed,
// and the named read hooks below, which take the client and the live
// version from it.
//
// The invalidation rule: the feed only names what changed, so every change
// notice bumps the version, and so does a reconnect, since notices may have
// been missed while the feed was down. Every live read refetches on it.
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import type { Api } from "./api/api";
import {
  STATES,
  type ApiError,
  type CIOverview,
  type CIOverviewFilter,
  type NeedsMeView,
  type Options,
  type ProjectRow,
  type SessionUsageFilter,
  type SessionUsageView,
  type SpecDetail,
  type SpecRow,
  type TicketRow,
  type TicketView,
  type UsageFilter,
  type UsageView,
} from "./api/types";

interface LiveValue {
  api: Api;
  version: number;
}

// LiveContext has no default: a read outside <Live> is a mistake.
const LiveContext = createContext<LiveValue | null>(null);

// Live opens the feed over api and gives the tree below it api and the
// live version.
export function Live({ api, children }: { api: Api; children: ReactNode }) {
  const version = useLiveVersion(api);
  const value = useMemo(() => ({ api, version }), [api, version]);
  return <LiveContext.Provider value={value}>{children}</LiveContext.Provider>;
}

// WithApi gives the tree below it api in place of <Live>'s client, with the
// live version unchanged, for a host that watches the writes its children
// make (Focus Mode, which notes what the human acted on).
export function WithApi({ api, children }: { api: Api; children: ReactNode }) {
  const { version } = useLive("WithApi");
  const value = useMemo(() => ({ api, version }), [api, version]);
  return <LiveContext.Provider value={value}>{children}</LiveContext.Provider>;
}

// useLive is what <Live> gives, or, outside it, a clear error naming hook.
function useLive(hook: string): LiveValue {
  const live = useContext(LiveContext);
  if (!live) throw new Error(`${hook} is used outside <Live>: render it inside <Live api={api}>`);
  return live;
}

// useApi is the client <Live> was given.
export function useApi(): Api {
  return useLive("useApi").api;
}

// useVersion is the live version, for a hook that reads through a loader
// of its own, and for a view handing it to children that still take it
// as a prop.
export function useVersion(): number {
  return useLive("useVersion").version;
}

// useLiveVersion opens the feed; only <Live> calls it.
function useLiveVersion(api: Api): number {
  const [version, setVersion] = useState(0);
  useEffect(
    () =>
      api.subscribe({
        onEvent: () => setVersion((v) => v + 1),
        onOpen: ({ reconnect }) => reconnect && setVersion((v) => v + 1),
      }),
    [api],
  );
  return version;
}

// useFetch runs load whenever its deps change and keeps the latest result;
// a response that arrives after a newer request started is dropped. It is
// private: a view reads through a named hook below.
function useFetch<T>(load: (() => Promise<T>) | null, deps: unknown[]): { data?: T; error?: ApiError } {
  const [state, setState] = useState<{ data?: T; error?: ApiError }>({});
  useEffect(() => {
    if (!load) {
      setState({});
      return;
    }
    let current = true;
    load().then(
      (data) => current && setState({ data }),
      (error) => current && setState((s) => ({ data: s.data, error })),
    );
    return () => {
      current = false;
    };
  }, deps);
  return state;
}

export interface ProjectTicket extends TicketRow {
  project: string;
}

export interface ProjectSpec extends SpecRow {
  project: string;
}

export interface InboxData {
  projects: ProjectRow[];
  needsMe: NeedsMeView;
  // tickets is every Project's Tickets, finished ones too, for the State
  // views, the counts and Drafts.
  tickets: ProjectTicket[];
  // specs is every Project's Specs, finished ones too.
  specs: ProjectSpec[];
  // requirements is the Specs that are Requirements, delivered ones too.
  requirements: ProjectSpec[];
  // maps is the Specs that are Maps, finished ones too.
  maps: ProjectSpec[];
}

// loadInbox reads every Project's Inbox data in one go.
export async function loadInbox(api: Api): Promise<InboxData> {
  const [projects, needsMe] = await Promise.all([api.listProjects(), api.needsMe()]);
  const [tickets, specs] = await Promise.all([
    Promise.all(projects.map(async (p) => (await api.findTickets(p.key, { state: [...STATES] })).map((t) => ({ ...t, project: p.key })))),
    Promise.all(projects.map(async (p) => (await api.findSpecs(p.key, { finished: true })).map((s) => ({ ...s, project: p.key })))),
  ]);
  const all = specs.flat();
  return {
    projects,
    needsMe,
    tickets: tickets.flat(),
    specs: all,
    requirements: all.filter((s) => s.requirement),
    maps: all.filter((s) => s.map),
  };
}

// ReadResult is a read hook's result: the latest data, and the error of the
// last read if it failed.
type ReadResult<T> = { data?: T; error?: ApiError };

// useInbox reads every Project's Inbox data, live.
export function useInbox(): ReadResult<InboxData> {
  const { api, version } = useLive("useInbox");
  return useFetch(() => loadInbox(api), [api, version]);
}

// useOptions reads the Options apart from the Inbox data, so a failed
// read of one never holds back the other. options.changed bumps the
// version like every notice, so an open view follows a change made
// elsewhere.
export function useOptions(): ReadResult<Options> {
  const { api, version } = useLive("useOptions");
  return useFetch(() => api.options(), [api, version]);
}

// useSpec reads a Spec, live; it reads nothing until given both.
export function useSpec(project: string | undefined, id: string | undefined): ReadResult<SpecDetail> {
  const { api, version } = useLive("useSpec");
  return useFetch(project && id ? () => api.getSpec(project, id) : null, [api, project, id, version]);
}

// useTicket reads a Ticket, live; it reads nothing until given both.
export function useTicket(project: string | undefined, id: string | undefined): ReadResult<TicketView> {
  const { api, version } = useLive("useTicket");
  return useFetch(project && id ? () => api.getTicket(project, id) : null, [api, project, id, version]);
}

// useCIOverview reads the CI overview for filter, live. With no filter, as
// before the caller's scope resolves, it reads nothing; a filter with no
// project reads All Projects.
export function useCIOverview(filter: CIOverviewFilter | undefined): ReadResult<CIOverview> {
  const { api, version } = useLive("useCIOverview");
  const { project, running } = filter ?? {};
  return useFetch(filter ? () => api.ciOverview({ project, running }) : null, [api, !!filter, project, running, version]);
}

// useUsage reads token usage for filter. With no filter, as before the
// caller's scope resolves, it reads nothing; a filter with no project
// reads All Projects. Usage loads when it is opened and when its scope or
// range changes, and is never live: it leaves out the version, so a busy
// Factory's notices don't refetch it.
export function useUsage(filter: UsageFilter | undefined): ReadResult<UsageView> {
  const { api } = useLive("useUsage");
  const { project, days } = filter ?? {};
  return useFetch(filter ? () => api.usage({ project, days }) : null, [api, !!filter, project, days]);
}

// useSessionUsage reads one session's responses; with no session it reads
// nothing. It loads when it is opened and when its session changes, and
// is never live: it leaves out the version, so a busy Factory's notices
// don't refetch it.
export function useSessionUsage(session: SessionUsageFilter | undefined): ReadResult<SessionUsageView> {
  const { api } = useLive("useSessionUsage");
  const { project, session: id, agent } = session ?? {};
  return useFetch(project && id ? () => api.sessionUsage({ project, session: id, agent }) : null, [api, project, id, agent]);
}
