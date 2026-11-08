// Live data: the feed only names what changed, so every notice bumps a
// version and every read keyed on it refetches. A reconnect bumps it too,
// since notices may have been missed while the feed was down.
import { useEffect, useState } from "react";
import type { Api } from "./api/api";
import {
  STATES,
  type ApiError,
  type NeedsMeView,
  type ProjectRow,
  type SpecDetail,
  type SpecRow,
  type TicketRow,
  type TicketView,
} from "./api/types";

export function useLiveVersion(api: Api): number {
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
// a response that arrives after a newer request started is dropped.
export function useFetch<T>(load: (() => Promise<T>) | null, deps: unknown[]): { data?: T; error?: ApiError } {
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
  // requirements is every Project's Requirements, delivered ones too.
  requirements: ProjectSpec[];
}

export async function loadInbox(api: Api): Promise<InboxData> {
  const [projects, needsMe] = await Promise.all([api.listProjects(), api.needsMe()]);
  const [tickets, requirements] = await Promise.all([
    Promise.all(projects.map(async (p) => (await api.findTickets(p.key, { state: [...STATES] })).map((t) => ({ ...t, project: p.key })))),
    Promise.all(
      projects.map(async (p) => (await api.findSpecs(p.key, { requirements: true, finished: true })).map((s) => ({ ...s, project: p.key }))),
    ),
  ]);
  return { projects, needsMe, tickets: tickets.flat(), requirements: requirements.flat() };
}

export function useInboxData(api: Api, version: number) {
  return useFetch(() => loadInbox(api), [api, version]);
}

export function useSpec(api: Api, project: string | undefined, id: string | undefined, version: number) {
  return useFetch<SpecDetail>(project && id ? () => api.getSpec(project, id) : null, [api, project, id, version]);
}

export function useTicket(api: Api, project: string | undefined, id: string | undefined, version: number) {
  return useFetch<TicketView>(project && id ? () => api.getTicket(project, id) : null, [api, project, id, version]);
}
