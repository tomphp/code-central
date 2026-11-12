// The Inbox's last grouping of a State list, kept per browser. The only
// place the UI touches localStorage; where it is unavailable, By Ticket.
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
