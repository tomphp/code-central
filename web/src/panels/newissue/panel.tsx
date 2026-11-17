// The New issue panel (code-central-287): an Incoming Issue that came in
// while New issues need me was on, waiting for the human to triage it,
// with its issue's link and the triage actions (code-central-289 adds
// Won't fix, code-central-290 To Requirement). What each does is the
// Ticket's triage notes, which follow its Integration.
import { LinkBadge } from "../../ticket/LinkBadge";
import { TriageActions, incomingFrom, triageSummary } from "../../ticket/Triage";
import type { PanelDef, PanelProps } from "../ActionPanel";

export function NewIssue({ project, ticket: t }: PanelProps) {
  return (
    <section aria-label="New issue" className="panel">
      <p>
        {t.link && <LinkBadge link={t.link} />} came in from {incomingFrom(t)} and waits for you to triage it.{" "}
        {triageSummary(t)}
      </p>
      <TriageActions project={project} ticket={t} />
    </section>
  );
}

export const panel: PanelDef = { reasons: ["new_issue"], Component: NewIssue };
