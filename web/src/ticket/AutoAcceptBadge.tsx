// The Auto-accept badge: "Auto-accept: <kind>" in the In Review panel's
// heading, in the words of the Auto-accept section of Project Options, or
// "never" for a Ticket with no kind.
import type { AutoAcceptKind } from "../api/types";
import { AUTO_ACCEPT_KIND_NAME } from "./autoAccept";

export function AutoAcceptBadge({ kind }: { kind?: AutoAcceptKind }) {
  return <span className="label auto-accept-badge">{`Auto-accept: ${kind ? AUTO_ACCEPT_KIND_NAME[kind] : "never"}`}</span>;
}
