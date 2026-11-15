// TicketPopup is the Board's popup for a Ticket that doesn't need the
// human: the Inbox's Ticket pane, unchanged and with its actions, in the
// wide modal shell. It reads the Ticket on the live version, so it follows
// the feed, and shows Loading… or the read's error as the Inbox's detail
// pane does. TicketPopupView is the popup over a read handed in.
import { projectOf } from "../api/api";
import type { ApiError, ProjectRow, TicketView } from "../api/types";
import { useTicket } from "../live";
import { TicketPane } from "../ticket/TicketDetail";
import { PopupShell } from "../controls/PopupShell";

export interface TicketPopupProps {
  now: Date;
  id: string;
  projects: ProjectRow[];
  onClose(): void;
  // openSpec switches the popup to the Ticket's Spec.
  openSpec(id: string): void;
  // openTicket switches the popup to another Ticket, as a new Retro is.
  openTicket(id: string): void;
}

export function TicketPopup(props: TicketPopupProps) {
  const ticket = useTicket(projectOf(props.id), props.id);
  return <TicketPopupView {...props} ticket={ticket} />;
}

export interface TicketPopupViewProps extends TicketPopupProps {
  // ticket is the read of the Ticket: none yet while loading.
  ticket: { data?: TicketView; error?: ApiError };
}

export function TicketPopupView({ now, id, projects, ticket, onClose, openSpec, openTicket }: TicketPopupViewProps) {
  const project = projectOf(id);
  const row = projects.find((p) => p.key === project);
  return (
    <PopupShell label="Ticket popup" size="wide" onClose={onClose}>
      <TicketPane
        project={project}
        ticket={ticket.data?.id === id ? ticket.data : undefined}
        error={ticket.error?.message}
        now={now}
        archived={!!row?.archived}
        reported={row?.reported_models}
        openSpec={openSpec}
        openTicket={openTicket}
      />
    </PopupShell>
  );
}
