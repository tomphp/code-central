// The Board view: the Tickets in scope as cards in one column per State, in
// workflow order, minus the columns Columns ▾ hides. By Spec splits the
// columns into one lane per Spec, headed by its Spec row. It is read-only: a
// card opens its Ticket in the Inbox's All Tickets view, on the Board's
// Project. It reads only what the Inbox loads, and follows the live feed.
// Back returns to the Inbox it was opened from.
import { useEffect, useRef, useState, type CSSProperties } from "react";
import type { Api } from "../api/api";
import { FINISHED, STATES, type NeedsMeReason, type State } from "../api/types";
import { ProjectPicker } from "../controls/ProjectPicker";
import { ToggleGroup } from "../controls/ToggleGroup";
import { ViewHeader } from "../controls/ViewHeader";
import { usePulse } from "../focus/pulse";
import { kindLabel, newestFirst } from "../inbox/AllTickets";
import { groupBySpec } from "../inbox/groupBySpec";
import { hiddenColumns, storedGroup, storeGroup, storeHidden } from "../inbox/groupPreference";
import { OneProject, ShortId } from "../inbox/ShortId";
import { SpecRow, specKindOf } from "../inbox/SpecRow";
import { stateSlug } from "../inbox/StateRow";
import type { InboxData, ProjectSpec, ProjectTicket } from "../live";
import { reasonsOf } from "../requirement/progress";
import { NewSplit } from "../requirement/NewSplit";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { ColumnsMenu } from "./ColumnsMenu";
import "./board.css";

export interface BoardViewProps {
  api: Api;
  now: Date;
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function BoardView({ api, now, data, loc, navigate }: BoardViewProps) {
  const projects = data?.projects ?? [];
  const hidden = hiddenColumns(loc.hide);
  const shown = STATES.filter((s) => !hidden.includes(s));
  // The grouping in effect, shared with the Inbox's State lists: the
  // address's, else this browser's last pick.
  const grouping = loc.group ?? storedGroup();
  const tickets = newestFirst((data?.tickets ?? []).filter((t) => !loc.project || t.project === loc.project));
  const inColumn = new Map(shown.map((s) => [s, tickets.filter((t) => t.state === s)]));
  const specs = new Map((data?.specs ?? []).map((s) => [s.id, s]));
  const reasons = reasonsOf(data?.needsMe);
  const joined = useJoined(data?.tickets);
  // The lanes the human has collapsed, by Spec; page-only.
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  // A card opens its Ticket in All Tickets on the Board's Project, keeping
  // the grouping, with Show finished on for a finished one.
  const open = (t: ProjectTicket): Partial<UiLocation> => ({
    view: "inbox",
    project: loc.project,
    list: "tickets",
    ticket: t.id,
    group: grouping === "spec" ? "spec" : undefined,
    finished: FINISHED.includes(t.state) || undefined,
  });
  const cardLink = (t: ProjectTicket): CardLink => {
    const to = open(t);
    return { href: hrefFor(to as UiLocation), onClick: clickTo(navigate, to) };
  };
  // A lane's Spec row opens the Spec, as All Tickets' By Spec rows do.
  const specLink = (id: string) => {
    const to: Partial<UiLocation> = { view: "inbox", project: loc.project, list: "tickets", requirement: id, group: "spec" };
    return { href: hrefFor(to as UiLocation), onClick: clickTo(navigate, to), "data-id": id };
  };
  // Every one of a Spec's Tickets, in every State and Project, for its row.
  const ticketsOf = new Map(groupBySpec(data?.tickets ?? []).map((g) => [g.id, g.tickets]));
  const toggleLane = (id: string) =>
    setCollapsed((c) => {
      const next = new Set(c);
      if (!next.delete(id)) next.add(id);
      return next;
    });

  return (
    <OneProject.Provider value={!!loc.project}>
      <main aria-label="Board" className="board">
        <ViewHeader
          title="Board"
          left={<ProjectPicker projects={projects} value={loc.project} onChange={(project) => navigate({ project })} />}
          right={
            <>
              <ToggleGroup
                label="Group the board"
                options={[
                  { value: "ticket", label: "Ticket" },
                  { value: "spec", label: "Spec" },
                ]}
                value={grouping}
                onChange={(g) => {
                  storeGroup(g);
                  navigate({ group: g === "spec" ? "spec" : undefined });
                }}
              />
              <ColumnsMenu
                hidden={hidden}
                onChange={(hide) => {
                  storeHidden(hide);
                  navigate({ hide });
                }}
              />
              <span className="board-sep" />
              <div className="board-new">
                <NewSplit
                  variant="plain"
                  api={api}
                  projects={projects.filter((p) => !p.archived)}
                  project={loc.project}
                  onSubmitted={(r) =>
                    navigate(
                      r.kind === "Small Change"
                        ? { view: "inbox", project: loc.project, ticket: r.ticket.id }
                        : { view: "inbox", project: loc.project, list: "requirements", requirement: r.spec.id },
                    )
                  }
                />
              </div>
              <button className="btn" onClick={() => navigate(loc.back ?? { view: "inbox" })}>
                Back to the Inbox
              </button>
            </>
          }
        />
        {!data ? (
          <p className="muted board-loading">Loading…</p>
        ) : (
          <div className="board-scroll">
            <div className="board-grid">
              <div className="board-heads" style={{ "--cols": shown.length } as CSSProperties}>
                {shown.map((s) => (
                  <h2 key={s} className={`board-col-head st-${stateSlug(s)}`}>
                    <span className="board-col-name">{s}</span> <span className="board-col-count">{inColumn.get(s)!.length}</span>
                  </h2>
                ))}
              </div>
              {grouping === "spec" ? (
                lanes(shown, inColumn).map((l) => (
                  <div key={l.id} role="group" aria-label={`Lane: ${specs.get(l.id)?.title ?? l.id}`} className="board-lane">
                    <div className="board-lane-head">
                      <SpecRow
                        id={l.id}
                        spec={specs.get(l.id)}
                        all={ticketsOf.get(l.id) ?? []}
                        reasons={reasons}
                        now={now}
                        link={specLink(l.id)}
                        lane={{ collapsed: collapsed.has(l.id), toggle: () => toggleLane(l.id) }}
                      />
                    </div>
                    <Cells shown={shown} tickets={l.inColumn} counts={collapsed.has(l.id)} specs={specs} reasons={reasons} joined={joined} link={cardLink} inLane />
                  </div>
                ))
              ) : (
                <Cells shown={shown} tickets={inColumn} specs={specs} reasons={reasons} joined={joined} link={cardLink} />
              )}
            </div>
          </div>
        )}
      </main>
    </OneProject.Provider>
  );
}

// useJoined tells whether a card mounting now marks a change: its Ticket
// changed, or is new, since the Board last rendered its Tickets. A card that
// moves column mounts again, so it pulses then; one in a column just shown,
// or on the Board's first render, doesn't.
function useJoined(tickets?: ProjectTicket[]): (t: ProjectTicket) => boolean {
  const seen = useRef<Map<string, string>>(undefined);
  const before = seen.current;
  useEffect(() => {
    if (tickets) seen.current = new Map(tickets.map((t) => [t.id, t.updated]));
  }, [tickets]);
  return (t) => !!before && before.get(t.id) !== t.updated;
}

type CardLink = { href: string; onClick: ReturnType<typeof clickTo> };

interface Lane {
  id: string;
  inColumn: Map<State, ProjectTicket[]>;
}

// lanes splits the shown columns' Tickets (each column newest first) into
// one lane per Spec, newest Spec first: by Project, then ID, descending.
function lanes(shown: State[], inColumn: Map<State, ProjectTicket[]>): Lane[] {
  const n = (id: string) => Number(id.slice(id.lastIndexOf("-") + 1));
  const ofSpec = groupBySpec(shown.flatMap((s) => inColumn.get(s)!));
  return ofSpec
    .sort((a, b) => b.tickets[0].project.localeCompare(a.tickets[0].project) || n(b.id) - n(a.id))
    .map((g) => ({ id: g.id, inColumn: new Map(shown.map((s) => [s, g.tickets.filter((t) => t.state === s)])) }));
}

// Cells is one row of the grid, a cell per shown column. In a lane that is
// collapsed (counts) a cell is just its number of Tickets; in a lane an
// empty cell is blank and its cards leave off the Spec line.
function Cells({
  shown,
  tickets,
  counts,
  specs,
  reasons,
  joined,
  link,
  inLane,
}: {
  shown: State[];
  tickets: Map<State, ProjectTicket[]>;
  counts?: boolean;
  specs: Map<string, ProjectSpec>;
  reasons: Map<string, NeedsMeReason>;
  joined(t: ProjectTicket): boolean;
  link(t: ProjectTicket): CardLink;
  inLane?: boolean;
}) {
  return (
    <div className={`board-cells${counts ? " board-counts" : ""}`} style={{ "--cols": shown.length } as CSSProperties}>
      {shown.map((s) => {
        const here = tickets.get(s)!;
        return (
          <section key={s} aria-label={s} className="board-cell">
            {counts ? (
              <span className="board-count">{here.length}</span>
            ) : here.length === 0 ? (
              !inLane && <p className="board-empty">No Tickets</p>
            ) : (
              here.map((t) => (
                <Card key={t.id} t={t} spec={inLane ? undefined : specs.get(t.spec)} reason={reasons.get(t.id)} joined={joined(t)} link={link(t)} />
              ))
            )}
          </section>
        );
      })}
    </div>
  );
}

// Card is a Ticket on the Board: its short ID, kind, Blocked and needs-you
// dot on top, its title, then its Spec's kind and name (spec is left out
// inside a lane). It pulses when its Ticket's last update changes.
function Card({ t, spec, reason, joined, link }: { t: ProjectTicket; spec?: ProjectSpec; reason?: NeedsMeReason; joined: boolean; link: CardLink }) {
  const pulse = usePulse(reason ? `board-card needs reason-${reason}` : "board-card", t.updated, joined, "board");
  const label = kindLabel(t);
  const kind = specKindOf(spec);
  return (
    <a {...pulse} {...link}>
      <span className="board-card-top">
        <ShortId id={t.id} />
        {label && <span className="stage-badge">{label}</span>}
        <span className="board-spacer" />
        {t.blocked && (
          <span className="board-blocked" title="Has open Blockers">
            ⛔ Blocked
          </span>
        )}
        {reason && <span className="dot" role="img" aria-label="Needs you" />}
      </span>
      <span className="board-card-title">{t.title}</span>
      {spec && (
        <span className="board-card-spec">
          <span className={`kind-badge kind-${stateSlug(kind)}`}>{kind}</span>{" "}
          {spec.title}
        </span>
      )}
    </a>
  );
}
