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
