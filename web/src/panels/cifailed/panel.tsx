// The CI failed panel (code-central-261): an Accepted Ticket whose CI
// result is failed. It is the Ticket's CI section, with each workflow's
// status, jobs and link, then the failure options, so the Ticket pane and
// the Focus Item both offer them.
import type { PanelDef, PanelProps } from "../ActionPanel";
import { CISection } from "../../ticket/CISection";

export function CIFailedPanel({ api, project, ticket }: PanelProps) {
  return (
    <section aria-label="CI failed" className="panel">
      <h3>CI failed</h3>
      <p className="small">Its pushed commit's CI failed, so it waits Accepted until you choose what to do.</p>
      <CISection api={api} project={project} ticket={ticket} />
    </section>
  );
}

export const panel: PanelDef = { reasons: ["ci_failed"], Component: CIFailedPanel };
