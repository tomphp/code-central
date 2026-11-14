// The Spec's Auto-accept switch, on the plain Spec and Requirement panes.
// Turning it on would accept each of the Spec's Tickets now In Review that
// can be auto-accepted, as core's sweep does.
import type { Api } from "../api/api";
import type { SpecDetail } from "../api/types";
import { projectOf } from "../api/api";
import { AutoAcceptSwitch } from "../ticket/AutoAcceptSwitch";
import { AUTO_ACCEPT_LABEL, canAutoAccept } from "../ticket/autoAccept";

export function SpecAutoAccept({ api, spec, archived }: { api: Api; spec: SpecDetail; archived?: boolean }) {
  if (spec.finished) return null;
  return (
    <AutoAcceptSwitch
      api={api}
      project={projectOf(spec.id)}
      target={spec.id}
      on={spec.labels.includes(AUTO_ACCEPT_LABEL)}
      accepts={spec.tickets.filter((t) => t.state === "In Review" && canAutoAccept(t, spec.labels)).length}
      archived={archived}
    />
  );
}
