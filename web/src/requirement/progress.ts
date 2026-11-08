// A Requirement's progress as the UI shows it: one segment per Stage and one
// for Build, worked out from the service's requirement block, the
// Requirement's implementation Tickets and what Needs Me says needs the human.
import { FINISHED, type NeedsMeReason, type NeedsMeView, type RequirementView, type StageName, type TicketRow } from "../api/types";

export const STAGE_TITLE: Record<StageName | "build", string> = {
  grilling: "Grilling",
  spec: "Spec",
  tickets: "Tickets",
  build: "Build",
};

// done: finished; needs: waiting on the human; queued: claimable, waiting
// for a Factory (drawn dashed); running: a Factory is on it; waiting: not
// started, behind an earlier segment.
export type SegmentProgress = "done" | "needs" | "queued" | "running" | "waiting";

export interface Segment {
  stage: StageName | "build";
  name: string;
  progress: SegmentProgress;
  // ticket is the Stage Ticket; Build has none of its own.
  ticket?: string;
  reason?: NeedsMeReason;
}

export interface Progress {
  segments: Segment[]; // grilling, spec, tickets, build
  // current is the first segment not done, or Build once all are.
  current: Segment;
  // build is the implementation Tickets, Cancelled ones included.
  build: TicketRow[];
  done: number;
  total: number;
  waitingForFactory: boolean;
  // needs is every Ticket of the Requirement waiting on the human, Stages first.
  needs: { ticket: string; reason: NeedsMeReason; stage?: StageName }[];
}

// reasonsOf maps each Ticket in Needs Me to its reason.
export function reasonsOf(needsMe?: NeedsMeView): Map<string, NeedsMeReason> {
  return new Map((needsMe?.groups ?? []).flatMap((g) => g.tickets.map((r) => [r.id, g.reason] as const)));
}

// needsMeCount is the Needs Me count as the UI shows it, over the rows
// inProject keeps: each Ticket row, with a Requirement's Stage Tickets
// counted once, as its Requirement's row, never as themselves.
export function needsMeCount(needsMe: NeedsMeView | undefined, inProject: (project: string) => boolean = () => true): number {
  const rows = (needsMe?.groups ?? []).flatMap((g) => g.tickets).filter((r) => inProject(r.project));
  const requirements = new Set(rows.flatMap((r) => (r.requirement?.stage ? [r.requirement.spec] : [])));
  return requirements.size + rows.filter((r) => !r.requirement?.stage).length;
}

export const isStage =(t: { labels: string[] }) => t.labels.some((l) => l.startsWith("stage:"));

export function progress(req: RequirementView, tickets: TicketRow[], reasons: Map<string, NeedsMeReason>): Progress {
  const build = tickets.filter((t) => !isStage(t));
  const firstOpen = req.stages.findIndex((s) => s.state !== "Done");
  const segments: Segment[] = req.stages.map((s, i) => {
    const reason = s.needs_me_reason ?? reasons.get(s.ticket);
    const segProgress: SegmentProgress =
      s.state === "Done"
        ? "done"
        : reason
          ? "needs"
          : i === firstOpen && req.waiting_for_factory
            ? "queued"
            : s.state === "Ready" || s.state === "Draft"
              ? "waiting"
              : "running";
    return { stage: s.name, name: STAGE_TITLE[s.name], progress: segProgress, ticket: s.ticket, ...(reason && { reason }) };
  });
  const buildNeeds = build.filter((t) => reasons.has(t.id));
  const inBuild = firstOpen < 0;
  const buildProgress: SegmentProgress =
    req.build.total > 0 && req.build.done === req.build.total && !buildNeeds.length
      ? "done"
      : buildNeeds.length
        ? "needs"
        : inBuild && req.waiting_for_factory
          ? "queued"
          : inBuild && req.build.total > 0
            ? "running"
            : "waiting";
  const buildSeg: Segment = { stage: "build", name: STAGE_TITLE.build, progress: buildProgress, ...(buildNeeds[0] && { reason: reasons.get(buildNeeds[0].id) }) };
  segments.push(buildSeg);
  const needs = [
    ...segments.filter((s) => s.ticket && s.reason).map((s) => ({ ticket: s.ticket!, reason: s.reason!, stage: s.stage as StageName })),
    ...buildNeeds.map((t) => ({ ticket: t.id, reason: reasons.get(t.id)! })),
  ];
  return {
    segments,
    current: segments.find((s) => s.progress !== "done") ?? buildSeg,
    build,
    done: req.build.done,
    total: req.build.total,
    waitingForFactory: req.waiting_for_factory,
    needs,
  };
}

// defaultTicket is the Ticket a Requirement opens on: the one that needs
// the human, else the current Stage's, else the first unfinished Build
// Ticket, else the last Build Ticket or Stage.
export function defaultTicket(p: Progress): string | undefined {
  if (p.needs[0]) return p.needs[0].ticket;
  if (p.current.ticket) return p.current.ticket;
  const open = p.build.find((t) => !FINISHED.includes(t.state));
  return open?.id ?? p.build.at(-1)?.id ?? [...p.segments].reverse().find((s) => s.ticket)?.ticket;
}

// waitingNote says what a Requirement waiting for a Factory is waiting for.
export function waitingNote(p: Progress): string | undefined {
  if (!p.waitingForFactory) return undefined;
  const what = p.current.ticket ? p.current.name : "a Build Ticket";
  return `Waiting for a Factory to claim ${what}.${p.needs.length ? "" : " Nothing needs you yet."}`;
}
