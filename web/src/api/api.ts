// The one API module: everything the UI reads, every human action and the
// live feed go through an Api. createHttpApi (http.ts) is the real one;
// FakeApi (fake.ts) is the in-memory fake the component and layout tests
// use. contract.ts runs one suite against both.
import type { ChangeEvent, FindTicketsFilter, NeedsMeView, ProjectRow, ProjectView, TicketRow, TicketView } from "./types";

// TicketActions maps each human action on one Ticket to its request body
// and result. Every one is `POST /api/p/{key}/tickets/{n}/{action}` with
// that body, so adding an action needs no new Api method: a panel's
// ticket declares its action by merging into this interface from its own
// file under ./actions/ (actions/move.ts is the example), which also adds
// the fake's behaviour with registerFakeAction (fakeActions.ts).
//
//   declare module "../api" {
//     interface TicketActions { answer: { body: { question: number; answer: string }; result: TicketView } }
//   }
export interface TicketActions {}
export type TicketAction = keyof TicketActions;

export interface EventHandlers {
  onEvent(e: ChangeEvent): void;
  // onOpen is called each time the feed is live: first with
  // reconnect=false, then reconnect=true after every reconnect, when
  // notices may have been missed and everything should be refetched.
  onOpen?(info: { reconnect: boolean }): void;
}

export interface Api {
  listProjects(): Promise<ProjectRow[]>;
  needsMe(): Promise<NeedsMeView>;
  // findTickets leaves out Done and Cancelled Tickets unless the filter names them.
  findTickets(project: string, filter?: FindTicketsFilter): Promise<TicketRow[]>;
  getTicket(project: string, id: string): Promise<TicketView>;
  act<A extends TicketAction>(
    project: string,
    id: string,
    action: A,
    body: TicketActions[A]["body"],
  ): Promise<TicketActions[A]["result"]>;
  // archiveProject puts away a Project whose Tickets are all finished;
  // an Archived Project refuses every write until unarchiveProject.
  archiveProject(project: string): Promise<ProjectView>;
  unarchiveProject(project: string): Promise<ProjectView>;
  // subscribe opens the live feed and returns a function that closes it.
  subscribe(handlers: EventHandlers): () => void;
}

// projectOf is a display ID's Project Key: everything before the last "-".
export function projectOf(id: string): string {
  const i = id.lastIndexOf("-");
  return i < 0 ? "" : id.slice(0, i);
}

// numberOf is a display ID's number: everything after the last "-".
export function numberOf(id: string): string {
  return id.slice(id.lastIndexOf("-") + 1);
}
