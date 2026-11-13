// The Board view: the Tickets in scope as cards in one column per State, in
// workflow order, minus the columns Columns ▾ hides. It is read-only: a card
// opens its Ticket in the Inbox's All Tickets view, on the Board's Project.
// It reads only what the Inbox loads, and follows the live feed. Back
// returns to the Inbox it was opened from.
import { useEffect, useRef, type CSSProperties } from "react";
import type { Api } from "../api/api";
import { FINISHED, STATES, type NeedsMeReason, type State } from "../api/types";
import { ProjectPicker } from "../controls/ProjectPicker";
import { ToggleGroup } from "../controls/ToggleGroup";
import { ViewHeader } from "../controls/ViewHeader";
import { usePulse } from "../focus/pulse";
import { kindLabel, newestFirst } from "../inbox/AllTickets";
import { hiddenColumns, storedGroup, storeGroup, storeHidden } from "../inbox/groupPreference";
import { OneProject, ShortId } from "../inbox/ShortId";
import { specKindOf } from "../inbox/SpecRow";
import { stateSlug } from "../inbox/StateRow";
import type { InboxData, ProjectSpec, ProjectTicket } from "../live";
import { reasonsOf } from "../requirement/progress";
import { NewSplit } from "../requirement/NewSplit";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { ColumnsMenu } from "./ColumnsMenu";
import "./board.css";

export interface BoardViewProps {
  api: Api;
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function BoardView({ api, data, loc, navigate }: BoardViewProps) {
  const projects = data?.projects ?? [];
  const hidden = hiddenColumns(loc.hide);
  const shown = STATES.filter((s) => !hidden.includes(s));
  // The grouping in effect, shared with the Inbox's State lists: the
  // address's, else this browser's last pick. By Spec shows the columns
  // alone until its lanes are built.
  const grouping = loc.group ?? storedGroup();
  const tickets = newestFirst((data?.tickets ?? []).filter((t) => !loc.project || t.project === loc.project));
  const inColumn = new Map(shown.map((s) => [s, tickets.filter((t) => t.state === s)]));
  const specs = new Map((data?.specs ?? []).map((s) => [s.id, s]));
  const reasons = reasonsOf(data?.needsMe);
  const joined = useJoined(data?.tickets);

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
            <div className="board-grid" style={{ "--cols": shown.length } as CSSProperties}>
              {shown.map((s) => (
                <h2 key={s} className={`board-col-head st-${stateSlug(s)}`}>
                  <span className="board-col-name">{s}</span> <span className="board-col-count">{inColumn.get(s)!.length}</span>
                </h2>
              ))}
              {shown.map((s) => (
                <Column
                  key={s}
                  state={s}
                  tickets={inColumn.get(s)!}
                  specs={specs}
                  reasons={reasons}
                  joined={joined}
                  open={(t) => {
                    const to = open(t);
                    return { href: hrefFor(to as UiLocation), onClick: clickTo(navigate, to) };
                  }}
                />
              ))}
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

function Column({
  state,
  tickets,
  specs,
  reasons,
  joined,
  open,
}: {
  state: State;
  tickets: ProjectTicket[];
  specs: Map<string, ProjectSpec>;
  reasons: Map<string, NeedsMeReason>;
  joined(t: ProjectTicket): boolean;
  open(t: ProjectTicket): CardLink;
}) {
  return (
    <section aria-label={state} className="board-cell">
      {tickets.length === 0 ? (
        <p className="board-empty">No Tickets</p>
      ) : (
        tickets.map((t) => (
          <Card key={t.id} t={t} spec={specs.get(t.spec)} reason={reasons.get(t.id)} joined={joined(t)} link={open(t)} />
        ))
      )}
    </section>
  );
}

// Card is a Ticket on the Board: its short ID, kind, Blocked and needs-you
// dot on top, its title, then its Spec's kind and name. It pulses when its
// Ticket's last update changes.
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
