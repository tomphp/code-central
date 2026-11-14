// The Focus badge: "Focus: <reason>" at the top of a Focus Item, in the words
// of the Focus section of Project Options.
import type { NeedsMeReason } from "../api/types";
import { REASON_LABEL } from "../format";

export function FocusBadge({ reason }: { reason: NeedsMeReason }) {
  return (
    <span className={`label focus-badge reason-${reason}`}>
      <span className="dot" aria-hidden="true" />
      {`Focus: ${REASON_LABEL[reason]}`}
    </span>
  );
}
