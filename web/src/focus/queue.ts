// The focus queue: from Needs Me in scope and the Focus Item on screen,
// which Focus Item Focus Mode shows, how many are left, and whether all
// is clear. No React; the Focus layout holds what's on screen and feeds
// it back in.
import type { NeedsMeRow, NeedsMeView } from "../api/types";

// A Ticket Focus Item is one Ticket waiting for one Needs Me reason; its
// key is its identity, (Ticket ID, reason). A Ticket whose reason changes
// is a new Focus Item.
export interface FocusItem {
  key: string;
  row: NeedsMeRow;
}

export interface FocusInput {
  needsMe: NeedsMeView;
  // project is the Project in scope; none means every Project.
  project?: string;
  // onScreen is the key of the Focus Item on screen, if any.
  onScreen?: string;
}

export interface FocusScreen {
  // item is the Focus Item to show; none while all is clear, or while
  // only Questions, not yet shown as Focus Items, are left.
  item?: FocusItem;
  // remaining is every Focus Item in scope, the one on screen included,
  // with each Round's open Questions counted one each.
  remaining: number;
  // clear is true only when nothing in scope needs the human.
  clear: boolean;
}

export function focusQueue({ needsMe, project, onScreen }: FocusInput): FocusScreen {
  // Needs Me's own order, Stage Tickets as themselves, unlike the Inbox.
  const rows = needsMe.groups.flatMap((g) => g.tickets).filter((r) => !project || r.project === project);
  const items = rows.filter((r) => r.reason !== "needs_answer").map((row) => ({ key: `${row.id} ${row.reason}`, row }));
  // The item on screen stays put while it's still in the queue: nothing
  // arriving, however urgent, replaces it.
  const item = items.find((i) => i.key === onScreen) ?? items[0];
  const remaining = rows.reduce((n, r) => n + (r.reason === "needs_answer" ? (r.round?.open ?? 1) : 1), 0);
  return { item, remaining, clear: rows.length === 0 };
}
