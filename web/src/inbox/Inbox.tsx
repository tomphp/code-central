// The triage inbox: Projects and Views on the left, the list in the
// middle, the selected Ticket on the right. A layout only: the data comes
// from live.ts and the Ticket pane is shared with any later view.
import type { MouseEvent } from "react";
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import { STATES, type NeedsMeRow, type TicketView } from "../api/types";
import { duration, REASON_LABEL, rowDetail } from "../format";
import type { InboxData, ProjectTicket } from "../live";
import { TicketPane } from "../ticket/TicketDetail";
import { hrefFor, type UiLocation } from "../url";

export interface InboxProps {
  api: Api;
  now: Date;
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  ticket?: TicketView;
  ticketError?: string;
}

export function Inbox({ api, now, data, loc, navigate, ticket, ticketError }: InboxProps) {
  const inProject = (key: string) => !loc.project || key === loc.project;
  const projects = data?.projects ?? [];
  const groups = (data?.needsMe.groups ?? [])
    .map((g) => ({ ...g, tickets: g.tickets.filter((r) => inProject(r.project)) }))
    .filter((g) => g.tickets.length > 0);
  const waiting = new Set((data?.needsMe.groups ?? []).flatMap((g) => g.tickets.map((r) => r.id)));
  const tickets = (data?.tickets ?? []).filter((t) => inProject(t.project));
  const drafts = tickets.filter((t) => t.state === "Draft");
  const needsMeCount = groups.reduce((n, g) => n + g.tickets.length, 0);
  const totalCount = data?.needsMe.count ?? 0;

  const select = (id: string) => (e: MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return; // let the browser open a tab
    e.preventDefault();
    navigate({ ticket: id });
  };
  const rowLink = (id: string) => ({
    href: hrefFor({ ...loc, ticket: id }),
    onClick: select(id),
    "data-id": id,
    "aria-current": loc.ticket === id ? ("true" as const) : undefined,
  });

  return (
    <div className="inbox">
      <nav aria-label="Filters" className="inbox-nav">
        <h1>Code Central</h1>
        <FilterButton on={!loc.project} label="All Projects" count={totalCount} loud onClick={() => navigate({ project: undefined })} />
        {projects.map((p) => (
          <FilterButton
            key={p.key}
            on={loc.project === p.key}
            label={p.display_name}
            count={p.needs_me}
            loud
            quiet={p.archived}
            onClick={() => navigate({ project: p.key })}
          />
        ))}
        <h2>View</h2>
        <FilterButton on={!loc.state} label="Needs Me" count={needsMeCount} loud onClick={() => navigate({ state: undefined })} />
        {STATES.map((s) => (
          <FilterButton
            key={s}
            sub
            on={loc.state === s}
            label={s}
            count={tickets.filter((t) => t.state === s).length}
            onClick={() => navigate({ state: s })}
          />
        ))}
      </nav>

      <main aria-label="Tickets" className="inbox-list">
        {!data ? (
          <p className="empty">Loading…</p>
        ) : loc.state ? (
          <>
            {tickets
              .filter((t) => t.state === loc.state)
              .map((t) => (
                <a key={t.id} className="row" {...rowLink(t.id)}>
                  <span className="row-top">
                    <span className="tid">{t.id}</span>
                    <span className="row-title">{t.title}</span>
                    {waiting.has(t.id) && <span className="dot needs-you" role="img" aria-label="Needs you" />}
                  </span>
                </a>
              ))}
          </>
        ) : (
          <>
            {groups.length === 0 && <p className="empty">Nothing needs you.</p>}
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
            {drafts.length > 0 && <Drafts drafts={drafts} link={rowLink} />}
          </>
        )}
      </main>

      <aside aria-label="Ticket" className="inbox-detail">
        {ticketError ? (
          <p className="empty">{ticketError}</p>
        ) : ticket && loc.ticket && ticket.id === loc.ticket ? (
          <TicketPane api={api} project={projectOf(ticket.id)} ticket={ticket} now={now} />
        ) : (
          <p className="empty">{loc.ticket ? "Loading…" : "Select a Ticket."}</p>
        )}
      </aside>
    </div>
  );
}

function FilterButton(p: { on: boolean; label: string; count: number; loud?: boolean; quiet?: boolean; sub?: boolean; onClick(): void }) {
  return (
    <button
      className={["filter", p.on && "on", p.sub && "sub", p.quiet && "quiet"].filter(Boolean).join(" ")}
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
        <span className="muted small">{duration(now, row.since)}</span>
      </span>
      <span className="row-detail">{rowDetail(now, row)}</span>
    </a>
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
