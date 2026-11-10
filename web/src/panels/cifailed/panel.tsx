// The CI failed panel (code-central-261): an Accepted Ticket whose CI
// result is failed. It is the Ticket's CI section, with each workflow's
// status, jobs and link, then the failure options, so the Ticket pane and
// the Focus Item both offer them; then the Accepted panel, so Land with a
// commit and Cancel stay at hand while it waits.
import type { PanelDef, PanelProps } from "../ActionPanel";
import { AcceptedPanel } from "../accepted/panel";
import { CISection } from "../../ticket/CISection";

export function CIFailedPanel(props: PanelProps) {
  const { api, project, ticket } = props;
  return (
    <>
      <section aria-label="CI failed" className="panel">
        <h3>CI failed</h3>
        <p className="small">Its pushed commit's CI failed, so it waits Accepted until you choose what to do.</p>
        <CISection api={api} project={project} ticket={ticket} />
      </section>
      <AcceptedPanel {...props} />
    </>
  );
}

export const panel: PanelDef = { reasons: ["ci_failed"], Component: CIFailedPanel };
