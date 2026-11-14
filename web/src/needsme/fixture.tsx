// Needing provides a story or a component test with the Needs Me read
// model over just these Tickets, each needing the human for its reason in
// reasons, if it has one. Only stories and tests import this.
import type { ReactNode } from "react";
import type { NeedsMeReason } from "../api/types";
import type { ProjectTicket } from "../live";
import { NeedsMeProvider } from "./context";
import { needsMeOf } from "./model";

export interface NeedingProps {
  tickets: ProjectTicket[];
  // reasons is why each Ticket that needs the human does, by ID.
  reasons: Record<string, NeedsMeReason>;
  children: ReactNode;
}

export function Needing({ tickets, reasons, children }: NeedingProps) {
  const needsMe = needsMeOf({
    projects: [],
    needsMe: { count: 0, counts: {}, groups: [] },
    tickets: tickets.map((t) => ({ ...t, needs_me_reason: reasons[t.id] })),
    specs: [],
    requirements: [],
    maps: [],
  });
  return <NeedsMeProvider value={needsMe}>{children}</NeedsMeProvider>;
}
