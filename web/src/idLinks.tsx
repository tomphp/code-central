// IdLinks is where an ID Link in rendered Markdown goes: the App provides
// the current location and navigate, so a click opens the Spec or Ticket in
// place; a Focus popup adds a hook to close itself, and the edit form's
// preview opens a new tab, to keep its draft.
import { createContext, useContext, useEffect, useState } from "react";
import type { Api } from "./api/api";
import type { NeedsMe } from "./needsme/model";
import { readLocation, type UiLocation } from "./url";

export interface IdLinks {
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  // onNavigate hears an ID Link navigate.
  onNavigate?(): void;
  // newTab makes ID Links open in a new tab.
  newTab?: boolean;
}

// With no provider an ID Link is the Inbox address for its ID, left to the browser.
export const IdLinksContext = createContext<IdLinks | undefined>(undefined);
export const NO_ID_LINKS: IdLinks = { loc: readLocation("?view=inbox"), navigate: () => {} };

export const useIdLinks = () => useContext(IdLinksContext);

// IdKind is how an ID opens: as a Spec, a Ticket, or a Ticket that needs the human.
export type IdKind = "spec" | "ticket" | "focus";

// useIdKind is the kind of the Spec or Ticket id: from the loaded Needs Me
// read model when it knows the ID, else, once ready, from the server: an
// ID that reads as a Spec in its Project is one, any other is a Ticket (and
// its own pane shows its error when it names nothing). Undefined while it waits.
export function useIdKind(api: Api, id: string | undefined, needsMe: NeedsMe, ready: boolean): IdKind | undefined {
  const known = id ? needsMe.popupKind(id) : undefined;
  const [read, setRead] = useState<{ id: string; kind: IdKind }>();
  const ask = !!id && ready && !known;
  useEffect(() => {
    if (!ask) return;
    let live = true;
    api.getSpec(id.slice(0, id.lastIndexOf("-")), id).then(
      () => live && setRead({ id, kind: "spec" }),
      () => live && setRead({ id, kind: "ticket" }),
    );
    return () => {
      live = false;
    };
  }, [api, id, ask]);
  return known ?? (ask && read?.id === id ? read.kind : undefined);
}
