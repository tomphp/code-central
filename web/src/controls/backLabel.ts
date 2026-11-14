import type { UiLocation } from "../url";

const LABELS = { inbox: "Back to the Inbox", focus: "Back to Focus Mode", activity: "Back to Activity" };

// backLabel names where a view's Back button returns to: the view that
// linked to it (Focus Mode or Activity), otherwise the Inbox.
export function backLabel(loc: Pick<UiLocation, "back">): string {
  const view = loc.back?.view;
  return LABELS[view === "focus" || view === "activity" ? view : "inbox"];
}
