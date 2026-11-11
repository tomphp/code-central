// A Project's Factory Status as Code Central serves it in the Project view:
// the dot in the left pane with its tooltip, and the list of Factories that
// both it and Project Options show. The UI never works a status out itself.
import { useId, useState } from "react";
import type { FactoryStatus, FactoryStatusName } from "../api/types";
import { ago } from "../format";

const LABELS: Record<FactoryStatusName, string> = { online: "Online", draining: "Draining", offline: "Offline" };

export const NO_HEARTBEATS = "No Factory has sent a Heartbeat since Code Central started.";

// FactoryList lists each Factory as name on host, its status, its last
// Heartbeat as a relative time and its note.
export function FactoryList({ status, now }: { status: FactoryStatus; now: Date }) {
  if (status.factories.length === 0) return <p className="factory-empty">{NO_HEARTBEATS}</p>;
  return (
    <ul className="factory-list">
      {status.factories.map((f) => (
        <li key={`${f.name} ${f.host}`}>
          <strong>
            {f.name} on {f.host}
          </strong>
          : {LABELS[f.status]}, last Heartbeat {ago(now, f.last_heartbeat)}
          {f.note && <span className="muted"> ({f.note})</span>}
        </li>
      ))}
    </ul>
  );
}

// FactoryStatusDot is the small dot before an active Project's name. It is
// focusable and named for its status, so colour is not the only signal; hover
// or focus shows the tooltip.
export function FactoryStatusDot({ status, now }: { status: FactoryStatus; now: Date }) {
  const [hover, setHover] = useState(false);
  const [focus, setFocus] = useState(false);
  const tipId = useId();
  const shown = hover || focus;
  return (
    <span className="factory-status" onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}>
      <span
        className={`factory-dot ${status.status}`}
        role="img"
        tabIndex={0}
        aria-label={`Factory Status: ${LABELS[status.status]}`}
        aria-describedby={shown ? tipId : undefined}
        onFocus={() => setFocus(true)}
        onBlur={() => setFocus(false)}
        onKeyDown={(e) => e.key === "Escape" && setFocus(false)}
      />
      {shown && (
        <div className="factory-tooltip" role="tooltip" id={tipId}>
          <FactoryList status={status} now={now} />
        </div>
      )}
    </span>
  );
}
