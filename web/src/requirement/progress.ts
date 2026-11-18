// A Requirement's progress as the UI words it. Its Stage strip's segments,
// current segment, needs and default Ticket are Go's, on the service's
// requirement block (ADR 0063); this names the segments and words the
// headline and what it waits for.
import { REASON_LABEL } from "../format";
import type { RequirementView, Segment, SegmentStage, StageName } from "../api/types";

// MAP_TITLE names the segment a Requirement made from a Map has in
// place of its grilling Stage.
export const MAP_TITLE = "Map";

export const STAGE_TITLE: Record<SegmentStage, string> = {
  grilling: "Grilling",
  spec: "Spec",
  tickets: "Tickets",
  build: "Build",
};

// segmentName is a segment's title: Map for the one a Map stands in for,
// else its Stage's.
export const segmentName = (s: Segment): string => (s.map ? MAP_TITLE : STAGE_TITLE[s.stage]);

// currentSegment is a Requirement's current segment, the first neither
// done nor skipped, else Build.
const currentSegment = (req: RequirementView): Segment => req.segments.find((s) => s.stage === req.current)!;

const STAGES: StageName[] = ["grilling", "spec", "tickets"];

// isStage reports whether a Ticket is a Stage Ticket: one of its Labels is
// a Stage's, stage:grilling, stage:spec or stage:tickets, as core says.
export const isStage = (t: { labels: string[] }) => !!stageOf(t);

// stageOf is a Stage Ticket's Stage, from its Label; none for any other Ticket.
export const stageOf = (t: { labels: string[] }): StageName | undefined => STAGES.find((s) => t.labels.includes(`stage:${s}`));

// stageRank orders a Requirement's Tickets: its Stage Tickets first, in
// Stage order, then every other Ticket.
export const stageRank = (t: { labels: string[] }): number => {
  const s = stageOf(t);
  return s ? STAGES.indexOf(s) : STAGES.length;
};

// waitingNote says what a Requirement waiting for a Factory is waiting for.
export function waitingNote(req: RequirementView): string | undefined {
  if (!req.waiting_for_factory) return undefined;
  const current = currentSegment(req);
  const what = current.ticket ? segmentName(current) : "a Build Ticket";
  return `Waiting for a Factory to claim ${what}.${req.needs.length ? "" : " Nothing needs you yet."}`;
}

// headline is a Requirement's one line in its Spec row: what needs the
// human, else what it is waiting for.
export function headline(finished: boolean, req: RequirementView): string {
  const { done, total } = req.build;
  if (finished) return total ? `Finished · ${done} of ${total} Tickets Done` : "Finished";
  const n = req.needs[0];
  if (n) {
    const where = STAGE_TITLE[n.stage ?? "build"];
    return `${where}: ${REASON_LABEL[n.reason]}${req.needs.length > 1 ? ` · +${req.needs.length - 1} more` : ""}`;
  }
  const current = currentSegment(req);
  if (req.waiting_for_factory) return `${segmentName(current)}: waiting for a Factory`;
  if (current.stage === "build") return total ? `Build: ${done} of ${total} Done` : "Build: no Tickets yet";
  return `${segmentName(current)}: ${current.progress === "running" ? "a Factory is on it" : "not started"}`;
}
