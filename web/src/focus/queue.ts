// The focus queue: from Needs Me in scope, the skip list, the Focus Item
// on screen, its fetched Ticket and whether the human acted on it here,
// which Focus Item Focus Mode shows, how many are left, whether all is
// clear or only skipped Focus Items are left, and whether the one on
// screen was dealt with elsewhere. No React; the Focus layout holds
// what's on screen and feeds it back in.
import type { NeedsMeRow, NeedsMeView, QuestionView, TicketView } from "../api/types";

// A Focus Item is one Ticket waiting for one Needs Me reason, its key
// (Ticket ID, reason), or one open Question of a Round, its key (Ticket
// ID, Round number, Question number). A Ticket whose reason changes is a
// new Focus Item.
//
// A needs_answer row stands for its Round's open Questions, which only
// its fetched Ticket names: until that Ticket is read, the row is one
// Focus Item for the whole Round, with no question, shown while the read
// is under way.
export interface FocusItem {
  key: string;
  row: NeedsMeRow;
  // roundKey is the key every Focus Item of one Round shares.
  roundKey?: string;
  question?: FocusQuestion;
}

export interface FocusQuestion {
  q: QuestionView;
  // round is the Round's number; place is the Question's place among the
  // Round's count Questions, open or not.
  round: number;
  place: number;
  count: number;
}

export interface FocusInput {
  needsMe: NeedsMeView;
  // project is the Project in scope; none means every Project.
  project?: string;
  // onScreen is the Focus Item on screen, if any.
  onScreen?: FocusItem;
  // ticket is the latest read of the on-screen item's Ticket, if any.
  ticket?: TicketView;
  // skipped is the Focus Items the human skipped this visit, in skip
  // order.
  skipped: FocusItem[];
  // acted is whether the human acted on the item on screen here, in
  // Focus Mode.
  acted: boolean;
}

export interface FocusScreen {
  // item is the Focus Item to show; none while all is clear or only
  // skipped items are left.
  item?: FocusItem;
  // remaining is every Focus Item in scope, the one on screen included,
  // with each Round's open Questions counted one each.
  remaining: number;
  // clear is true only when nothing in scope needs the human.
  clear: boolean;
  // onlySkipped is true when everything left in scope was skipped.
  onlySkipped: boolean;
  // elsewhere is true when the item on screen is gone without the human
  // acting on it here: its Ticket left that reason in Needs Me, or its
  // Question is no longer open.
  elsewhere: boolean;
}

export function focusQueue({ needsMe, project, onScreen, ticket, skipped, acted }: FocusInput): FocusScreen {
  // Needs Me's own order, Stage Tickets as themselves, unlike the Inbox.
  const rows = needsMe.groups.flatMap((g) => g.tickets).filter((r) => !project || r.project === project);
  const items = rows.flatMap((row) => itemsOf(row, ticket));
  // Skipped items go to the back, after every unskipped one. They come
  // back only when the human goes through them again, which clears the
  // skip list and starts from the front, so their own order never shows.
  const unskipped = items.filter((i) => !isSkipped(i, skipped));
  // The item on screen stays put while it's still in the queue and not
  // skipped: nothing arriving, however urgent, replaces it. Once it's gone,
  // its Round's next open, unskipped Question comes before anything else.
  const item =
    unskipped.find((i) => i.key === onScreen?.key) ??
    (onScreen?.roundKey ? unskipped.find((i) => i.roundKey === onScreen.roundKey) : undefined) ??
    unskipped[0];
  const remaining = rows.reduce((n, r) => n + (r.reason === "needs_answer" ? (r.round?.open ?? 1) : 1), 0);
  return { item, remaining, clear: rows.length === 0, onlySkipped: !item && items.length > 0, elsewhere: !acted && isGone(onScreen, items) };
}

// isGone is whether the item on screen has left the queue, skipped items
// included. A Round read since it went on screen isn't gone: it became
// its open Questions.
function isGone(onScreen: FocusItem | undefined, items: FocusItem[]): boolean {
  if (!onScreen) return false;
  const unreadRound = onScreen.roundKey !== undefined && !onScreen.question;
  return !items.some((i) => i.key === onScreen.key || (unreadRound && i.roundKey === onScreen.roundKey));
}

// isSkipped is whether the human skipped the item. A Round not on screen
// is one unread Focus Item, skipped once any of its Questions was: Focus
// Mode only leaves a Round with open Questions when it skipped them all.
function isSkipped(item: FocusItem, skipped: FocusItem[]): boolean {
  const unreadRound = item.roundKey !== undefined && !item.question;
  return skipped.some((s) => s.key === item.key || (unreadRound && s.roundKey === item.roundKey));
}

function itemsOf(row: NeedsMeRow, ticket?: TicketView): FocusItem[] {
  // A Base diverged item is a Project's, with no Ticket ID.
  if (row.reason !== "needs_answer") return [{ key: `${row.id || row.project} ${row.reason}`, row }];
  // The key comes from the row alone, so it's the same before and after
  // the read; a row naming no Round means the Ticket's latest.
  const roundKey = `${row.id} round ${row.round?.number ?? "latest"}`;
  if (ticket?.id !== row.id) return [{ key: roundKey, row, roundKey }];
  const round = row.round ? ticket.rounds.find((r) => r.number === row.round?.number) : ticket.rounds.at(-1);
  if (!round) return [];
  // A read Ticket with no open Question left in the Round has none to
  // show, though Needs Me may not have caught up yet.
  return round.questions.flatMap((q, i) =>
    q.status === "open"
      ? [{ key: `${roundKey} question ${q.number}`, row, roundKey, question: { q, round: round.number, place: i + 1, count: round.questions.length } }]
      : [],
  );
}
