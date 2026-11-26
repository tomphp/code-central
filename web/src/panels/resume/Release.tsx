// Release is the release action: it warns what it costs, takes an optional
// note, then ends the Claim for good. Shared by the panels that offer to
// resume the session first.
import { useState } from "react";
import "../../api/actions/release";
import { ApiError, type TicketView } from "../../api/types";
import { Button } from "../../controls/Button";
import { useApi } from "../../live";
import "./resume.css";

// Release stays busy once the release is made: the Ticket leaves the panel.
export function Release({ project, ticket: t }: { project: string; ticket: TicketView }) {
  const api = useApi();
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  if (!open) {
    return (
      <p>
        <Button type="button" variant="danger" onClick={() => setOpen(true)}>
          Release…
        </Button>
      </p>
    );
  }

  // After a release, do nothing: the service's notice refetches the UI.
  const release = async () => {
    setBusy(true);
    setError("");
    try {
      const n = note.trim();
      await api.act(project, t.id, "release", n ? { note: n } : {});
    } catch (e) {
      setError(e instanceof ApiError || e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  };

  return (
    <div className="release">
      <p role="alert" className="warning">
        Releasing ends this Claim for good and moves {t.id} back to Ready: a live session's next write will be rejected, and the Claim can't be
        restored.
      </p>
      <label>
        <span className="small muted">Note (optional)</span>
        <textarea rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </label>
      {error && <p className="warning">{error}</p>}
      <p>
        <Button type="button" variant="danger" disabled={busy} onClick={release}>
          Release Claim
        </Button>{" "}
        <Button type="button" disabled={busy} onClick={() => setOpen(false)}>
          Keep the Claim
        </Button>
      </p>
    </div>
  );
}
