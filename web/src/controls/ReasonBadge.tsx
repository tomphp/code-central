import type { NeedsMeReason } from "../api/types";
import { REASON_LABEL } from "../format";

// ReasonBadge is the Reason badge: a Ticket's Needs Me reason as a dot and
// the words the Focus section of Project Options uses. prefix comes before
// the words, as the Focus badge's "Focus: ".
export function ReasonBadge({ reason, prefix = "" }: { reason: NeedsMeReason; prefix?: string }) {
  return (
    <span className={`reason reason-${reason}`}>
      <span className="dot" aria-hidden="true" />
      {prefix + REASON_LABEL[reason]}
    </span>
  );
}
