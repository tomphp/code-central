// The Blocked panel: a Ticket stranded behind a Cancelled Blocker that has
// no Replacement. It names each such Blocker and offers the three ways
// out: remove the Blocker, To Draft, Cancel.
import { useAct } from "../../ticket/useAct";
import type { PanelProps } from "../ActionPanel";
import { Button } from "../../controls/Button";

export function Blocked({ api, project, ticket: t }: PanelProps) {
  const { busy, error, run } = useAct();
  // An open Blocker shown Cancelled is a dead end: either the stored
  // Blocker itself, or (via set) where its Replacement chain ended.
  const stuck = t.blockers.filter((b) => b.state === "Cancelled");
  return (
    <section aria-label="Blocked" className="panel">
      {stuck.map((b) => {
        const stored = b.via ?? b.id;
        return (
          <div key={b.id} className="blocker">
            <p>
              Blocked by <span className="tid">{b.id}</span> {b.title}
              {b.via && <> (replacing {b.via})</>}, Cancelled with no Replacement.
            </p>
            <Button
              variant="primary"
              disabled={busy}
              onClick={() => run(() => api.act(project, t.id, "remove_blocker", { blocker: stored }))}
            >
              Remove {stored} as a Blocker
            </Button>
          </div>
        );
      })}
      <p className="actions">
        {t.state !== "Draft" && (
          <Button  disabled={busy} onClick={() => run(() => api.act(project, t.id, "move", { to_state: "Draft" }))}>
            To Draft
          </Button>
        )}{" "}
        <Button variant="danger" disabled={busy} onClick={() => run(() => api.act(project, t.id, "cancel", {}))}>
          Cancel
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
