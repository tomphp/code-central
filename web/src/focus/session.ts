// The Focus session: what Focus Mode and the Focus popup share, so each is
// only a layout over it. From the Needs Me read model's Focus Items in
// scope it holds the Focus Item on screen and the read of its Ticket, the
// skip list, whether the human acted on the item here, and the brief note
// on what happened to it; it says how many are left, and whether all is
// clear or only skipped Focus Items are left. It also holds the Focus
// Pop-up trigger, which reads the same Focus Item keys. A key's format is
// this module's own: a caller only holds keys to hand back.
import { useEffect, useMemo, useReducer, useState } from "react";
import type { Api } from "../api/api";
import type { FocusPopup, NeedsMeRow, QuestionView, TicketView } from "../api/types";
import { modalOpen, useModalOpen } from "../controls/Modal";
import { ticketOf } from "../format";
import { useTicket } from "../live";
import { useNeedsMe } from "../needsme/context";
import type { NeedsMe } from "../needsme/model";
import type { View } from "../url";

declare const focusKey: unique symbol;

// FocusKey is a Focus Item's key: only this module makes one.
export type FocusKey = string & { readonly [focusKey]: true };

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
  key: FocusKey;
  row: NeedsMeRow;
  // roundKey is the key every Focus Item of one Round shares.
  roundKey?: FocusKey;
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

export interface FocusSessionInput {
  api: Api;
  version: number;
  // project is the Project in scope; none means every Project.
  project?: string;
  // scope keeps only the rows for one Ticket: its own row, or the CI item
  // row that lists it, Focus Items or not. None means every Focus Item.
  scope?: string;
}

export interface FocusSession {
  // item is the Focus Item to show; none while all is clear, only skipped
  // items are left, or a read Round with no open Question left waits for
  // Needs Me to catch up.
  item?: FocusItem;
  // ticket is the latest read of item's Ticket, once there is one.
  ticket?: TicketView;
  // api is the Api to act on item through: its writes record that the
  // human acted on it here.
  api: Api;
  // remaining is every Focus Item in scope, the one on screen included,
  // with each Round's open Questions counted one each.
  remaining: number;
  // clear is true only when nothing in scope needs the human.
  clear: boolean;
  // onlySkipped is true when everything left in scope was skipped.
  onlySkipped: boolean;
  // skip sends item to the back, after every unskipped one; again clears
  // the skip list, to go through the skipped ones from the front.
  skip(): void;
  again(): void;
  // note is the brief note on what happened: "Dealt with elsewhere" once
  // the item on screen left without the human acting on it here, shown
  // over the next item (one the human acted on leaves silently), or what
  // the layout said last. There's only ever one note, the latest.
  note?: string;
  say(text: string): void;
  // keys is the key of every Focus Item in scope, to dismiss.
  keys: FocusKey[];
}

// useFocusSession is one visit's Focus session: skipped and acted live
// here for the session's life only, never sent or put in the URL.
export function useFocusSession({ api, version, project, scope }: FocusSessionInput): FocusSession {
  const needsMe = useNeedsMe();
  const [onScreen, setOnScreen] = useState<FocusItem>();
  const [skipped, setSkipped] = useState<FocusItem[]>([]);
  // actedOn is the key of the item on screen once the human acts on it
  // here, until a refusal takes it back.
  const [actedOn, setActedOn] = useState<FocusKey>();
  const [note, setNote] = useState<Note>();
  // A CI item has no Ticket of its own: its panel shows the first Ticket
  // it lists, whose options act on every one of them.
  const read = useTicket(api, onScreen?.row.project, onScreen && ticketOf(onScreen.row), version);
  const acted = !!onScreen && actedOn === onScreen.key;
  const queue = queueOf({ needsMe, project, scope, onScreen, ticket: read.data, skipped, acted });
  const { item, elsewhere } = queue;
  // What's shown becomes what's on screen, so a later read keeps it there.
  // While a read Round has no open Question left and Needs Me hasn't
  // caught up, the last item stays on screen: dropping it would drop its
  // read, bring back the unread Round and read it again, forever.
  //
  // Moving on shows "Dealt with elsewhere" only when the item left without
  // the human, and only over a next item, so All clear stays calm; moving
  // on otherwise ends it, but not a note the layout said. A Round becoming
  // its first Question, once read, isn't moving on, so it keeps the note.
  const waiting = !item && !queue.clear && !queue.onlySkipped;
  useEffect(() => {
    if (waiting || item?.key === onScreen?.key) return;
    const roundRead = !!onScreen?.roundKey && !onScreen.question && onScreen.roundKey === item?.roundKey;
    setOnScreen(item);
    setActedOn(undefined);
    if (roundRead) return;
    if (elsewhere && item) setNote({ text: "Dealt with elsewhere", elsewhere: true });
    else setNote((n) => (n?.elsewhere ? undefined : n));
  }, [waiting, item, onScreen, elsewhere]);
  // The note is brief: it goes after NOTE_MS, and a newer note starts its
  // own NOTE_MS.
  useEffect(() => {
    if (!note) return;
    const timer = setTimeout(() => setNote(undefined), NOTE_MS);
    return () => clearTimeout(timer);
  }, [note]);
  const itemKey = item?.key;
  const itemApi = useMemo(
    () =>
      itemKey
        ? recordingWrites(api, {
            onWrite: () => setActedOn(itemKey),
            onRefused: () => setActedOn((k) => (k === itemKey ? undefined : k)),
          })
        : api,
    [api, itemKey],
  );
  return {
    item,
    ticket: item && read.data?.id === ticketOf(item.row) ? read.data : undefined,
    api: itemApi,
    remaining: queue.remaining,
    clear: queue.clear,
    onlySkipped: queue.onlySkipped,
    skip: () => item && setSkipped((s) => [...s, item]),
    again: () => setSkipped([]),
    note: note?.text,
    say: (text) => setNote({ text }),
    keys: queue.keys,
  };
}

// Note is the brief note; elsewhere marks "Dealt with elsewhere", which
// moving on ends.
interface Note {
  text: string;
  elsewhere?: boolean;
}

// NOTE_MS is how long a note stays.
const NOTE_MS = 4000;

// WRITES are the Api calls a panel or the Question form makes to act on
// a Ticket.
const WRITES: ReadonlySet<PropertyKey> = new Set<keyof Api>(["act", "unlink"]);

// recordingWrites is api with each write calling onWrite before it's
// forwarded, so the live notice it causes finds it recorded, and
// onRefused if it's refused, as a refused write left the item where it
// was. Every other call goes straight to api.
function recordingWrites(api: Api, { onWrite, onRefused }: { onWrite(): void; onRefused(): void }): Api {
  return new Proxy(api, {
    get(target, prop) {
      const v = Reflect.get(target, prop);
      if (typeof v !== "function") return v;
      if (!WRITES.has(prop)) return v.bind(target);
      return (...args: unknown[]) => {
        onWrite();
        return (v.apply(target, args) as Promise<unknown>).catch((e: unknown) => {
          onRefused();
          throw e;
        });
      };
    },
  });
}

interface QueueInput {
  needsMe: NeedsMe;
  project?: string;
  scope?: string;
  // onScreen is the Focus Item on screen, if any.
  onScreen?: FocusItem;
  // ticket is the latest read of the on-screen item's Ticket, if any.
  ticket?: TicketView;
  // skipped is the Focus Items the human skipped, in skip order.
  skipped: FocusItem[];
  // acted is whether the human acted on the item on screen here.
  acted: boolean;
}

interface Queue {
  item?: FocusItem;
  remaining: number;
  clear: boolean;
  onlySkipped: boolean;
  // elsewhere is true when the item on screen is gone without the human
  // acting on it here: its Ticket left that reason in Needs Me, or its
  // Question is no longer open.
  elsewhere: boolean;
  keys: FocusKey[];
}

// queueOf is the focus queue: which Focus Item to show, how many are left,
// whether all is clear or only skipped Focus Items are left, whether the
// one on screen was dealt with elsewhere, and every key in scope.
function queueOf({ needsMe, project, scope, onScreen, ticket, skipped, acted }: QueueInput): Queue {
  const rows = needsMe.focusItems({ project, ticket: scope });
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
  return {
    item,
    remaining,
    clear: rows.length === 0,
    onlySkipped: !item && items.length > 0,
    elsewhere: !acted && isGone(onScreen, items),
    keys: keysOf(rows, ticket),
  };
}

// keysOf is the key of every Focus Item of rows, given the latest read of
// the on-screen item's Ticket. A Round is keyed as its row as well as by
// each open Question, so it's known by the same key read or not.
function keysOf(rows: NeedsMeRow[], ticket?: TicketView): FocusKey[] {
  return rows.flatMap((row) => {
    const keys = itemsOf(row, ticket).map((i) => i.key);
    return row.reason === "needs_answer" ? [...new Set([roundKeyOf(row), ...keys])] : keys;
  });
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
// is one unread Focus Item, skipped once any of its Questions was: a
// layout only leaves a Round with open Questions when it skipped them all.
function isSkipped(item: FocusItem, skipped: FocusItem[]): boolean {
  const unreadRound = item.roundKey !== undefined && !item.question;
  return skipped.some((s) => s.key === item.key || (unreadRound && s.roundKey === item.roundKey));
}

function itemsOf(row: NeedsMeRow, ticket?: TicketView): FocusItem[] {
  // A Base diverged item is a Project's, with no Ticket ID; a CI item is
  // a Project's pushed commit's.
  if (row.reason !== "needs_answer") return [{ key: keyOf(`${row.id || row.project} ${row.reason}${row.ci_item ? ` ${row.ci_item.commit}` : ""}`), row }];
  const roundKey = roundKeyOf(row);
  if (ticket?.id !== row.id) return [{ key: roundKey, row, roundKey }];
  const round = row.round ? ticket.rounds.find((r) => r.number === row.round?.number) : ticket.rounds.at(-1);
  if (!round) return [];
  // A read Ticket with no open Question left in the Round has none to
  // show, though Needs Me may not have caught up yet.
  return round.questions.flatMap((q, i) =>
    q.status === "open"
      ? [{ key: keyOf(`${roundKey} question ${q.number}`), row, roundKey, question: { q, round: round.number, place: i + 1, count: round.questions.length } }]
      : [],
  );
}

// roundKeyOf is a needs_answer row's Round's key. It comes from the row
// alone, so it's the same before and after the read; a row naming no
// Round means the Ticket's latest.
function roundKeyOf(row: NeedsMeRow): FocusKey {
  return keyOf(`${row.id} round ${row.round?.number ?? "latest"}`);
}

const keyOf = (s: string) => s as FocusKey;

// The Focus Pop-up trigger: whether the automatic Focus popup is open over
// the view showing. It opens while a Focus Item in the view's scope waits
// that the human hasn't dismissed, on the views Focus Pop-up covers, and
// stays open until the popup closes. Closing it, however it closes,
// dismisses every Focus Item in scope then, for the tab's life: only a new
// Focus Item, a key not in Needs Me before, opens it again. It never
// interrupts: it opens only once no other popup is open and no text field
// has focus, and as soon as both clear. A row core doesn't mark as a Focus
// Item never opens it.

// POPUP_VIEWS is the views the popup opens over by itself: never Focus
// Mode, which shows the same queue, nor Options or Project Options, where
// it could cost an edit in a form.
const POPUP_VIEWS: View[] = ["inbox", "board", "activity", "ci", "usage"];

export interface TriggerInput {
  option: FocusPopup;
  view: View;
  // project is the view's Project scope; none means All Projects.
  project?: string;
  // needsMe is the Needs Me read model, none while it loads.
  needsMe?: NeedsMe;
}

interface CheckInput extends TriggerInput {
  // dismissed is the key of every Focus Item the popup closed on, this
  // tab.
  dismissed: ReadonlySet<FocusKey>;
  // open is whether the popup is open now.
  open: boolean;
  // otherPopup is whether another popup is open: any Modal but the
  // popup's own.
  otherPopup: boolean;
  // typing is whether a text field has focus.
  typing: boolean;
}

// popupOpen is whether the popup should be open. Once open it stays open
// until it closes, over dismissed Focus Items too. A Round counts here by
// its row's key alone, which the popup's close dismisses with its
// Questions' keys, so a Round dismissed unread isn't new once read. It
// waits to open while another popup is open or a text field has focus.
function popupOpen({ option, view, project, needsMe, dismissed, open, otherPopup, typing }: CheckInput): boolean {
  if (option !== "on" || !POPUP_VIEWS.includes(view) || !needsMe) return false;
  if (open) return true;
  return !otherPopup && !typing && keysOf(needsMe.focusItems({ project })).some((k) => !dismissed.has(k));
}

// usePopupTrigger holds the dismissed set and whether the popup is open
// for the tab's life: not in the URL, not saved, not shared between tabs,
// and kept across views. close is the popup's onClose, and dismiss the
// Board's Focus popup's onGone, whose Focus Items it holds closed too.
export function usePopupTrigger(input: TriggerInput): {
  open: boolean;
  close(closed: { keys: FocusKey[] }): void;
  dismiss(keys: FocusKey[]): void;
} {
  const [{ dismissed, shown }, dispatch] = useReducer(trigger, { dismissed: new Set<FocusKey>(), shown: false });
  // It opens only once rendered with the signals read live, after every
  // effect of the render: a card's popup loaded from the address mounts in
  // the same render as Needs Me, and counts itself open only in its
  // effect. Another popup or a text field changing renders it again.
  useModalOpen();
  useTextFieldFocused();
  useEffect(() => dispatch({ type: "check", input: { ...input, otherPopup: modalOpen(), typing: isTextField(document.activeElement) } }));
  // Off, or on another view, it closes at once, not a render later.
  const open = shown && popupOpen({ ...input, dismissed, open: true, otherPopup: false, typing: false });
  return {
    open,
    close: ({ keys }) => dispatch({ type: "close", keys }),
    dismiss: (keys) => dispatch({ type: "dismiss", keys }),
  };
}

interface TriggerState {
  dismissed: ReadonlySet<FocusKey>;
  shown: boolean;
}

type TriggerAction =
  | { type: "check"; input: Omit<CheckInput, "dismissed" | "open"> }
  | { type: "close" | "dismiss"; keys: FocusKey[] };

// trigger is the hook's state as it changes: a check against the latest,
// so a close in the same round of effects isn't undone by a check that
// read the state before it.
function trigger(state: TriggerState, action: TriggerAction): TriggerState {
  switch (action.type) {
    case "check": {
      const shown = popupOpen({ ...action.input, dismissed: state.dismissed, open: state.shown });
      return shown === state.shown ? state : { ...state, shown };
    }
    case "close":
      return { dismissed: new Set([...state.dismissed, ...action.keys]), shown: false };
    case "dismiss":
      return { ...state, dismissed: new Set([...state.dismissed, ...action.keys]) };
  }
}

// NOT_TEXT is the input types that take no typing.
const NOT_TEXT = new Set(["button", "checkbox", "color", "file", "hidden", "image", "radio", "range", "reset", "submit"]);

// isTextField is whether el is a text field: a text-like input, a
// textarea, a select or a contenteditable element.
function isTextField(el: EventTarget | null): boolean {
  if (el instanceof HTMLInputElement) return !NOT_TEXT.has(el.type);
  if (el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement) return true;
  return el instanceof HTMLElement && el.closest('[contenteditable]:not([contenteditable="false"])') !== null;
}

// useTextFieldFocused is whether a text field has focus, re-checked on
// every focus change, which renders it again.
function useTextFieldFocused(): boolean {
  const [focused, setFocused] = useState(() => isTextField(document.activeElement));
  useEffect(() => {
    const on = (e: FocusEvent) => setFocused(isTextField(e.type === "focusin" ? e.target : e.relatedTarget));
    document.addEventListener("focusin", on);
    document.addEventListener("focusout", on);
    return () => {
      document.removeEventListener("focusin", on);
      document.removeEventListener("focusout", on);
    };
  }, []);
  return focused;
}
