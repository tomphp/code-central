// The Stage strip: Grilling · Spec · Tickets · Build n/m, one segment per
// Stage and one for Build, as the service's requirement block gives them.
// A queued segment (waiting for a Factory) is dashed. With onPick its
// segments are buttons that select the segment's Ticket; without, it is a
// picture only (inside a row link). A Requirement made from a Map has a
// done Map segment in place of Grilling, a link to the Map with mapLink.
// One with neither a Grilling Stage nor a Map (Straight to Spec) has a
// skipped Grilling segment, a picture only.
import type { RequirementView, Segment } from "../api/types";
import type { Link } from "../url";
import { segmentName } from "./progress";

export interface StripProps {
  requirement: RequirementView;
  // build is the Requirement's Build Tickets, for onPick and current.
  build?: { id: string }[];
  onPick?(s: Segment): void;
  // current is the selected Ticket, marking the segment it belongs to.
  current?: string;
  // mapLink links the Map segment to its Map.
  mapLink?(map: string): Link;
}

export function Strip({ requirement, build = [], onPick, current, mapLink }: StripProps) {
  const inBuild = !!current && build.some((t) => t.id === current);
  const { done, total } = requirement.build;
  return (
    <ol className="strip" aria-label="Stages">
      {requirement.segments.map((s) => {
        const name = segmentName(s);
        const label = (
          <>
            {name}
            {s.stage === "build" && total > 0 && (
              <span className="seg-n">
                {" "}
                {done}/{total}
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
              <a {...mapLink(s.map)} title={`${name} ${s.map}: ${PROGRESS_TITLE[s.progress]}`}>
                {label}
              </a>
            ) : onPick && !s.map && (s.ticket || s.stage === "build") ? (
              <button type="button" disabled={!s.ticket && !build.length} onClick={() => onPick(s)} title={segmentTitle(s)}>
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
  return `${segmentName(s)}: ${PROGRESS_TITLE[s.progress]}`;
}
