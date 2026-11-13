// The closed-externally panel (ticket 42): a linked Ticket whose external
// item was closed, deleted or left its Integration's query. Code Central
// never moves State for it, so the human decides: Cancel, Mark Done (the
// human's move to Done) or Unlink.
import type { ExternalState } from "../../api/types";
import { LinkBadge } from "../../ticket/LinkBadge";
import { useAct } from "../../ticket/useAct";
import type { PanelProps } from "../ActionPanel";
import { Button } from "../../controls/Button";

const WHAT: Record<ExternalState, string> = {
  open: "is open",
  closed: "was closed",
  deleted: "was deleted",
  out_of_scope: "left the Integration's query",
};

export function ClosedExternally({ api, project, ticket: t }: PanelProps) {
  const { busy, error, run } = useAct();
  const l = t.link;
  return (
    <section aria-label="Closed externally" className="panel">
      {l && (
        <p>
          <LinkBadge link={l} /> {WHAT[l.external_state]}. This Ticket is still {t.state}.
        </p>
      )}
      <p className="actions">
        <Button variant="danger" disabled={busy} onClick={() => run(() => api.act(project, t.id, "cancel", {}))}>
          Cancel
        </Button>{" "}
        <Button
          variant="primary"
          disabled={busy}
          onClick={() => run(() => api.act(project, t.id, "move", { to_state: "Done" }))}
        >
          Mark Done
        </Button>{" "}
        <Button  disabled={busy} onClick={() => run(() => api.unlink(project, t.id))}>
          Unlink
        </Button>
      </p>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </section>
  );
}
