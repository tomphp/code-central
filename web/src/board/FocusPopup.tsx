// FocusPopup is the Focus popup: the Focus Item view shared with Focus
// Mode, in the wide modal shell. It holds the item on screen and the read
// of its Ticket as Focus Mode does, so a Round shows its open Questions one
// at a time, and a Focus Item that leaves Needs Me while on screen, whoever
// dealt with it, moves on silently. It has no + New ▾ or notes, and acts
// through the plain Api. It has two modes:
//
//   - Ticket mode is the Board's popup for a Ticket that needs the human,
//     scoped to the one Ticket, whatever its Project's Focus Kinds, with
//     no Skip, N left or Open in Inbox. It closes by itself once Needs Me
//     has no row left for the Ticket.
//   - Queue mode is scoped to a Project, or All Projects, over every Focus
//     Item in Needs Me, of a reason its Project's Focus Kinds leave on,
//     with Skip, N left and Open in Inbox. It keeps its own skip list, and
//     closes by itself once the queue is clear or only skipped Focus Items
//     are left.
//
// Either way it tells its host why it closed and the key of every Focus
// Item in scope then. It changes no address of its own.
import { useEffect, useRef, useState } from "react";
import type { Api } from "../api/api";
import { ticketOf } from "../format";
import { useTicket, type InboxData } from "../live";
import type { UiLocation } from "../url";
import { FocusItemView, inboxOf } from "../focus/FocusItemView";
import { focusKeys, focusQueue, type FocusItem } from "../focus/queue";
import { PopupShell } from "./PopupShell";

// FocusPopupClose is why the popup closed and the keys of the Focus Items
// in scope then: "dismissed" by ×, Escape, a click outside or leaving
// through a link in it such as Open in Inbox; "cleared" once nothing is
// left in scope; "only_skipped" once only skipped Focus Items are.
export interface FocusPopupClose {
  why: "dismissed" | "cleared" | "only_skipped";
  keys: string[];
}

export type FocusPopupProps = {
  api: Api;
  now: Date;
  version: number;
  data: InboxData;
  onClose(closed: FocusPopupClose): void;
  // onGone hears the popup leave the page however it goes, closed or
  // unmounted by Back or another view, with the keys in scope then.
  onGone?(keys: string[]): void;
  navigate(patch: Partial<UiLocation>): void;
} & (
  | {
      mode: "ticket";
      // id is the Ticket the popup is scoped to.
      id: string;
    }
  | {
      mode: "queue";
      // project is the Project in scope; none means All Projects.
      project?: string;
    }
);

export function FocusPopup(props: FocusPopupProps) {
  const { api, now, version, data, onClose, navigate } = props;
  const queue = props.mode === "queue";
  const project = queue ? props.project : undefined;
  const scope = queue ? undefined : props.id;
  const [onScreen, setOnScreen] = useState<FocusItem>();
  // skipped lives here for the popup's life, as Focus Mode's does for a
  // visit; Ticket mode has no Skip, so it stays empty there.
  const [skipped, setSkipped] = useState<FocusItem[]>([]);
  // A CI item has no Ticket of its own: its panel shows the first Ticket
  // it lists, whose options act on every one of them.
  const ticket = useTicket(api, onScreen?.row.project, onScreen && ticketOf(onScreen.row), version);
  const screen = focusQueue({ needsMe: data.needsMe, projects: data.projects, project, scope, onScreen, ticket: ticket.data, skipped, acted: false });
  const item = screen.item;
  // What's shown becomes what's on screen, so a later read keeps it there.
  // While nothing can be shown (a read Round with no open Question, before
  // Needs Me catches up) the last item stays on screen: dropping it would
  // drop its read, bring back the unread Round and read it again, forever.
  useEffect(() => {
    if (item && item.key !== onScreen?.key) setOnScreen(item);
  }, [item, onScreen]);
  const keys = () => focusKeys({ needsMe: data.needsMe, projects: data.projects, project, scope, ticket: ticket.data });
  const close = (why: FocusPopupClose["why"]) => onClose({ why, keys: keys() });
  const gone = useRef(() => props.onGone?.(keys()));
  gone.current = () => props.onGone?.(keys());
  useEffect(() => () => gone.current(), []);
  // It closes once, as the queue turns clear or, in Queue mode, only
  // skipped Focus Items are left: close is a new function each render, so
  // it isn't a dependency.
  const done = screen.clear ? "cleared" : queue && screen.onlySkipped ? "only_skipped" : undefined;
  useEffect(() => {
    if (done) close(done);
  }, [done]);
  // Leaving through a link in a Queue mode item, Open in Inbox or Base
  // missing's Project Options, dismisses the popup on the way.
  const leave = (patch: Partial<UiLocation>) => {
    close("dismissed");
    navigate(patch);
  };

  return (
    // Queue mode is the automatic Focus popup's, which doesn't wait on
    // itself as another popup.
    <PopupShell
      label="Focus popup"
      uncounted={queue}
      onClose={() => close("dismissed")}
      head={queue && !screen.clear && <span className="focus-left">{screen.remaining} left</span>}
    >
      {item ? (
        <FocusItemView
          key={item.key}
          api={api}
          now={now}
          data={data}
          item={item}
          ticket={ticket.data?.id === ticketOf(item.row) ? ticket.data : undefined}
          navigate={queue ? leave : navigate}
          skip={queue ? () => setSkipped([...skipped, item]) : undefined}
          inbox={queue ? inboxOf(item.row, project) : undefined}
        />
      ) : (
        <p className="empty">Loading…</p>
      )}
    </PopupShell>
  );
}
