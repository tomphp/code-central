// FocusPopup is the Focus popup: the Focus Item view shared with Focus
// Mode, in the wide modal shell. A layout only, over the Focus session as
// Focus Mode is, so a Round shows its open Questions one at a time, and a
// Focus Item that leaves Needs Me while on screen moves on as there: with
// a brief "Dealt with elsewhere" note unless the human acted on it here.
// It has no + New ▾. It has two modes:
//
//   - Ticket mode is the Board's popup for a Ticket that needs the human,
//     scoped to the one Ticket, Focus Item or not, with no Skip, N left or
//     Open in Inbox. It closes by itself once Needs Me has no row left for
//     the Ticket.
//   - Queue mode is scoped to a Project, or All Projects, over every Focus
//     Item in Needs Me, with Skip, N left and Open in Inbox. It keeps its
//     own skip list, and closes by itself once the queue is clear or only
//     skipped Focus Items are left.
//
// Either way it tells its host why it closed and the key of every Focus
// Item in scope then. It changes no address of its own.
import { useEffect, useRef } from "react";
import type { Api } from "../api/api";
import type { InboxData } from "../live";
import type { UiLocation } from "../url";
import { FocusItemView, inboxOf } from "../focus/FocusItemView";
import { useFocusSession, type FocusKey } from "../focus/session";
import { PopupShell } from "./PopupShell";

// FocusPopupClose is why the popup closed and the keys of the Focus Items
// in scope then: "dismissed" by ×, Escape, a click outside or leaving
// through a link in it such as Open in Inbox; "cleared" once nothing is
// left in scope; "only_skipped" once only skipped Focus Items are.
export interface FocusPopupClose {
  why: "dismissed" | "cleared" | "only_skipped";
  keys: FocusKey[];
}

export type FocusPopupProps = {
  api: Api;
  now: Date;
  version: number;
  data: InboxData;
  onClose(closed: FocusPopupClose): void;
  // onGone hears the popup leave the page however it goes, closed or
  // unmounted by Back or another view, with the keys in scope then.
  onGone?(keys: FocusKey[]): void;
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
  // The session lives for the popup's life; Ticket mode has no Skip, so
  // its skip list stays empty.
  const session = useFocusSession({ api, version, project, scope: queue ? undefined : props.id });
  const { item } = session;
  const close = (why: FocusPopupClose["why"]) => onClose({ why, keys: session.keys });
  const gone = useRef(() => props.onGone?.(session.keys));
  gone.current = () => props.onGone?.(session.keys);
  useEffect(() => () => gone.current(), []);
  // It closes once, as the queue turns clear or, in Queue mode, only
  // skipped Focus Items are left: close is a new function each render, so
  // it isn't a dependency.
  const done = session.clear ? "cleared" : queue && session.onlySkipped ? "only_skipped" : undefined;
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
      head={queue && !session.clear && <span className="focus-left">{session.remaining} left</span>}
    >
      {session.note && (
        <p role="status" className="focus-note">
          {session.note}
        </p>
      )}
      {item ? (
        <FocusItemView
          key={item.key}
          api={session.api}
          now={now}
          data={data}
          item={item}
          ticket={session.ticket}
          navigate={queue ? leave : navigate}
          skip={queue ? session.skip : undefined}
          inbox={queue ? inboxOf(item.row, project) : undefined}
        />
      ) : (
        <p className="empty">Loading…</p>
      )}
    </PopupShell>
  );
}
