// The Focus Pop-up trigger: whether the automatic Focus popup is open over
// the view showing. It opens while a Focus Item in the view's scope waits
// that the human hasn't dismissed, on the views Focus Pop-up covers, and
// stays open until the popup closes. Closing it, however it closes,
// dismisses every Focus Item in scope then, for the tab's life: only a new
// Focus Item, a key not in Needs Me before, opens it again.
import { useEffect, useState } from "react";
import type { FocusPopup, NeedsMeView } from "../api/types";
import type { FocusPopupClose } from "../board/FocusPopup";
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
  // needsMe is none while it loads.
  needsMe?: NeedsMeView;
  // dismissed is the key of every Focus Item the popup closed on, this
  // tab.
  dismissed: ReadonlySet<string>;
  // open is whether the popup is open now.
  open: boolean;
}

// popupOpen is whether the popup should be open. Once open it stays open
// until it closes, over dismissed Focus Items too. A Round counts here by
// its row's key alone, which the popup's close dismisses with its
// Questions' keys, so a Round dismissed unread isn't new once read.
export function popupOpen({ option, view, project, needsMe, dismissed, open }: TriggerInput): boolean {
  if (option !== "on" || !POPUP_VIEWS.includes(view) || !needsMe) return false;
  return open || focusKeys({ needsMe, project }).some((k) => !dismissed.has(k));
}

// usePopupTrigger holds the dismissed set and whether the popup is open
// for the tab's life: not in the URL, not saved, not shared between tabs,
// and kept across views. close is the popup's onClose.
export function usePopupTrigger(input: Omit<TriggerInput, "dismissed" | "open">): { open: boolean; close(closed: FocusPopupClose): void } {
  const [dismissed, setDismissed] = useState<ReadonlySet<string>>(new Set());
  const [shown, setShown] = useState(false);
  const open = popupOpen({ ...input, dismissed, open: shown });
  useEffect(() => setShown(open), [open]);
  const close = ({ keys }: FocusPopupClose) => {
    setDismissed((d) => new Set([...d, ...keys]));
    setShown(false);
  };
  return { open, close };
}
