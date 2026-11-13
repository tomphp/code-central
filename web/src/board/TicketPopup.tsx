// TicketPopup is the Board's popup for a Ticket that doesn't need the
// human: the Inbox's Ticket pane, unchanged and with its actions, in the
// wide modal shell. It reads the Ticket on the live version, so it follows
// the feed, and shows Loading… or the read's error as the Inbox's detail
// pane does.
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import type { ProjectRow } from "../api/types";
import { useTicket } from "../live";
import { TicketPane } from "../ticket/TicketDetail";
import { PopupShell } from "./PopupShell";

export interface TicketPopupProps {
  api: Api;
  now: Date;
  version: number;
  id: string;
  projects: ProjectRow[];
  onClose(): void;
  // openSpec switches the popup to the Ticket's Spec.
  openSpec(id: string): void;
}

export function TicketPopup({ api, now, version, id, projects, onClose, openSpec }: TicketPopupProps) {
  const project = projectOf(id);
  const ticket = useTicket(api, project, id, version);
  const row = projects.find((p) => p.key === project);
  return (
    <PopupShell label="Ticket popup" onClose={onClose}>
      {ticket.data?.id === id ? (
        <TicketPane
          api={api}
          project={project}
          ticket={ticket.data}
          now={now}
          archived={!!row?.archived}
          reported={row?.reported_models}
          openSpec={openSpec}
        />
      ) : (
        <p className="empty">{ticket.error ? ticket.error.message : "Loading…"}</p>
      )}
    </PopupShell>
  );
}
