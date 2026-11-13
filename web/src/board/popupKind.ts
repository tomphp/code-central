// popupKind is which popup the Board opens for an ID, from the Inbox data
// it already loads: a Spec's opens its Spec popup, a Ticket's its Focus
// popup while Needs Me has a reason for it (a CI item's Tickets included),
// else its Ticket popup. Any other ID, or any ID before the data loads,
// opens nothing. It is the only place that maps Needs Me to a popup kind.
import { useState } from "react";
import type { InboxData } from "../live";
import { reasonsOf } from "../requirement/progress";

export type PopupKind = "spec" | "focus" | "ticket";

export function popupKind(id: string, data: InboxData | undefined): PopupKind | undefined {
  if (!data) return undefined;
  if (data.specs.some((s) => s.id === id)) return "spec";
  if (!data.tickets.some((t) => t.id === id)) return undefined;
  return reasonsOf(data.needsMe).has(id) ? "focus" : "ticket";
}

// usePopupKind is the popup kind for the open ID, chosen when the ID
// changes (a click, a load, Back or Forward) and then held while it stays
// open, so a Ticket popup whose Ticket starts needing the human stays one.
// Until an ID opens a popup, each render chooses again.
export function usePopupKind(id: string | undefined, data: InboxData | undefined): PopupKind | undefined {
  const [held, setHeld] = useState<{ id?: string; kind?: PopupKind }>({});
  const kind = held.id === id && held.kind ? held.kind : id ? popupKind(id, data) : undefined;
  if (held.id !== id || held.kind !== kind) setHeld({ id, kind });
  return kind;
}
