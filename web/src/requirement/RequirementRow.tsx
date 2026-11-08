// One Requirement as a row: ID and title, its Stage strip and a one-line
// headline of where it is.
import type { MouseEvent } from "react";
import { REASON_LABEL } from "../format";
import type { ProjectSpec } from "../live";
import type { Progress } from "./progress";
import { Strip } from "./Strip";

export interface RequirementRowProps {
  spec: ProjectSpec;
  p: Progress;
  link: { href: string; onClick(e: MouseEvent): void; "data-id": string; "aria-current"?: "true" };
}

export function RequirementRow({ spec, p, link }: RequirementRowProps) {
  return (
    <a className={`row req-row${spec.finished || !p.needs.length ? " quiet" : ""}`} {...link}>
      <span className="row-top">
        <span className="tid">{spec.id}</span>
        <span className="row-title">{spec.title}</span>
        {p.needs[0] && <span className={`dot reason-${p.needs[0].reason}`} role="img" aria-label="Needs you" />}
      </span>
      <Strip p={p} />
      <span className="row-detail">{headline(spec, p)}</span>
    </a>
  );
}

// headline is a Requirement's one line: what needs the human, else what
// it is waiting for.
export function headline(spec: ProjectSpec, p: Progress): string {
  if (spec.finished) return p.total ? `Finished · ${p.done} of ${p.total} Tickets Done` : "Finished";
  const n = p.needs[0];
  if (n) {
    const where = n.stage ? p.segments.find((s) => s.stage === n.stage)!.name : "Build";
    return `${where}: ${REASON_LABEL[n.reason]}${p.needs.length > 1 ? ` · +${p.needs.length - 1} more` : ""}`;
  }
  if (p.waitingForFactory) return `${p.current.name}: waiting for a Factory`;
  if (p.current.stage === "build") return p.total ? `Build: ${p.done} of ${p.total} Done` : "Build: no Tickets yet";
  return `${p.current.name}: ${p.current.progress === "running" ? "a Factory is on it" : "not started"}`;
}
