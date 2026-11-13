// The Pull request closed panel (code-central-391): an Accepted Ticket
// whose pull request was closed or abandoned without merging. It links the
// pull request, then offers Send back, to Ready with a note, and Cancel.
// If the pull request is reopened on the host the item clears by itself.
import { useState } from "react";
import "../../api/actions/cancel";
import "../../api/actions/send_back";
import type { PanelDef, PanelProps } from "../ActionPanel";
import { PullRequestBadge } from "../../ticket/PullRequestBadge";
import { useAct } from "../../ticket/useAct";
import { Button } from "../../controls/Button";

export function PullRequestClosedPanel({ api, project, ticket: t }: PanelProps) {
  const { busy, error, run } = useAct();
  const [note, setNote] = useState("");
  return (
    <section aria-label="Pull request closed" className="panel accepted-panel">
      <h3>Pull request closed</h3>
      <p className="small">
        <PullRequestBadge pr={t.pull_request} /> Its pull request was closed without merging, so it waits Accepted until you choose. Reopen it on your Code
        Host and it is watched again; or send the work back to be redone, or cancel it. Pull request comments aren't read here: put what to change in the
        note.
      </p>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
      <label className="land-commit">
        <span className="small muted">Note for the next agent (required)</span>
        <textarea rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
      </label>
      <p>
        <Button
          type="button"
          variant="primary"
          disabled={busy || !note.trim()}
          onClick={() => run(() => api.act(project, t.id, "send_back", { note: note.trim() }))}
        >
          Send back
        </Button>{" "}
        <Button type="button" variant="danger" disabled={busy} onClick={() => run(() => api.act(project, t.id, "cancel", {}))}>
          Cancel
        </Button>
      </p>
    </section>
  );
}

export const panel: PanelDef = { reasons: ["pull_request_closed"], Component: PullRequestClosedPanel };
