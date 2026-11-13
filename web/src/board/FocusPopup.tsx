// FocusPopup is the Board's popup for a Ticket that needs the human: the
// Focus Item view shared with Focus Mode, in the wide modal shell, scoped
// to the one Ticket. It holds the item on screen and the read of its Ticket
// as Focus Mode does, so a Round shows its open Questions one at a time,
// and moves on to the Ticket's next Focus Item after each. It has no Skip,
// N left, + New ▾, notes or Open in Inbox, and acts through the plain Api.
// Once Needs Me has no row left for the Ticket, whoever dealt with it, it
// closes by itself, silently.
import { useEffect, useState } from "react";
import type { Api } from "../api/api";
import { ticketOf } from "../format";
import { useTicket, type InboxData } from "../live";
import type { UiLocation } from "../url";
import { FocusItemView } from "../focus/FocusItemView";
import { focusQueue, type FocusItem } from "../focus/queue";
import { PopupShell } from "./PopupShell";

export interface FocusPopupProps {
  api: Api;
  now: Date;
  version: number;
  data: InboxData;
  // id is the Ticket the popup is scoped to.
  id: string;
  onClose(): void;
  navigate(patch: Partial<UiLocation>): void;
}

export function FocusPopup({ api, now, version, data, id, onClose, navigate }: FocusPopupProps) {
  const [onScreen, setOnScreen] = useState<FocusItem>();
  // A CI item has no Ticket of its own: its panel shows the first Ticket
  // it lists, whose options act on every one of them.
  const ticket = useTicket(api, onScreen?.row.project, onScreen && ticketOf(onScreen.row), version);
  const screen = focusQueue({ needsMe: data.needsMe, scope: id, onScreen, ticket: ticket.data, skipped: [], acted: false });
  const item = screen.item;
  // What's shown becomes what's on screen, so a later read keeps it there.
  useEffect(() => {
    if (item?.key !== onScreen?.key) setOnScreen(item);
  }, [item, onScreen]);
  // It closes once, as the queue turns clear: onClose is a new function
  // each render, so it isn't a dependency.
  const { clear } = screen;
  useEffect(() => {
    if (clear) onClose();
  }, [clear]);

  return (
    <PopupShell label="Focus popup" onClose={onClose}>
      {item ? (
        <FocusItemView
          key={item.key}
          api={api}
          now={now}
          data={data}
          item={item}
          ticket={ticket.data?.id === ticketOf(item.row) ? ticket.data : undefined}
          navigate={navigate}
        />
      ) : (
        <p className="empty">Loading…</p>
      )}
    </PopupShell>
  );
}
