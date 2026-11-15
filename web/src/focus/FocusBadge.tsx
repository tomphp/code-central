// The Focus badge: the Reason badge at the top of a Focus Item, as
// "Focus: <reason>".
import type { NeedsMeReason } from "../api/types";
import { ReasonBadge } from "../controls/ReasonBadge";

export function FocusBadge({ reason }: { reason: NeedsMeReason }) {
  return <ReasonBadge reason={reason} prefix="Focus: " />;
}
