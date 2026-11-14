// The Auto-accept switch (Spec code-central-589): the control on a Ticket's
// detail or a Spec's page by which the human adds or removes that item's
// auto-accept Label. Turning it off writes at once; turning it on asks
// first when it would accept Tickets now In Review.
import { useEffect, useState } from "react";
import type { Api } from "../api/api";
import { Button } from "../controls/Button";
import { AUTO_ACCEPT_LABEL } from "./autoAccept";
import { useAct } from "./useAct";

export interface AutoAcceptSwitchProps {
  api: Api;
  project: string;
  // target is the Ticket or Spec the Label goes on.
  target: string;
  // on is whether the item's own Labels include auto-accept.
  on: boolean;
  // accepts is how many Tickets now In Review turning it on would accept.
  accepts: number;
  // archived shows the switch read-only: the Project takes no edits.
  archived?: boolean;
  // viaSpec is a Ticket's note that its Spec carries the Label.
  viaSpec?: React.ReactNode;
}

export function AutoAcceptSwitch({ api, project, target, on, accepts, archived, viaSpec }: AutoAcceptSwitchProps) {
  const { busy, error, run } = useAct();
  // shown is the switch as the last write left it; the item read again
  // follows it.
  const [shown, setShown] = useState(on);
  const [asking, setAsking] = useState(false);
  useEffect(() => setShown(on), [on]);

  const write = (turnOn: boolean) =>
    run(async () => {
      const v = await (turnOn ? api.addLabels : api.removeLabels).call(api, project, target, [AUTO_ACCEPT_LABEL]);
      setShown(v.labels.includes(AUTO_ACCEPT_LABEL));
      setAsking(false);
    });

  return (
    <div className="auto-accept-switch">
      <label>
        <input
          type="checkbox"
          role="switch"
          checked={shown}
          disabled={archived || busy}
          onChange={(e) => {
            if (!e.target.checked) write(false);
            else if (accepts > 0) setAsking(true);
            else write(true);
          }}
        />{" "}
        Auto-accept
      </label>
      {viaSpec && <span className="muted small"> {viaSpec}</span>}
      {asking && !shown && (
        <form
          aria-label="Turn on auto-accept"
          onSubmit={(e) => {
            e.preventDefault();
            write(true);
          }}
        >
          <p className="small">
            This will accept {accepts} {accepts === 1 ? "Ticket" : "Tickets"} now In Review. Turn on?
          </p>
          <p className="actions">
            <Button type="submit" variant="primary" disabled={busy}>
              Turn on
            </Button>
            <Button type="button" disabled={busy} onClick={() => setAsking(false)}>
              Cancel
            </Button>
          </p>
        </form>
      )}
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </div>
  );
}
