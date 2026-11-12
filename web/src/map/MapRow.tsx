// One Map as a row in the Requirements view: ID and title, and in place
// of a Requirement's Stage strip, how many of its decisions are made and
// how many of its Tickets need the human.
import type { MouseEvent } from "react";
import type { MapView } from "../api/types";
import type { ProjectSpec } from "../live";
import { ShortId } from "../inbox/ShortId";

export interface MapRowProps {
  spec: ProjectSpec & { map: MapView };
  link: { href: string; onClick(e: MouseEvent): void; "data-id": string; "aria-current"?: "true" };
}

export function MapRow({ spec, link }: MapRowProps) {
  const m = spec.map;
  return (
    <a className={`row req-row map-row${spec.finished || !m.needs_you ? " quiet" : ""}`} {...link}>
      <span className="row-top">
        <ShortId id={spec.id} />
        <span className="row-title">{spec.title}</span>
        {m.needs_you > 0 && <span className="dot" role="img" aria-label="Needs you" />}
      </span>
      <span className="map-count">{mapCount(m)}</span>
      <span className="row-detail">{spec.finished ? "Map · Finished" : "Map"}</span>
    </a>
  );
}

// mapCount is a Map's progress as a count: "<decided>/<total> decided",
// then "· <n> needs you" only when something does.
export function mapCount(m: MapView): string {
  const decided = `${m.decided}/${m.total} decided`;
  return m.needs_you > 0 ? `${decided} · ${m.needs_you} needs you` : decided;
}
