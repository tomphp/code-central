// The Stage strip: Grilling · Spec · Tickets · Build n/m, one segment per
// step. A queued step (waiting for a Factory) is dashed. With onPick its
// segments are buttons that select the step's Ticket; without, it is a
// picture only (inside a row link).
import type { Progress, Segment } from "./progress";

export interface StripProps {
  p: Progress;
  onPick?(s: Segment): void;
  // current is the selected Ticket, marking the segment it belongs to.
  current?: string;
}

export function Strip({ p, onPick, current }: StripProps) {
  const inBuild = !!current && p.build.some((t) => t.id === current);
  return (
    <ol className="strip" aria-label="Stages">
      {p.segments.map((s) => {
        const label = (
          <>
            {s.name}
            {s.stage === "build" && p.total > 0 && (
              <span className="seg-n">
                {" "}
                {p.done}/{p.total}
              </span>
            )}
          </>
        );
        const selected = s.ticket ? s.ticket === current : s.stage === "build" && inBuild;
        return (
          <li
            key={s.stage}
            className={["seg", `step-${s.step}`, s.reason && `reason-${s.reason}`].filter(Boolean).join(" ")}
            data-step={s.step}
            aria-current={selected ? "step" : undefined}
          >
            {onPick ? (
              <button type="button" disabled={!s.ticket && !p.build.length} onClick={() => onPick(s)} title={stepTitle(s)}>
                {label}
              </button>
            ) : (
              <span title={stepTitle(s)}>{label}</span>
            )}
          </li>
        );
      })}
    </ol>
  );
}

const STEP_TITLE = { done: "Done", needs: "Needs you", queued: "Waiting for a Factory", running: "A Factory is on it", waiting: "Not started" };

function stepTitle(s: Segment) {
  return `${s.name}: ${STEP_TITLE[s.step]}`;
}
