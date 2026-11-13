// The Inbox's last grouping of a State list, kept per browser and shared
// with the Board view. The only place the UI touches localStorage; where it
// is unavailable, By Ticket.
import { STATES, type State } from "../api/types";

const KEY = "code-central.inbox.group";

export type Group = "spec" | "ticket";

export function storedGroup(): Group {
  try {
    return localStorage.getItem(KEY) === "spec" ? "spec" : "ticket";
  } catch {
    return "ticket";
  }
}

export function storeGroup(group: Group): void {
  try {
    localStorage.setItem(KEY, group);
  } catch {
    // The choice then lasts only as long as the URL.
  }
}

// Show finished is kept beside the grouping: on only once chosen.
const FINISHED_KEY = "code-central.inbox.finished";

export function storedFinished(): boolean {
  try {
    return localStorage.getItem(FINISHED_KEY) === "1";
  } catch {
    return false;
  }
}

export function storeFinished(on: boolean): void {
  try {
    localStorage.setItem(FINISHED_KEY, on ? "1" : "0");
  } catch {
    // The choice then lasts only as long as the URL.
  }
}

// The Board's hidden columns are kept beside them too, comma-separated;
// none stored is undefined, for the Board's default, and an empty value is
// every column shown.
const HIDDEN_KEY = "code-central.board.hide";

export function storedHidden(): State[] | undefined {
  try {
    const hidden = localStorage.getItem(HIDDEN_KEY)?.split(",");
    return hidden && STATES.filter((s) => hidden.includes(s));
  } catch {
    return undefined;
  }
}

export function storeHidden(hidden: readonly State[]): void {
  try {
    localStorage.setItem(HIDDEN_KEY, hidden.join(","));
  } catch {
    // The choice then lasts only as long as the URL.
  }
}
