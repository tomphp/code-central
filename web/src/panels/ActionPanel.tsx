// The action panel slot: the first thing in the selected Ticket's pane,
// shared by every view. Each panel lives in its own folder,
// src/panels/<name>/panel.tsx, exporting `panel: PanelDef` naming the
// Needs Me reasons it handles; the slot finds them all on its own, so
// adding a panel edits no shared list. See the UI note.
import type { ComponentType } from "react";
import type { Api } from "../api/api";
import type { NeedsMeReason, TicketView } from "../api/types";

export interface PanelProps {
  api: Api;
  project: string;
  ticket: TicketView;
  now: Date;
}

export interface PanelDef {
  reasons: NeedsMeReason[];
  Component: ComponentType<PanelProps>;
}

const modules = import.meta.glob<{ panel: PanelDef }>("./*/panel.tsx", { eager: true });

const byReason = new Map<NeedsMeReason, PanelDef>();
for (const [path, m] of Object.entries(modules)) {
  for (const r of m.panel.reasons) {
    if (byReason.has(r)) throw new Error(`two action panels handle ${r}: ${path} and another`);
    byReason.set(r, m.panel);
  }
}

export function panelFor(reason: NeedsMeReason | undefined): PanelDef | undefined {
  return reason ? byReason.get(reason) : undefined;
}

export function ActionPanel(props: PanelProps) {
  const def = panelFor(props.ticket.needs_me_reason);
  return (
    <section aria-label="Action panel" className="action-slot">
      {/* Keyed on the Ticket and its State so a panel's local state (a
          half-typed note) never carries over to another Ticket or State. */}
      {def && <def.Component key={`${props.ticket.id} ${props.ticket.state}`} {...props} />}
    </section>
  );
}
