// The New issue panel (code-central-287): an Incoming Issue that came in
// while New issues need me was on, waiting for the human to triage it,
// with its issue's link and the triage actions (code-central-289 adds
// Won't fix).
import { LinkBadge } from "../../ticket/LinkBadge";
import { TriageActions } from "../../ticket/Triage";
import type { PanelDef, PanelProps } from "../ActionPanel";

export function NewIssue({ api, project, ticket: t }: PanelProps) {
  return (
    <section aria-label="New issue" className="panel">
      <p>
        {t.link && <LinkBadge link={t.link} />} came in from GitHub Issues and waits for you to triage it. Keep as Draft
        leaves it here as it is, out of Needs Me, and writes nothing to GitHub. Won't fix Cancels it and closes the issue
        as not planned, with your reason as a comment.
      </p>
      <TriageActions api={api} project={project} ticket={t} />
    </section>
  );
}

export const panel: PanelDef = { reasons: ["new_issue"], Component: NewIssue };
