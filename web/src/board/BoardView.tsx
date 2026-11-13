// The Board view: the Tickets in scope as cards in one column per State, in
// workflow order, minus the columns Columns ▾ hides. By Spec splits the
// columns into one lane per Spec, headed by its Lane header. It takes no
// action itself: a card opens its Ticket's popup and a Lane header its
// Spec's, over the Board, kept in the address as popup=, and only the popup
// acts. A card opens its Focus popup while the Ticket needs the human and its
// Ticket popup otherwise. It reads only what the Inbox loads, and follows the
// live feed. Back returns to the Inbox it was opened from.
import { useEffect, useRef, useState, type CSSProperties } from "react";
import type { Api } from "../api/api";
import { STATES, type NeedsMeReason, type State } from "../api/types";
import { ProjectPicker } from "../controls/ProjectPicker";
import { ToggleGroup } from "../controls/ToggleGroup";
import { ViewHeader } from "../controls/ViewHeader";
import { kindLabel, newestFirst } from "../inbox/AllTickets";
import { groupBySpec } from "../inbox/groupBySpec";
import { hiddenColumns, storedGroup, storeGroup, storeHidden } from "../inbox/groupPreference";
import { OneProject } from "../inbox/ShortId";
import { specKindOf } from "../inbox/describeSpec";
import { stateSlug } from "../controls/StateChip";
import type { InboxData, ProjectSpec, ProjectTicket } from "../live";
import { reasonsOf } from "../requirement/progress";
import { NewSplit } from "../requirement/NewSplit";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { TicketCard } from "../ticket/TicketCard";
import { ColumnsMenu } from "./ColumnsMenu";
import { LaneHeader } from "./LaneHeader";
import { FocusPopup, type FocusPopupClose } from "./FocusPopup";
import { usePopupKind } from "./popupKind";
import { SpecPopup } from "./SpecPopup";
import { TicketPopup } from "./TicketPopup";
import { Button } from "../controls/Button";
import "./board.css";

export interface BoardViewProps {
  api: Api;
  now: Date;
  // version is the live feed's, which the popups read on.
  version: number;
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  // onFocusPopupClose hears a card's Focus popup close, for the automatic
  // Focus popup not to reopen for its Focus Items.
  onFocusPopupClose?(closed: FocusPopupClose): void;
}

export function BoardView({ api, now, version, data, loc, navigate, onFocusPopupClose }: BoardViewProps) {
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

  // A card opens its Ticket's popup, a Lane header its Spec's: the Board's
  // address with popup=.
  const popupLink = (id: string) => ({
    href: hrefFor({ ...loc, popup: id }),
    onClick: clickTo(navigate, { popup: id }),
  });
  const cardLink = (t: ProjectTicket): CardLink => popupLink(t.id);
  const kind = usePopupKind(loc.popup, data);
  const closePopup = () => navigate({ popup: undefined });
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
              <Button  onClick={() => navigate(loc.back ?? { view: "inbox" })}>
                Back to the Inbox
              </Button>
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
                  <div
                    key={l.id}
                    role="group"
                    aria-label={`Lane: ${specs.get(l.id)?.title ?? l.id}`}
                    className={`board-lane${collapsed.has(l.id) ? " collapsed" : ""}`}
                  >
                    <LaneHeader
                      id={l.id}
                      spec={specs.get(l.id)}
                      all={ticketsOf.get(l.id) ?? []}
                      shown={l.count}
                      reasons={reasons}
                      link={{ ...popupLink(l.id), "data-id": l.id }}
                      collapsed={collapsed.has(l.id)}
                      onToggle={() => toggleLane(l.id)}
                    />
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
      {/* At most one popup, chosen from popup=. */}
      {loc.popup && kind === "focus" && data && (
        <FocusPopup
          key={loc.popup}
          mode="ticket"
          api={api}
          now={now}
          version={version}
          data={data}
          id={loc.popup}
          onClose={(closed) => {
            onFocusPopupClose?.(closed);
            closePopup();
          }}
          navigate={navigate}
        />
      )}
      {loc.popup && kind === "spec" && (
        <SpecPopup
          api={api}
          now={now}
          version={version}
          id={loc.popup}
          projects={projects}
          reasons={reasons}
          onClose={closePopup}
          open={(id) => navigate({ popup: id })}
          specLink={popupLink}
        />
      )}
      {loc.popup && kind === "ticket" && (
        <TicketPopup
          api={api}
          now={now}
          version={version}
          id={loc.popup}
          projects={projects}
          onClose={closePopup}
          openSpec={(id) => navigate({ popup: id })}
        />
      )}
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
  // count is how many Tickets the lane shows, across its columns.
  count: number;
}

// lanes splits the shown columns' Tickets (each column newest first) into
// one lane per Spec, newest Spec first: by Project, then ID, descending.
function lanes(shown: State[], inColumn: Map<State, ProjectTicket[]>): Lane[] {
  const n = (id: string) => Number(id.slice(id.lastIndexOf("-") + 1));
  const ofSpec = groupBySpec(shown.flatMap((s) => inColumn.get(s)!));
  return ofSpec
    .sort((a, b) => b.tickets[0].project.localeCompare(a.tickets[0].project) || n(b.id) - n(a.id))
    .map((g) => ({ id: g.id, inColumn: new Map(shown.map((s) => [s, g.tickets.filter((t) => t.state === s)])), count: g.tickets.length }));
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
              here.map((t) => {
                const spec = inLane ? undefined : specs.get(t.spec);
                return (
                  <TicketCard
                    key={t.id}
                    t={t}
                    link={link(t)}
                    badge={kindLabel(t)}
                    reason={reasons.get(t.id)}
                    spec={spec && { kind: specKindOf(spec), title: spec.title }}
                    joined={joined(t)}
                  />
                );
              })
            )}
          </section>
        );
      })}
    </div>
  );
}
