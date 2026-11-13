// The Accepted panel (ADR 0006): the human has accepted the work and it
// waits to land in the base. It shows the branch and head commit, then
// Land, which asks for the commit that put the work into the base, and
// Cancel. Under the Branch Landing Method the Ticket is To merge, and the
// panel tells the human to merge the branch themselves.
import { useState } from "react";
import "../../api/actions/land";
import "../../api/actions/cancel";
import "../../api/actions/send_back";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { PullRequestBadge } from "../../ticket/PullRequestBadge";
import { useAct } from "../../ticket/useAct";
import { Button } from "../../controls/Button";
import "./accepted.css";

export function AcceptedPanel({ api, project, ticket: t }: PanelProps) {
  const [commit, setCommit] = useState("");
  const { busy, error, run } = useAct();
  const canLand = commit.trim() !== "" && !busy;
  const toMerge = t.needs_me_reason === "to_merge";
  const pr = t.pull_request;
  const [note, setNote] = useState("");

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
      {toMerge && pr && (
        <>
          <p className="small">
            <PullRequestBadge pr={pr} /> Merge, squash or rebase it on your Code Host and the Factory lands it at the merge commit.
            {pr.state === "conflicting" && " The host reports it conflicting, so it can't merge until it is fixed."} Pull request comments aren't read here: put
            what to fix in the note.
          </p>
          <label className="land-commit">
            <span className="small muted">Note for the fix (required)</span>
            <textarea rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
          </label>
          <p>
            <Button
              type="button"

              disabled={busy || !note.trim()}
              onClick={() => run(() => api.act(project, t.id, "send_back", { note: note.trim() }))}
            >
              Send back to fix
            </Button>
          </p>
        </>
      )}
      {toMerge && !pr && (
        <p className="small">
          The Factory leaves the branch for you: merge {t.branch ? <code>{t.branch}</code> : "it"} into the base and the Factory lands it, or
          Land it yourself with the commit.
        </p>
      )}
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
        <Button
          type="button"
          variant="primary"
          disabled={!canLand}
          onClick={() => run(() => api.act(project, t.id, "land", { commit: commit.trim() }))}
        >
          Land
        </Button>{" "}
        <Button type="button" variant="danger" disabled={busy} onClick={() => run(() => api.act(project, t.id, "cancel", {}))}>
          Cancel
        </Button>
      </p>
    </section>
  );
}

export const panel: PanelDef = { reasons: ["to_merge", "accepted_not_landed"], states: ["Accepted"], Component: AcceptedPanel };
