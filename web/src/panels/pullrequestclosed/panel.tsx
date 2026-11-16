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
import { useApi } from "../../live";

export function PullRequestClosedPanel({ project, ticket: t }: PanelProps) {
  const api = useApi();
  const { busy, error, run } = useAct();
  const [note, setNote] = useState("");
  // With several Repositories each closed pull request is named by its
  // Repository, and once a sibling has merged its work is in a base, so
  // the Ticket can't be cancelled (Spec code-central-848).
  const repositories = t.repositories ?? [];
  const closed = repositories.filter((r) => r.pull_request?.state === "closed");
  const merged = repositories.filter((r) => r.pull_request?.state === "merged");
  return (
    <section aria-label="Pull request closed" className="panel accepted-panel">
      <h3>Pull request closed</h3>
      {closed.length === 0 ? (
        <p className="small">
          <PullRequestBadge pr={t.pull_request} /> Its pull request was closed without merging, so it waits Accepted until you choose. Reopen it on your
          Code Host and it is watched again; or send the work back to be redone, or cancel it. Pull request comments aren't read here: put what to
          change in the note.
        </p>
      ) : (
        <>
          <p className="small">
            {closed.length === 1 ? "Its pull request in " + closed[0].name + " was" : "Its pull requests in " + closed.length + " Repositories were"}{" "}
            closed without merging, so it waits Accepted until you choose. Reopen{" "}
            {closed.length === 1 ? "it" : "them"} on the Code Host and {closed.length === 1 ? "it is" : "they are"} watched again; or send the work back
            to be redone{merged.length === 0 && ", or cancel it"}. Pull request comments aren't read here: put what to change in the note.
          </p>
          <ul aria-label="Closed pull requests" className="repos">
            {closed.map((r) => (
              <li key={r.name} aria-label={r.name}>
                <span className="repo-name">{r.name}</span> <PullRequestBadge pr={r.pull_request} />
              </li>
            ))}
          </ul>
          {merged.length > 0 && (
            <p className="small muted">
              Already merged in {merged.map((r) => r.name).join(", ")}, so it can&apos;t be cancelled: sending it back keeps that work landed.
            </p>
          )}
        </>
      )}
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
        </Button>
        {merged.length === 0 && (
          <>
            {" "}
            <Button type="button" variant="danger" disabled={busy} onClick={() => run(() => api.act(project, t.id, "cancel", {}))}>
              Cancel
            </Button>
          </>
        )}
      </p>
    </section>
  );
}

export const panel: PanelDef = { reasons: ["pull_request_closed"], Component: PullRequestClosedPanel };
