// StopButton stops a Factory Claim's session while the Ticket is stoppable.
// The Claim is kept, so it confirms first, saying so.
import { useState } from "react";
import "../api/actions/stop";
import type { TicketView } from "../api/types";
import { Button } from "../controls/Button";
import { useApi } from "../live";
import { useAct } from "./useAct";

export function StopButton({ project, ticket: t }: { project: string; ticket: TicketView }) {
  const api = useApi();
  const [confirming, setConfirming] = useState(false);
  const { busy, error, run } = useAct();

  if (!t.stoppable) return null;
  if (!confirming) {
    return (
      <Button type="button" onClick={() => setConfirming(true)}>
        Stop
      </Button>
    );
  }
  return (
    <span role="alert" className="stop-confirm">
      Stop {t.id}'s session? The Claim is kept; resume it, take it over or release it later.{" "}
      <Button type="button" variant="danger" disabled={busy} onClick={() => run(() => api.act(project, t.id, "stop", {}))}>
        Stop session
      </Button>{" "}
      <Button type="button" disabled={busy} onClick={() => setConfirming(false)}>
        Keep it running
      </Button>
      {error && <span className="warning"> {error}</span>}
    </span>
  );
}
