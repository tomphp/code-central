// The Focus Pop-up trigger: whether the automatic Focus popup is open over
// the view showing. It opens while a Focus Item in the view's scope waits
// that the human hasn't dismissed, on the views Focus Pop-up covers, and
// stays open until the popup closes. Closing it, however it closes,
// dismisses every Focus Item in scope then, for the tab's life: only a new
// Focus Item, a key not in Needs Me before, opens it again. It never
// interrupts: it opens only once no other popup is open and no text field
// has focus, and as soon as both clear. A row core doesn't mark as a Focus
// Item never opens it.
import { useEffect, useReducer, useState } from "react";
import type { FocusPopup } from "../api/types";
import type { FocusPopupClose } from "../board/FocusPopup";
import { modalOpen, useModalOpen } from "../controls/Modal";
import type { NeedsMe } from "../needsme/model";
import type { View } from "../url";
import { focusKeys } from "./queue";

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
  // dismissed is the key of every Focus Item the popup closed on, this
  // tab.
  dismissed: ReadonlySet<string>;
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
export function popupOpen({ option, view, project, needsMe, dismissed, open, otherPopup, typing }: TriggerInput): boolean {
  if (option !== "on" || !POPUP_VIEWS.includes(view) || !needsMe) return false;
  if (open) return true;
  return !otherPopup && !typing && focusKeys({ needsMe, project }).some((k) => !dismissed.has(k));
}

// usePopupTrigger holds the dismissed set and whether the popup is open
// for the tab's life: not in the URL, not saved, not shared between tabs,
// and kept across views. close is the popup's onClose, and dismiss the
// Board's Focus popup's onGone, whose Focus Items it holds closed too.
export function usePopupTrigger(input: Omit<TriggerInput, "dismissed" | "open" | "otherPopup" | "typing">): {
  open: boolean;
  close(closed: FocusPopupClose): void;
  dismiss(keys: string[]): void;
} {
  const [{ dismissed, shown }, dispatch] = useReducer(trigger, { dismissed: new Set<string>(), shown: false });
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
  dismissed: ReadonlySet<string>;
  shown: boolean;
}

type TriggerAction =
  | { type: "check"; input: Omit<TriggerInput, "dismissed" | "open"> }
  | { type: "close" | "dismiss"; keys: string[] };

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
