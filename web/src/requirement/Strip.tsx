// The Stage strip: Grilling · Spec · Tickets · Build n/m, one segment per
// Stage and one for Build. A queued segment (waiting for a Factory) is
// dashed. With onPick its segments are buttons that select the segment's
// Ticket; without, it is a picture only (inside a row link). A Requirement
// made from a Map has a done Map segment in place of Grilling, a link to
// the Map with mapLink.
import type { Link } from "../url";
import type { Progress, Segment } from "./progress";

export interface StripProps {
  p: Progress;
  onPick?(s: Segment): void;
  // current is the selected Ticket, marking the segment it belongs to.
  current?: string;
  // mapLink links the Map segment to its Map.
  mapLink?(map: string): Link;
}

export function Strip({ p, onPick, current, mapLink }: StripProps) {
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
            className={["seg", `progress-${s.progress}`, s.reason && `reason-${s.reason}`].filter(Boolean).join(" ")}
            data-progress={s.progress}
            aria-current={selected ? "step" : undefined}
          >
            {s.map && mapLink ? (
              <a {...mapLink(s.map)} title={`${s.name} ${s.map}: ${PROGRESS_TITLE[s.progress]}`}>
                {label}
              </a>
            ) : onPick && !s.map ? (
              <button type="button" disabled={!s.ticket && !p.build.length} onClick={() => onPick(s)} title={segmentTitle(s)}>
                {label}
              </button>
            ) : (
              <span title={segmentTitle(s)}>{label}</span>
            )}
          </li>
        );
      })}
    </ol>
  );
}

const PROGRESS_TITLE = { done: "Done", skipped: "skipped", needs: "Needs you", queued: "Waiting for a Factory", running: "A Factory is on it", waiting: "Not started" };

function segmentTitle(s: Segment) {
  return `${s.name}: ${PROGRESS_TITLE[s.progress]}`;
}
