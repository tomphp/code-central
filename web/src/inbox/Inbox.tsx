// The triage inbox: on the left, a top row of the split "+ New" button
// (New requirement, with Requirement and Map in its ▾ menu) and "◎ Focus",
// then Projects and Views, then an Admin section linking Usage, CI and
// Options; the list in the middle; the selected Requirement, Map or plain
// Spec and Ticket on the right. A layout only: the data comes from live.ts and the
// Ticket pane is shared with any later view.
import { useState, type MouseEvent } from "react";
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import {
  FINISHED,
  STATES,
  type MapView,
  type NeedsMeReason,
  type NeedsMeRow,
  type ProjectRow,
  type SpecDetail,
  type State,
  type TicketView,
} from "../api/types";
import { duration, REASON_LABEL, rowDetail, ticketOf } from "../format";
import { CIBadge } from "../ticket/CIBadge";
import { PullRequestBadge } from "../ticket/PullRequestBadge";
import { LinkBadge } from "../ticket/LinkBadge";
import { PrototypeBadge } from "../ticket/PrototypeBadge";
import type { InboxData, ProjectSpec, ProjectTicket } from "../live";
import { MapPane } from "../map/MapPane";
import {
  isStage,
  needsMeCount,
  progress,
  reasonsOf,
} from "../requirement/progress";
import { RequirementPane } from "../requirement/RequirementPane";
import { SpecPane } from "../spec/SpecPane";
import { TicketPane } from "../ticket/TicketDetail";
import { OptionsLink } from "../options/OptionsLink";
import { UsageLink } from "../usage/UsageLink";
import { CILink } from "../ci/CILink";
import { SidebarTop } from "./SidebarTop";
import { ArchivedProjects, ProjectFilter } from "./Projects";
import { AllTicketsBySpec, AllTicketsByTicket, matchesId } from "./AllTickets";
import { groupBySpec } from "./groupBySpec";
import { SpecRow } from "./SpecRow";
import { StateListBySpec } from "./StateListBySpec";
import { StateRow, type InboxRowLinkFactory } from "./StateRow";
import { storedFinished, storedGroup, storeFinished, storeGroup } from "./groupPreference";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { OneProject, ShortId } from "./ShortId";

export interface InboxProps {
  api: Api;
  now: Date;
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  // requirement is the selected Requirement, once read.
  requirement?: SpecDetail & {
    requirement: NonNullable<SpecDetail["requirement"]>;
  };
  // map is the selected Map, once read; it shares requirement's place.
  map?: SpecDetail & { map: MapView };
  // spec is the selected plain Spec, neither a Requirement nor a Map,
  // once read; it shares requirement's place too.
  spec?: SpecDetail;
  requirementError?: string;
  ticket?: TicketView;
  ticketError?: string;
}

export function Inbox({
  api,
  now,
  data,
  loc,
  navigate,
  requirement,
  map,
  spec,
  requirementError,
  ticket,
  ticketError,
}: InboxProps) {
  const inProject = (key: string) => !loc.project || key === loc.project;
  const projects = data?.projects ?? [];
  const archived = projects.filter((p) => p.archived);
  const isArchived = (id: string) =>
    archived.some((p) => p.key === projectOf(id));
  const shown = (data?.needsMe.groups ?? []).map((g) => ({
    ...g,
    tickets: g.tickets.filter((r) => inProject(r.project)),
  }));
  // Stage Tickets are never Needs Me rows: their Requirement's row stands in.
  const stageNeeds = new Set(
    shown
      .flatMap((g) => g.tickets)
      .flatMap((r) => (r.requirement?.stage ? [r.requirement.spec] : [])),
  );
  const groups = shown
    .map((g) => ({
      ...g,
      tickets: g.tickets.filter((r) => !r.requirement?.stage),
    }))
    .filter((g) => g.tickets.length > 0);
  const reasons = reasonsOf(data?.needsMe);
  const tickets = (data?.tickets ?? []).filter((t) => inProject(t.project));
  // Every Spec's Tickets, in every State and Project, for its Spec row.
  const bySpec = new Map(groupBySpec(data?.tickets ?? []).map((g) => [g.id, g.tickets]));
  const ticketsOf = (spec: string) => bySpec.get(spec) ?? [];
  const requirements = requirementList(
    (data?.requirements ?? []).filter((s) => inProject(s.project)),
  );
  const maps = (data?.maps ?? []).filter((s) => inProject(s.project));
  // The Requirements view lists Maps among the Requirements.
  const listed = requirementList(byCreation([...requirements, ...maps]));
  const progressOf = (s: ProjectSpec) =>
    progress(s.requirement!, ticketsOf(s.id), reasons);
  const needingReqs = requirements.filter((s) => stageNeeds.has(s.id));
  // In flight: unfinished, and nothing of it waits on the human.
  const inFlight = requirements.filter(
    (s) => !s.finished && !progressOf(s).needs.length,
  );
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
    "aria-current":
      !loc.requirement && loc.ticket === id ? ("true" as const) : undefined,
  });
  // specLink opens a Spec, a Requirement or Map, on its default Ticket.
  const specLink = (id: string) => ({
    href: hrefFor({ ...loc, requirement: id, ticket: undefined }),
    onClick: go({ requirement: id, ticket: undefined }),
  });
  const requirementLink = (id: string) => ({
    ...specLink(id),
    "data-id": id,
    "aria-current": loc.requirement === id ? ("true" as const) : undefined,
  });
  // specRow is a Spec's row in the Requirements view and Needs Me, opening
  // on all of its Tickets.
  const specRow = (s: ProjectSpec, { quietWhenIdle = false } = {}) => (
    <SpecRow
      key={s.id}
      id={s.id}
      spec={s}
      all={ticketsOf(s.id)}
      reasons={reasons}
      now={now}
      link={requirementLink(s.id)}
      ticketLink={rowLink}
      selectedTicket={loc.ticket}
      quietWhenIdle={quietWhenIdle}
    />
  );

  const showFinished = loc.finished ?? storedFinished();
  const chooseFinished = (on: boolean) => {
    storeFinished(on);
    navigate({ finished: on ? true : undefined });
  };
  // The ID search is page-only. While it is not empty it overrides Show
  // finished; By Spec also takes every Ticket of a Spec whose own ID matches.
  const [search, setSearch] = useState("");
  const query = search.trim();
  const listedTickets = query
    ? tickets.filter((t) => matchesId(t.id, query))
    : showFinished
      ? tickets
      : tickets.filter((t) => !FINISHED.includes(t.state));
  const specSearchTickets = tickets.filter((t) => matchesId(t.id, query) || matchesId(t.spec, query));
  // The grouping in effect: the URL's, else this browser's last pick.
  const grouping = loc.group ?? storedGroup();
  const chooseGroup = (g: "spec" | "ticket") => {
    storeGroup(g);
    navigate({ group: g === "spec" ? "spec" : undefined });
  };
  const toggleBar = (
  <div className="bs-toggle-bar">
    <div
      className="bs-toggle"
      role="group"
      aria-label="Group the list"
    >
      {(["ticket", "spec"] as const).map((g) => (
        <button
          key={g}
          aria-pressed={grouping === g}
          onClick={() => chooseGroup(g)}
        >
          {g === "spec" ? "By Spec" : "By Ticket"}
        </button>
      ))}
    </div>
    {loc.list === "tickets" && (
      <label className="bs-finished">
        <input
          type="checkbox"
          checked={showFinished}
          onChange={(e) => chooseFinished(e.target.checked)}
        />{" "}
        Show finished
      </label>
    )}
    {loc.list === "tickets" && (
      <input
        type="search"
        className="bs-search"
        aria-label="Ticket ID"
        placeholder="Ticket ID, e.g. 452"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
    )}
  </div>
  );

  // Only an active Project has a Project Options link: an Archived one takes no writes.
  const projectFilter = (p: ProjectRow) => (
    <ProjectFilter
      key={p.key}
      api={api}
      project={p}
      now={now}
      on={loc.project === p.key}
      onSelect={() => navigate({ project: p.key })}
      projectOptions={
        p.archived
          ? undefined
          : {
              href: hrefFor({ view: "project-options", project: p.key }),
              onClick: go({ view: "project-options", project: p.key }),
            }
      }
    />
  );

  return (
    <OneProject.Provider value={!!loc.project}>
    <div className="inbox">
      <nav aria-label="Filters" className="inbox-nav">
        <div className="inbox-title">
          <h1>Code Central</h1>
        </div>
        <SidebarTop
          newProps={{
            api,
            projects: projects.filter((p) => !p.archived),
            project: loc.project,
            onSubmitted: (r) =>
              navigate({
                list: "requirements",
                state: undefined,
                requirement: r.spec.id,
                ticket: undefined,
              }),
          }}
          onFocusMode={() => navigate({ view: "focus" })}
        />
        <FilterButton
          on={!loc.project}
          label="All Projects"
          count={totalCount}
          loud
          onClick={() => navigate({ project: undefined })}
        />
        {projects.filter((p) => !p.archived).map(projectFilter)}
        <h2>View</h2>
        <FilterButton
          on={!loc.state && !loc.list}
          label="Needs Me"
          count={shownCount}
          loud
          onClick={() => navigate({ state: undefined, list: undefined })}
        />
        <FilterButton
          on={loc.list === "requirements"}
          label="Requirements"
          count={listed.filter((s) => !s.finished).length}
          onClick={() => navigate({ list: "requirements", state: undefined })}
        />
        <FilterButton
          on={loc.list === "tickets"}
          label="All Tickets"
          count={tickets.filter((t) => !FINISHED.includes(t.state)).length}
          onClick={() => navigate({ list: "tickets", state: undefined })}
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
        <h2>Admin</h2>
        <UsageLink loc={loc} navigate={navigate} className="filter" />
        <CILink loc={loc} navigate={navigate} className="filter" />
        <OptionsLink loc={loc} navigate={navigate} className="filter" />
        <ArchivedProjects count={archived.length}>
          {archived.map(projectFilter)}
        </ArchivedProjects>
      </nav>

      <main aria-label="Tickets" className="inbox-list">
        {!data ? (
          <p className="empty">Loading…</p>
        ) : loc.list === "requirements" ? (
          <>
            {listed.length === 0 && (
              <p className="empty">No Requirements yet.</p>
            )}
            <div className="spec-rows">
              {listed.map((s) => specRow(s, { quietWhenIdle: true }))}
            </div>
          </>
        ) : loc.list === "tickets" ? (
          <>
            {toggleBar}
            {grouping === "spec" ? (
              <AllTicketsBySpec
                tickets={query ? specSearchTickets : listedTickets}
                search={query}
                showFinished={showFinished}
                specs={new Map((data.specs ?? []).map((s) => [s.id, s]))}
                ticketsOf={ticketsOf}
                reasons={reasons}
                now={now}
                link={rowLink}
                specLink={requirementLink}
                selectedTicket={loc.ticket}
              />
            ) : (
              <AllTicketsByTicket
                tickets={listedTickets}
                search={query}
                showFinished={showFinished}
                specs={new Map((data.specs ?? []).map((s) => [s.id, s]))}
                reasons={reasons}
                link={rowLink}
              />
            )}
          </>
        ) : loc.state ? (
          <>
            {toggleBar}
            {grouping === "spec" ? (
              <StateListBySpec
                key={loc.state}
                state={loc.state}
                tickets={stateList(tickets, loc.state)}
                specs={new Map((data.specs ?? []).map((s) => [s.id, s]))}
                ticketsOf={ticketsOf}
                reasons={reasons}
                now={now}
                link={rowLink}
                specLink={requirementLink}
                selectedTicket={loc.ticket}
              />
            ) : (
              <StateList
                tickets={stateList(tickets, loc.state)}
                reasons={reasons}
                now={now}
                link={rowLink}
              />
            )}
          </>
        ) : (
          <>
            {shownCount === 0 && <p className="empty">Nothing needs you.</p>}
            {needingReqs.length > 0 && (
              <section
                aria-label="Requirements"
                className="reason-requirements"
              >
                <h3 className="group-head">
                  Requirements <span>{needingReqs.length}</span>
                </h3>
                {needingReqs.map((s) => specRow(s))}
              </section>
            )}
            {groups.map((g) => (
              <section
                key={g.reason}
                aria-label={REASON_LABEL[g.reason]}
                className={`reason-${g.reason}`}
              >
                <h3 className="group-head" data-reason={g.reason}>
                  {REASON_LABEL[g.reason]} <span>{g.tickets.length}</span>
                </h3>
                {g.tickets.map((r) => (
                  <NeedsMeLink
                    key={r.id || `${r.project} ${r.reason} ${r.ci_item?.commit ?? ""}`}
                    row={r}
                    now={now}
                    link={rowLink(ticketOf(r))}
                  />
                ))}
              </section>
            ))}
            {inFlight.length > 0 && (
              <details className="quiet-group">
                <summary>
                  Requirements in flight ({inFlight.length}): not waiting on you
                </summary>
                {inFlight.map((s) => specRow(s))}
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
            <RequirementPane
              api={api}
              spec={requirement}
              archived={isArchived(requirement.id)}
              reasons={reasons}
              now={now}
              ticket={loc.ticket}
              select={(id) => navigate({ ticket: id })}
              specLink={specLink}
            />
          ) : map ? (
            <MapPane
              api={api}
              spec={map}
              reasons={reasons}
              now={now}
              ticket={loc.ticket}
              select={(id) => navigate({ ticket: id })}
              specLink={specLink}
              written={(id) =>
                navigate({
                  list: "requirements",
                  state: undefined,
                  requirement: id,
                  ticket: undefined,
                })
              }
            />
          ) : spec ? (
            <SpecPane
              api={api}
              spec={spec}
              archived={isArchived(spec.id)}
              reasons={reasons}
              now={now}
              ticket={loc.ticket}
              select={(id) => navigate({ ticket: id })}
            />
          ) : (
            <p className="empty">Loading…</p>
          ))}
        {ticketError ? (
          <p className="empty">{ticketError}</p>
        ) : ticket && loc.ticket && ticket.id === loc.ticket ? (
          <TicketPane
            api={api}
            project={projectOf(ticket.id)}
            ticket={ticket}
            now={now}
            archived={isArchived(ticket.id)}
            reported={
              projects.find((p) => p.key === projectOf(ticket.id))
                ?.reported_models
            }
            openSpec={(id) => navigate({ requirement: id })}
          />
        ) : (
          !loc.requirement && (
            <p className="empty">
              {loc.ticket ? "Loading…" : "Select a Ticket."}
            </p>
          )
        )}
      </aside>
    </div>
    </OneProject.Provider>
  );
}

// stateList is one State's Tickets: in claim order, as found, except the
// finished States, which read newest first.
function stateList(tickets: ProjectTicket[], state: State): ProjectTicket[] {
  const rows = tickets.filter((t) => t.state === state);
  return FINISHED.includes(state)
    ? rows.sort((a, b) => b.updated.localeCompare(a.updated))
    : rows;
}

// byCreation puts Specs from several lists back in creation order: by
// Project, then ID.
function byCreation(specs: ProjectSpec[]): ProjectSpec[] {
  const n = (s: ProjectSpec) => Number(s.id.slice(s.id.lastIndexOf("-") + 1));
  return [...specs].sort(
    (a, b) => a.project.localeCompare(b.project) || n(a) - n(b),
  );
}

// requirementList is the Requirements view's order: unfinished ones, then
// finished ones, newest first in each.
function requirementList(specs: ProjectSpec[]): ProjectSpec[] {
  const newest = [...specs].reverse();
  return [
    ...newest.filter((s) => !s.finished),
    ...newest.filter((s) => s.finished),
  ];
}

function FilterButton(p: {
  on: boolean;
  label: string;
  count: number;
  loud?: boolean;
  sub?: boolean;
  onClick(): void;
}) {
  return (
    <button
      className={["filter", p.on && "on", p.sub && "sub"]
        .filter(Boolean)
        .join(" ")}
      aria-pressed={p.on}
      onClick={p.onClick}
    >
      {p.label}{" "}
      <span className={p.loud && p.count > 0 ? "count loud" : "count"}>
        {p.count}
      </span>
    </button>
  );
}

type RowLink = ReturnType<InboxRowLinkFactory>;

function NeedsMeLink({
  row,
  now,
  link,
}: {
  row: NeedsMeRow;
  now: Date;
  link: RowLink;
}) {
  // A Project's Base diverged has no Ticket to open. A CI item has none
  // of its own either: it opens on the first Ticket it lists, whose panel
  // shows the item and its options (code-central-432).
  if (!row.id) {
    const top = (
      <span className="row-top">
        <span className="tid">{row.project}</span>
        <span className="row-title">{row.title}</span>
        {row.ci_item && <CIBadge result={row.ci_result} />}
        <span className="muted small">{duration(now, row.since)}</span>
      </span>
    );
    if (row.ci_item) {
      return (
        <a className="row" {...link}>
          {top}
          <span className="row-detail">{rowDetail(now, row)}</span>
        </a>
      );
    }
    return (
      <div className="row">
        {top}
        <span className="row-detail">{rowDetail(now, row)}</span>
      </div>
    );
  }
  return (
    <a className="row" {...link}>
      <span className="row-top">
        <ShortId id={row.id} />
        <span className="row-title">{row.title}</span>
        {row.prototype && <PrototypeBadge />}
        {row.link && <LinkBadge link={row.link} plain />}
        <CIBadge result={row.ci_result} />
        <PullRequestBadge pr={row.pull_request} inRowLink />
        <span className="muted small">{duration(now, row.since)}</span>
      </span>
      <span className="row-detail">
        {row.requirement && (
          <span className="req-chip">↳ {row.requirement.title}</span>
        )}
        {row.spec && (
          <span className="req-chip">
            ↳ {row.spec.map ? `Map: ${row.spec.title}` : row.spec.title}
          </span>
        )}{" "}
        {rowDetail(now, row)}
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
    <StateRow
      key={t.id}
      t={t}
      needsYou={reasons.has(t.id)}
      now={now}
      link={link}
    />
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

function Drafts({
  drafts,
  link,
}: {
  drafts: ProjectTicket[];
  link: InboxRowLinkFactory;
}) {
  return (
    <details className="quiet-group">
      <summary>Drafts ({drafts.length}): not waiting on anyone</summary>
      {drafts.map((t) => (
        <a key={t.id} className="row quiet" {...link(t.id)}>
          <span className="row-top">
            <ShortId id={t.id} />
            <span className="row-title">{t.title}</span>
          </span>
        </a>
      ))}
    </details>
  );
}
