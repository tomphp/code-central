// The triage inbox: "New requirement" and "New Map", Projects and Views on
// the left, the list in the middle, the selected Requirement or Map and
// Ticket on the right. A layout only: the data comes
// from live.ts and the Ticket pane is shared with any later view.
import type { MouseEvent } from "react";
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import { FINISHED, STATES, type MapView, type NeedsMeReason, type NeedsMeRow, type ProjectRow, type SpecDetail, type State, type TicketView } from "../api/types";
import { duration, REASON_LABEL, rowDetail } from "../format";
import { RowLatestEvent } from "../LatestEvent";
import { LinkBadge } from "../ticket/LinkBadge";
import { PrototypeBadge } from "../ticket/PrototypeBadge";
import type { InboxData, ProjectSpec, ProjectTicket } from "../live";
import { MapPane } from "../map/MapPane";
import { MapRow } from "../map/MapRow";
import { NewMap, NewRequirement } from "../requirement/NewRequirement";
import { isStage, needsMeCount, progress, reasonsOf } from "../requirement/progress";
import { RequirementPane } from "../requirement/RequirementPane";
import { RequirementRow } from "../requirement/RequirementRow";
import { TicketPane } from "../ticket/TicketDetail";
import { OptionsLink } from "../options/OptionsLink";
import { UsageLink } from "../usage/UsageLink";
import { ArchivedProjects, ProjectFilter } from "./Projects";
import { clickTo, hrefFor, type UiLocation } from "../url";

export interface InboxProps {
  api: Api;
  now: Date;
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  // requirement is the selected Requirement, once read.
  requirement?: SpecDetail & { requirement: NonNullable<SpecDetail["requirement"]> };
  // map is the selected Map, once read; it shares requirement's place.
  map?: SpecDetail & { map: MapView };
  requirementError?: string;
  ticket?: TicketView;
  ticketError?: string;
}

export function Inbox({ api, now, data, loc, navigate, requirement, map, requirementError, ticket, ticketError }: InboxProps) {
  const inProject = (key: string) => !loc.project || key === loc.project;
  const projects = data?.projects ?? [];
  const archived = projects.filter((p) => p.archived);
  const shown = (data?.needsMe.groups ?? []).map((g) => ({ ...g, tickets: g.tickets.filter((r) => inProject(r.project)) }));
  // Stage Tickets are never Needs Me rows: their Requirement's row stands in.
  const stageNeeds = new Set(shown.flatMap((g) => g.tickets).flatMap((r) => (r.requirement?.stage ? [r.requirement.spec] : [])));
  const groups = shown
    .map((g) => ({ ...g, tickets: g.tickets.filter((r) => !r.requirement?.stage) }))
    .filter((g) => g.tickets.length > 0);
  const reasons = reasonsOf(data?.needsMe);
  const tickets = (data?.tickets ?? []).filter((t) => inProject(t.project));
  const requirements = requirementList((data?.requirements ?? []).filter((s) => inProject(s.project)));
  const maps = (data?.maps ?? []).filter((s) => inProject(s.project));
  // The Requirements view lists Maps among the Requirements.
  const listed = requirementList(byCreation([...requirements, ...maps]));
  const progressOf = (s: ProjectSpec) => progress(s.requirement!, (data?.tickets ?? []).filter((t) => t.spec === s.id), reasons);
  const needingReqs = requirements.filter((s) => stageNeeds.has(s.id));
  // In flight: unfinished, and nothing of it waits on the human.
  const inFlight = requirements.filter((s) => !s.finished && !progressOf(s).needs.length);
  const drafts = tickets.filter((t) => t.state === "Draft");
  const shownCount = needsMeCount(data?.needsMe, inProject);
  const totalCount = needsMeCount(data?.needsMe);

  // Selecting a row: a Ticket row leaves any Requirement; a Requirement
  // row opens on its default Ticket.
  const go = (patch: Partial<UiLocation>) => clickTo(navigate, patch);
  const rowLink = (id: string) => ({
    href: hrefFor({ ...loc, requirement: undefined, ticket: id }),
    onClick: go({ requirement: undefined, ticket: id }),
    "data-id": id,
    "aria-current": !loc.requirement && loc.ticket === id ? ("true" as const) : undefined,
  });
  const requirementLink = (id: string) => ({
    href: hrefFor({ ...loc, requirement: id, ticket: undefined }),
    onClick: go({ requirement: id, ticket: undefined }),
    "data-id": id,
    "aria-current": loc.requirement === id ? ("true" as const) : undefined,
  });

  // Only an active Project has a Project Options link: an Archived one takes no writes.
  const projectFilter = (p: ProjectRow) => (
    <ProjectFilter
      key={p.key}
      api={api}
      project={p}
      on={loc.project === p.key}
      onSelect={() => navigate({ project: p.key })}
      projectOptions={p.archived ? undefined : { href: hrefFor({ view: "project-options", project: p.key }), onClick: go({ view: "project-options", project: p.key }) }}
    />
  );

  return (
    <div className="inbox">
      <nav aria-label="Filters" className="inbox-nav">
        <div className="inbox-title">
          <h1>Code Central</h1>
          <UsageLink loc={loc} navigate={navigate} />
          <OptionsLink loc={loc} navigate={navigate} />
        </div>
        <div className="new-specs">
          <NewRequirement
            api={api}
            projects={projects.filter((p) => !p.archived)}
            project={loc.project}
            onSubmitted={(r) => navigate({ list: "requirements", state: undefined, requirement: r.spec.id, ticket: undefined })}
          />
          <NewMap
            api={api}
            projects={projects.filter((p) => !p.archived)}
            project={loc.project}
            onSubmitted={(r) => navigate({ list: "requirements", state: undefined, requirement: r.spec.id, ticket: undefined })}
          />
        </div>
        <button className="btn focus-enter" onClick={() => navigate({ view: "focus" })}>
          Focus
        </button>
        <FilterButton on={!loc.project} label="All Projects" count={totalCount} loud onClick={() => navigate({ project: undefined })} />
        {projects.filter((p) => !p.archived).map(projectFilter)}
        <h2>View</h2>
        <FilterButton on={!loc.state && !loc.list} label="Needs Me" count={shownCount} loud onClick={() => navigate({ state: undefined, list: undefined })} />
        <FilterButton
          on={loc.list === "requirements"}
          label="Requirements"
          count={listed.filter((s) => !s.finished).length}
          onClick={() => navigate({ list: "requirements", state: undefined })}
        />
        {STATES.map((s) => (
          <FilterButton
            key={s}
            sub
            on={loc.state === s}
            label={s}
            count={tickets.filter((t) => t.state === s && !isStage(t)).length}
            onClick={() => navigate({ state: s, list: undefined })}
          />
        ))}
        <ArchivedProjects count={archived.length}>
          {archived.map(projectFilter)}
        </ArchivedProjects>
      </nav>

      <main aria-label="Tickets" className="inbox-list">
        {!data ? (
          <p className="empty">Loading…</p>
        ) : loc.list === "requirements" ? (
          <>
            {listed.length === 0 && <p className="empty">No Requirements yet.</p>}
            {listed.map((s) =>
              s.map ? (
                <MapRow key={s.id} spec={{ ...s, map: s.map }} link={requirementLink(s.id)} />
              ) : (
                <RequirementRow key={s.id} spec={s} p={progressOf(s)} link={requirementLink(s.id)} />
              ),
            )}
          </>
        ) : loc.state ? (
          <StateList tickets={stateList(tickets, loc.state)} reasons={reasons} now={now} link={rowLink} />
        ) : (
          <>
            {shownCount === 0 && <p className="empty">Nothing needs you.</p>}
            {needingReqs.length > 0 && (
              <section aria-label="Requirements" className="reason-requirements">
                <h3 className="group-head">
                  Requirements <span>{needingReqs.length}</span>
                </h3>
                {needingReqs.map((s) => (
                  <RequirementRow key={s.id} spec={s} p={progressOf(s)} link={requirementLink(s.id)} />
                ))}
              </section>
            )}
            {groups.map((g) => (
              <section key={g.reason} aria-label={REASON_LABEL[g.reason]} className={`reason-${g.reason}`}>
                <h3 className="group-head" data-reason={g.reason}>
                  {REASON_LABEL[g.reason]} <span>{g.tickets.length}</span>
                </h3>
                {g.tickets.map((r) => (
                  <NeedsMeLink key={r.id} row={r} now={now} link={rowLink(r.id)} />
                ))}
              </section>
            ))}
            {inFlight.length > 0 && (
              <details className="quiet-group">
                <summary>Requirements in flight ({inFlight.length}): not waiting on you</summary>
                {inFlight.map((s) => (
                  <RequirementRow key={s.id} spec={s} p={progressOf(s)} link={requirementLink(s.id)} />
                ))}
              </details>
            )}
            {drafts.length > 0 && <Drafts drafts={drafts} link={rowLink} />}
          </>
        )}
      </main>

      <aside aria-label="Ticket" className="inbox-detail">
        {loc.requirement &&
          (requirementError ? (
            <p className="empty">{requirementError}</p>
          ) : requirement ? (
            <RequirementPane spec={requirement} reasons={reasons} now={now} ticket={loc.ticket} select={(id) => navigate({ ticket: id })} />
          ) : map ? (
            <MapPane spec={map} reasons={reasons} now={now} ticket={loc.ticket} select={(id) => navigate({ ticket: id })} />
          ) : (
            <p className="empty">Loading…</p>
          ))}
        {ticketError ? (
          <p className="empty">{ticketError}</p>
        ) : ticket && loc.ticket && ticket.id === loc.ticket ? (
          <TicketPane api={api} project={projectOf(ticket.id)} ticket={ticket} now={now} />
        ) : (
          !loc.requirement && <p className="empty">{loc.ticket ? "Loading…" : "Select a Ticket."}</p>
        )}
      </aside>
    </div>
  );
}

// stateList is one State's Tickets: in claim order, as found, except the
// finished States, which read newest first.
function stateList(tickets: ProjectTicket[], state: State): ProjectTicket[] {
  const rows = tickets.filter((t) => t.state === state);
  return FINISHED.includes(state) ? rows.sort((a, b) => b.updated.localeCompare(a.updated)) : rows;
}

// byCreation puts Specs from several lists back in creation order: by
// Project, then ID.
function byCreation(specs: ProjectSpec[]): ProjectSpec[] {
  const n = (s: ProjectSpec) => Number(s.id.slice(s.id.lastIndexOf("-") + 1));
  return [...specs].sort((a, b) => a.project.localeCompare(b.project) || n(a) - n(b));
}

// requirementList is the Requirements view's order: unfinished ones, then
// finished ones, newest first in each.
function requirementList(specs: ProjectSpec[]): ProjectSpec[] {
  const newest = [...specs].reverse();
  return [...newest.filter((s) => !s.finished), ...newest.filter((s) => s.finished)];
}

function FilterButton(p: { on: boolean; label: string; count: number; loud?: boolean; sub?: boolean; onClick(): void }) {
  return (
    <button
      className={["filter", p.on && "on", p.sub && "sub"].filter(Boolean).join(" ")}
      aria-pressed={p.on}
      onClick={p.onClick}
    >
      {p.label} <span className={p.loud && p.count > 0 ? "count loud" : "count"}>{p.count}</span>
    </button>
  );
}

type RowLink = ReturnType<InboxRowLinkFactory>;
type InboxRowLinkFactory = (id: string) => {
  href: string;
  onClick(e: MouseEvent): void;
  "data-id": string;
  "aria-current"?: "true";
};

function NeedsMeLink({ row, now, link }: { row: NeedsMeRow; now: Date; link: RowLink }) {
  return (
    <a className="row" {...link}>
      <span className="row-top">
        <span className="tid">{row.id}</span>
        <span className="row-title">{row.title}</span>
        {row.prototype && <PrototypeBadge />}
        {row.link && <LinkBadge link={row.link} plain />}
        <span className="muted small">{duration(now, row.since)}</span>
      </span>
      <span className="row-detail">
        {row.requirement && <span className="req-chip">↳ {row.requirement.title}</span>}
        {row.spec && <span className="req-chip">↳ {row.spec.map ? `Map: ${row.spec.title}` : row.spec.title}</span>} {rowDetail(now, row)}
      </span>
    </a>
  );
}

// StateList is one State's Tickets, flat, with its Stage Tickets folded
// into a collapsed group after them: a Factory runs those. An In Progress
// row shows its latest Session Event under its title.
function StateList({
  tickets,
  reasons,
  now,
  link,
}: {
  tickets: ProjectTicket[];
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  link: InboxRowLinkFactory;
}) {
  const stages = tickets.filter(isStage);
  const row = (t: ProjectTicket) => (
    <a key={t.id} className="row" {...link(t.id)}>
      <span className="row-top">
        <span className="tid">{t.id}</span>
        <span className="row-title">{t.title}</span>
        {t.link && <LinkBadge link={t.link} plain />}
        {reasons.has(t.id) && <span className="dot needs-you" role="img" aria-label="Needs you" />}
      </span>
      <RowLatestEvent row={t} now={now} className="row-detail" />
    </a>
  );
  return (
    <>
      {tickets.filter((t) => !isStage(t)).map(row)}
      {stages.length > 0 && (
        <details className="quiet-group">
          <summary>Stage Tickets ({stages.length}): run by a Factory</summary>
          {stages.map(row)}
        </details>
      )}
    </>
  );
}

function Drafts({ drafts, link }: { drafts: ProjectTicket[]; link: InboxRowLinkFactory }) {
  return (
    <details className="quiet-group">
      <summary>Drafts ({drafts.length}): not waiting on anyone</summary>
      {drafts.map((t) => (
        <a key={t.id} className="row quiet" {...link(t.id)}>
          <span className="row-top">
            <span className="tid">{t.id}</span>
            <span className="row-title">{t.title}</span>
          </span>
        </a>
      ))}
    </details>
  );
}
