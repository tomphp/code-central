// The focus queue: from Needs Me in scope, the Focus Item on screen and
// its fetched Ticket, which Focus Item Focus Mode shows, how many are
// left, and whether all is clear. No React; the Focus layout holds what's
// on screen and feeds it back in.
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
  // round is the key every Focus Item of one Round shares.
  round?: string;
  question?: FocusQuestion;
}

export interface FocusQuestion {
  q: QuestionView;
  // n is the Question's place in its Round, of m.
  n: number;
  m: number;
  round: number;
}

export interface FocusInput {
  needsMe: NeedsMeView;
  // project is the Project in scope; none means every Project.
  project?: string;
  // onScreen is the Focus Item on screen, if any.
  onScreen?: FocusItem;
  // ticket is the latest read of the on-screen item's Ticket, if any.
  ticket?: TicketView;
}

export interface FocusScreen {
  // item is the Focus Item to show; none only while all is clear.
  item?: FocusItem;
  // remaining is every Focus Item in scope, the one on screen included,
  // with each Round's open Questions counted one each.
  remaining: number;
  // clear is true only when nothing in scope needs the human.
  clear: boolean;
}

export function focusQueue({ needsMe, project, onScreen, ticket }: FocusInput): FocusScreen {
  // Needs Me's own order, Stage Tickets as themselves, unlike the Inbox.
  const rows = needsMe.groups.flatMap((g) => g.tickets).filter((r) => !project || r.project === project);
  const items = rows.flatMap((row) => itemsOf(row, ticket));
  // The item on screen stays put while it's still in the queue: nothing
  // arriving, however urgent, replaces it. Once it's gone, its Round's
  // next open Question comes before anything else.
  const item =
    items.find((i) => i.key === onScreen?.key) ?? (onScreen?.round ? items.find((i) => i.round === onScreen.round) : undefined) ?? items[0];
  const remaining = rows.reduce((n, r) => n + (r.reason === "needs_answer" ? (r.round?.open ?? 1) : 1), 0);
  return { item, remaining, clear: rows.length === 0 };
}

function itemsOf(row: NeedsMeRow, ticket?: TicketView): FocusItem[] {
  if (row.reason !== "needs_answer") return [{ key: `${row.id} ${row.reason}`, row }];
  const read = ticket?.id === row.id ? ticket : undefined;
  const round = read ? (row.round ? read.rounds.find((r) => r.number === row.round!.number) : read.rounds.at(-1)) : undefined;
  const number = round?.number ?? row.round?.number;
  const key = `${row.id} round ${number}`;
  if (!read) return [{ key, row, round: key }];
  // A read Ticket with no open Question left in the Round has none to
  // show, though Needs Me may not have caught up yet.
  return (round?.questions ?? []).flatMap((q, i, qs) =>
    q.status === "open"
      ? [{ key: `${key} question ${q.number}`, row, round: key, question: { q, n: i + 1, m: qs.length, round: round!.number } }]
      : [],
  );
}
