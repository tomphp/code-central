// TicketPopup is the Board's popup for a Ticket that doesn't need the
// human: the Inbox's Ticket pane, unchanged and with its actions, in the
// wide modal shell. It reads the Ticket on the live version, so it follows
// the feed, and shows Loading… or the read's error as the Inbox's detail
// pane does. TicketPopupView is the popup over a read handed in.
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import type { ApiError, ProjectRow, TicketView } from "../api/types";
import { useTicket } from "../live";
import { TicketPane } from "../ticket/TicketDetail";
import { PopupShell } from "../controls/PopupShell";

export interface TicketPopupProps {
  api: Api;
  now: Date;
  version: number;
  id: string;
  projects: ProjectRow[];
  onClose(): void;
  // openSpec switches the popup to the Ticket's Spec.
  openSpec(id: string): void;
  // openTicket switches the popup to another Ticket, as a new Retro is.
  openTicket(id: string): void;
}

export function TicketPopup({ version, ...props }: TicketPopupProps) {
  const ticket = useTicket(props.api, projectOf(props.id), props.id, version);
  return <TicketPopupView {...props} ticket={ticket} />;
}

export interface TicketPopupViewProps extends Omit<TicketPopupProps, "version"> {
  // ticket is the read of the Ticket: none yet while loading.
  ticket: { data?: TicketView; error?: ApiError };
}

export function TicketPopupView({ api, now, id, projects, ticket, onClose, openSpec, openTicket }: TicketPopupViewProps) {
  const project = projectOf(id);
  const row = projects.find((p) => p.key === project);
  return (
    <PopupShell label="Ticket popup" size="wide" onClose={onClose}>
      {ticket.data?.id === id ? (
        <TicketPane
          api={api}
          project={project}
          ticket={ticket.data}
          now={now}
          archived={!!row?.archived}
          reported={row?.reported_models}
          openSpec={openSpec}
          openTicket={openTicket}
        />
      ) : (
        <p className="empty">{ticket.error ? ticket.error.message : "Loading…"}</p>
      )}
    </PopupShell>
  );
}
