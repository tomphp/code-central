// The Accepted panel (ADR 0006): the human has accepted the work and it
// waits to land in the base. It shows the branch and head commit, then
// Land, which asks for the commit that put the work into the base, and
// Cancel.
import { useState } from "react";
import "../../api/actions/land";
import "../../api/actions/cancel";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { useAct } from "../../ticket/useAct";
import "./accepted.css";

export function AcceptedPanel({ api, project, ticket: t }: PanelProps) {
  const [commit, setCommit] = useState("");
  const { busy, error, run } = useAct();
  const canLand = commit.trim() !== "" && !busy;

  return (
    <section aria-label="Accepted" className="panel accepted-panel">
      <h3>Accepted, waiting to land</h3>
      <p className="small">
        {t.branch ? (
          <>
            Branch <code>{t.branch}</code>
            {t.commit && (
              <>
                {" "}
                at <code>{t.commit}</code>
              </>
            )}
            . It is Done once it is merged into the base and landed.
          </>
        ) : (
          "It is Done once it is merged into the base and landed."
        )}
      </p>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
      <label className="land-commit">
        <span className="small muted">Commit in the base (required to land)</span>
        <input type="text" value={commit} onChange={(e) => setCommit(e.target.value)} />
      </label>
      <p>
        <button
          type="button"
          className="btn primary"
          disabled={!canLand}
          onClick={() => run(() => api.act(project, t.id, "land", { commit: commit.trim() }))}
        >
          Land
        </button>{" "}
        <button type="button" className="btn danger" disabled={busy} onClick={() => run(() => api.act(project, t.id, "cancel", {}))}>
          Cancel
        </button>
      </p>
    </section>
  );
}

export const panel: PanelDef = { reasons: ["accepted_not_landed"], states: ["Accepted"], Component: AcceptedPanel };
