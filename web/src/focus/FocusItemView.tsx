// The Focus Item view: one Focus Item, top to bottom, shared by Focus Mode
// and the Focus popup. Where it's from, why it waits, then its Question
// with the answer form, or its reason's panel, and the Base diverged and
// Base missing fixes. What differs by host is passed in: the Api to act
// through, and the optional Skip and Open in Inbox.
import type { Api } from "../api/api";
import type { NeedsMeRow, TicketView } from "../api/types";
import { duration, REASON_LABEL, ticketOf } from "../format";
import type { InboxData } from "../live";
import { Markdown } from "../Markdown";
import { panelFor } from "../panels/ActionPanel";
import { QuestionAnswer } from "../panels/round/QuestionAnswer";
import { PrototypeBadge } from "../ticket/PrototypeBadge";
import { clickTo, hrefFor, type UiLocation } from "../url";
import type { FocusItem } from "./session";
import { Button } from "../controls/Button";
import "./focus.css";

export interface FocusItemViewProps {
  api: Api;
  now: Date;
  data: InboxData;
  item: FocusItem;
  // ticket is the item's Ticket, once read.
  ticket?: TicketView;
  navigate(patch: Partial<UiLocation>): void;
  // skip sends the item to the back of the queue; no Skip shows without it.
  skip?(): void;
  // inbox is where Open in Inbox goes; no link shows without it, or for an
  // item with no Ticket.
  inbox?: UiLocation;
}

// inboxOf is where Open in Inbox goes for a row, if it has a Ticket, on
// the Project in scope: a Stage Ticket opens in its Requirement, with it
// selected; a CI item opens on the first Ticket it lists.
export function inboxOf(row: NeedsMeRow, project?: string): UiLocation | undefined {
  const ticket = ticketOf(row);
  if (!ticket) return undefined;
  return {
    view: "inbox" as const,
    project,
    requirement: row.requirement?.stage && row.requirement.spec,
    ticket,
  };
}

// FocusItemView is one Focus Item: where it's from, why it waits, then its
// Question with the answer form, or its reason's panel with the panel's
// own buttons, and Skip when given.
export function FocusItemView({
  api,
  now,
  data,
  item: { row, question },
  ticket,
  navigate,
  skip,
  inbox,
}: FocusItemViewProps) {
  const project = data.projects.find((p) => p.key === row.project);
  const Panel = panelFor(row.reason, ticket?.state)?.Component;
  const waited = duration(now, row.since);
  const open = inbox && clickTo(navigate, inbox);
  const skipButton = skip && (
    <Button type="button"  onClick={skip}>
      Skip
    </Button>
  );

  return (
    <article
      aria-label="Focus Item"
      className={question ? "focus-item focus-item-question" : "focus-item"}
    >
      <p className="focus-from">
        <span className="focus-item-project">
          {project?.display_name ?? row.project}
        </span>{" "}
        <span className="muted">{row.project}</span>
      </p>
      {row.id ? (
        <p className="focus-ticket">
          <span className="tid">{row.id}</span>
        </p>
      ) : (
        row.ci_item && (
          // A CI item's line is the Tickets it lists.
          <p className="focus-ticket">
            {row.ci_item.tickets.map((t, i) => (
              <span key={t.id}>
                {i > 0 && " "}
                <span className="tid">{t.id}</span>
              </span>
            ))}
          </p>
        )
      )}
      <h2>{row.title}</h2>
      {row.requirement && (
        <p className="focus-requirement">
          {row.requirement.stage
            ? `Requirement: ${row.requirement.title} · ${row.requirement.stage}`
            : `Requirement: ${row.requirement.title}`}
        </p>
      )}
      {row.spec && (
        <p className="focus-requirement">{`${row.spec.map ? "Map" : "Spec"}: ${row.spec.title}`}</p>
      )}
      {row.prototype && (
        <p className="focus-badges">
          <PrototypeBadge />
        </p>
      )}
      <p className="focus-why">
        <span className={`reason reason-${row.reason}`}>
          <span className="dot" aria-hidden="true" />{" "}
          <span>{REASON_LABEL[row.reason]}</span>
        </span>{" "}
        · <span>{waited === "just now" ? waited : `waiting ${waited}`}</span>
      </p>
      {/* Keyed on the State too, as the Inbox's slot is, so a panel's local
          state never carries over to another State. */}
      {question ? (
        <section aria-label="Question" className="focus-question">
          <p className="focus-question-of">
            Question {question.place} of {question.count} · Round{" "}
            {question.round}
          </p>
          <h3>{question.q.title}</h3>
          <div className="focus-question-body">
            <Markdown>{question.q.body}</Markdown>
          </div>
          <QuestionAnswer
            api={api}
            project={row.project}
            ticket={row.id}
            q={question.q}
            submitOnModEnter
          >
            {skipButton}
          </QuestionAnswer>
        </section>
      ) : row.base_diverged ? (
        <>
          <BaseDivergedFix row={row} />
          {skipButton && <p className="focus-skip">{skipButton}</p>}
        </>
      ) : row.base_missing ? (
        <>
          <BaseMissingFix row={row} navigate={navigate} />
          {skipButton && <p className="focus-skip">{skipButton}</p>}
        </>
      ) : (
        <>
          {ticket ? (
            Panel && (
              <Panel
                key={ticket.state}
                api={api}
                project={row.project}
                ticket={ticket}
                now={now}
              />
            )
          ) : (
            <p className="muted">Loading…</p>
          )}
          {/* Skip waits for the read, so a Round is never skipped whole
              before its Questions are known. */}
          {ticket && skipButton && <p className="focus-skip">{skipButton}</p>}
        </>
      )}
      {inbox && open && (
        <p className="focus-open">
          <a href={hrefFor(inbox)} onClick={open}>
            Open in Inbox
          </a>
        </p>
      )}
    </article>
  );
}

// BaseDivergedFix is a Project's Base diverged item: both commits and the
// fix, which is the human's by hand; the Factory carries on by itself.
function BaseDivergedFix({ row }: { row: NeedsMeRow }) {
  const b = row.base_diverged!;
  const base = b.base ?? "the base";
  return (
    <section aria-label="Base diverged" className="panel">
      <p>
        The Factory can't fast-forward {base}: it holds commits the remote's
        doesn't. Nothing lands and no new branch is cut until they agree.
      </p>
      <p>
        Local <code>{b.local.slice(0, 7)}</code> · remote{" "}
        <code>{b.remote.slice(0, 7)}</code>
      </p>
      <p className="small">
        Reconcile {base} with the remote by hand, for example by resetting or
        rebasing it. The Factory carries on by itself on its next poll.
      </p>
    </section>
  );
}

// BaseMissingFix is a Project's Base missing item: the branch and the fix,
// with a link to where the base is changed.
function BaseMissingFix({
  row,
  navigate,
}: {
  row: NeedsMeRow;
  navigate: (patch: Partial<UiLocation>) => void;
}) {
  const b = row.base_missing!;
  const options = { view: "project-options" as const, project: row.project };
  return (
    <section aria-label="Base missing" className="panel">
      <p>
        The base, <code>{b.base}</code>, exists neither locally nor on the
        remote. Nothing lands and no new branch is cut until it does.
      </p>
      <p className="small">
        Create the branch, or change the base in{" "}
        <a href={hrefFor(options)} onClick={clickTo(navigate, options)}>
          Project Options
        </a>
        . The Factory carries on by itself on its next poll.
      </p>
    </section>
  );
}
