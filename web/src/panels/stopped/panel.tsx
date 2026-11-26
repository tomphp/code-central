// The Stopped panel: when the Claim was stopped, then Resume (the cheap way
// back), the Resume Command to take the session over by hand, and Release,
// which warns what it costs first.
import "../../api/actions/resume";
import { ago } from "../../format";
import { Button } from "../../controls/Button";
import { useApi } from "../../live";
import { useAct } from "../../ticket/useAct";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { Release } from "../resume/Release";
import { ResumeCommand, sessionOf } from "../resume/ResumeCommand";

// Stopped says when the Claim was stopped, falling back to when the Ticket last changed.
function Stopped({ project, ticket: t, now }: PanelProps) {
  const api = useApi();
  const { busy, error, run } = useAct();
  const stopped = t.claims?.find((c) => !c.ended)?.stops?.at(-1)?.stopped ?? t.updated;
  return (
    <section aria-label="Stopped Claim" className="panel">
      <p>Stopped {ago(now, stopped)}</p>
      <p>
        <Button type="button" variant="primary" disabled={busy} onClick={() => run(() => api.act(project, t.id, "resume", {}))}>
          Resume
        </Button>
      </p>
      {error && <p className="warning">{error}</p>}
      <ResumeCommand {...sessionOf(t)} copyLabel="Copy resume command" />
      <Release project={project} ticket={t} />
    </section>
  );
}

export const panel: PanelDef = { reasons: ["stopped"], Component: Stopped };
