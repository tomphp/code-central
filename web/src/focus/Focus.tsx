// Focus Mode: one Focus Item at a time from Needs Me, filling the window
// with no nav, list or side pane. A layout only: the data comes from
// live.ts, as the Inbox's does, and focusQueue picks what to show. Its
// root's class scopes the Focus styles, so the panels inside read larger
// here and nowhere else.
import { useEffect, useState, type MouseEvent } from "react";
import type { Api } from "../api/api";
import type { TicketView } from "../api/types";
import { duration, REASON_LABEL } from "../format";
import { useTicket, type InboxData } from "../live";
import { Markdown } from "../Markdown";
import { panelFor } from "../panels/ActionPanel";
import { QuestionAnswer } from "../panels/round/QuestionAnswer";
import { hrefFor, opensInPlace, type UiLocation } from "../url";
import { focusQueue, type FocusItem } from "./queue";
import "./focus.css";

export interface FocusProps {
  api: Api;
  now: Date;
  version: number;
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function Focus({ api, now, version, data, loc, navigate }: FocusProps) {
  const project = data?.projects.find((p) => p.key === loc.project);
  const [onScreen, setOnScreen] = useState<FocusItem>();
  const ticket = useTicket(api, onScreen?.row.project, onScreen?.row.id, version);
  const screen = data && focusQueue({ needsMe: data.needsMe, project: loc.project, onScreen, ticket: ticket.data });
  const item = screen?.item;
  // What's shown becomes what's on screen, so a later read keeps it there.
  useEffect(() => {
    if (screen && item?.key !== onScreen?.key) setOnScreen(item);
  }, [screen, item, onScreen]);

  return (
    <main aria-label="Focus Mode" className="focus">
      <header className="focus-head">
        {loc.project && (
          <span className="focus-project">
            {project?.display_name ?? loc.project} <span className="muted">{loc.project}</span>
          </span>
        )}
        {screen && !screen.clear && <span className="focus-left">{screen.remaining} left</span>}
        <button className="btn focus-exit" onClick={() => navigate({ view: "inbox" })}>
          Exit
        </button>
      </header>
      {!screen ? (
        <p className="focus-loading">Loading…</p>
      ) : screen.clear ? (
        <section aria-label="All clear" className="focus-clear">
          <h2>All clear</h2>
          <p>Nothing needs you.</p>
        </section>
      ) : item ? (
        <Item
          key={item.key}
          api={api}
          now={now}
          data={data}
          item={item}
          ticket={ticket.data?.id === item.row.id ? ticket.data : undefined}
          loc={loc}
          navigate={navigate}
        />
      ) : (
        <p className="focus-loading">Loading…</p>
      )}
    </main>
  );
}

interface ItemProps {
  api: Api;
  now: Date;
  data: InboxData;
  item: FocusItem;
  // ticket is the item's Ticket, once read.
  ticket?: TicketView;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

// Item is one Focus Item, top to bottom: where it's from, why it waits,
// then its Question with the answer form, or its reason's panel with the
// panel's own buttons.
function Item({ api, now, data, item: { row, question }, ticket, loc, navigate }: ItemProps) {
  const project = data.projects.find((p) => p.key === row.project);
  const Panel = panelFor(row.reason, ticket?.state)?.Component;
  const waited = duration(now, row.since);
  // A Stage Ticket opens in its Requirement, with it selected.
  const inbox = { view: "inbox" as const, project: loc.project, requirement: row.requirement?.stage && row.requirement.spec, ticket: row.id };
  const open = (e: MouseEvent) => {
    if (!opensInPlace(e)) return;
    e.preventDefault();
    navigate(inbox);
  };

  return (
    <article aria-label="Focus Item" className={question ? "focus-item focus-item-question" : "focus-item"}>
      <p className="focus-from">
        <span className="focus-item-project">{project?.display_name ?? row.project}</span> <span className="muted">{row.project}</span>
      </p>
      <p className="focus-ticket">
        <span className="tid">{row.id}</span>
      </p>
      <h2>{row.title}</h2>
      {row.requirement && (
        <p className="focus-requirement">
          {row.requirement.stage ? `Requirement: ${row.requirement.title} · ${row.requirement.stage}` : `Requirement: ${row.requirement.title}`}
        </p>
      )}
      <p className="focus-why">
        <span className={`reason reason-${row.reason}`}>
          <span className="dot" aria-hidden="true" /> <span>{REASON_LABEL[row.reason]}</span>
        </span>{" "}
        · <span>{waited === "just now" ? waited : `waiting ${waited}`}</span>
      </p>
      {/* Keyed on the State too, as the Inbox's slot is, so a panel's local
          state never carries over to another State. */}
      {question ? (
        <section aria-label="Question" className="focus-question">
          <p className="focus-question-of">
            Question {question.place} of {question.count} · Round {question.round}
          </p>
          <h3>{question.q.title}</h3>
          <div className="focus-question-body">
            <Markdown>{question.q.body}</Markdown>
          </div>
          <QuestionAnswer api={api} project={row.project} ticket={row.id} q={question.q} submitOnModEnter />
        </section>
      ) : ticket ? (
        Panel && <Panel key={ticket.state} api={api} project={row.project} ticket={ticket} now={now} />
      ) : (
        <p className="muted">Loading…</p>
      )}
      <p className="focus-open">
        <a href={hrefFor(inbox)} onClick={open}>
          Open in Inbox
        </a>
      </p>
    </article>
  );
}
