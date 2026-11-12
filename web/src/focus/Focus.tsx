// Focus Mode: one Focus Item at a time from Needs Me, filling the window
// with no nav, list or side pane. A layout only: the data comes from
// live.ts, as the Inbox's does, and focusQueue picks what to show. Its
// root's class scopes the Focus styles, so the panels inside read larger
// here and nowhere else.
//
// The panels and the Question form get an Api whose writes (act, and
// unlink) first record that the item on screen was acted on, so an item that leaves after the
// human's own action moves on silently, and one that leaves otherwise
// moves on with a brief "Dealt with elsewhere" note. Submitting from
// + New ▾ shows a brief "Requirement added" or "Map added" note instead;
// there's only ever one note, the latest.
import { useEffect, useMemo, useState } from "react";
import type { Api } from "../api/api";
import type { FocusClear, NeedsMeRow, TicketView } from "../api/types";
import { duration, REASON_LABEL, ticketOf } from "../format";
import { useTicket, type InboxData } from "../live";
import { Markdown } from "../Markdown";
import { panelFor } from "../panels/ActionPanel";
import { QuestionAnswer } from "../panels/round/QuestionAnswer";
import { NewSplit } from "../requirement/NewSplit";
import { PrototypeBadge } from "../ticket/PrototypeBadge";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { ActivityDashboard } from "./ActivityDashboard";
import { focusQueue, type FocusItem } from "./queue";
import "./focus.css";

export interface FocusProps {
  api: Api;
  now: Date;
  version: number;
  data?: InboxData;
  // focusClear is what to show when nothing needs the human: All clear,
  // or the Activity Dashboard.
  focusClear: FocusClear;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function Focus({
  api,
  now,
  version,
  data,
  focusClear,
  loc,
  navigate,
}: FocusProps) {
  const project = data?.projects.find((p) => p.key === loc.project);
  const [onScreen, setOnScreen] = useState<FocusItem>();
  // skipped lives here for this visit only: leaving Focus Mode or
  // reloading forgets it, and it's never sent or put in the URL.
  const [skipped, setSkipped] = useState<FocusItem[]>([]);
  // actedOn is the key of the item on screen once the human acts on it
  // here, until a refusal takes it back.
  const [actedOn, setActedOn] = useState<string>();
  const [note, setNote] = useState<Note>();
  // A CI item has no Ticket of its own: its panel shows the first Ticket
  // it lists, whose options act on every one of them.
  const ticket = useTicket(
    api,
    onScreen?.row.project,
    onScreen && ticketOf(onScreen.row),
    version,
  );
  const acted = !!onScreen && actedOn === onScreen.key;
  const screen =
    data &&
    focusQueue({
      needsMe: data.needsMe,
      project: loc.project,
      onScreen,
      ticket: ticket.data,
      skipped,
      acted,
    });
  const item = screen?.item;
  // What's shown becomes what's on screen, so a later read keeps it there.
  // Moving on shows "Dealt with elsewhere" only when the item left without
  // the human, and only over a next item, so All clear stays calm; moving
  // on otherwise ends it, but not an "added" note. A Round becoming its
  // first Question, once read, isn't moving on, so it keeps the note.
  useEffect(() => {
    if (!screen || item?.key === onScreen?.key) return;
    const roundRead =
      !!onScreen?.roundKey &&
      !onScreen.question &&
      onScreen.roundKey === item?.roundKey;
    setOnScreen(item);
    setActedOn(undefined);
    if (roundRead) return;
    if (screen.elsewhere && item)
      setNote({ text: "Dealt with elsewhere", elsewhere: true });
    else setNote((n) => (n?.elsewhere ? undefined : n));
  }, [screen, item, onScreen]);
  // The note is brief: it goes after NOTE_MS, and a newer note starts its
  // own NOTE_MS.
  useEffect(() => {
    if (!note) return;
    const timer = setTimeout(() => setNote(undefined), NOTE_MS);
    return () => clearTimeout(timer);
  }, [note]);
  const itemKey = item?.key;
  const itemApi = useMemo(
    () =>
      itemKey
        ? recordingWrites(api, {
            onWrite: () => setActedOn(itemKey),
            onRefused: () => setActedOn((k) => (k === itemKey ? undefined : k)),
          })
        : undefined,
    [api, itemKey],
  );

  return (
    <main aria-label="Focus Mode" className="focus">
      <header className="focus-head">
        {loc.project && (
          <span className="focus-project">
            {project?.display_name ?? loc.project}{" "}
            <span className="muted">{loc.project}</span>
          </span>
        )}
        {screen && !screen.clear && (
          <span className="focus-left">{screen.remaining} left</span>
        )}
        {/* Submitting goes nowhere: whatever is on screen stays, skips and
            all, with a note on every screen. */}
        <div className="focus-new">
          <NewSplit
            variant="plain"
            api={api}
            projects={data?.projects.filter((p) => !p.archived) ?? []}
            project={loc.project}
            onSubmitted={({ kind }) => setNote({ text: `${kind} added` })}
          />
        </div>
        <button className="btn" onClick={() => navigate({ view: "inbox" })}>
          Exit
        </button>
      </header>
      {note && (
        <p role="status" className="focus-note">
          {note.text}
        </p>
      )}
      {!screen ? (
        <p className="focus-loading">Loading…</p>
      ) : screen.clear && focusClear === "activity_dashboard" ? (
        <ActivityDashboard
          api={api}
          now={now}
          version={version}
          data={data}
          loc={loc}
          navigate={navigate}
        />
      ) : screen.clear ? (
        <section aria-label="All clear" className="focus-clear">
          <h2>All clear</h2>
          <p>Nothing needs you.</p>
        </section>
      ) : screen.onlySkipped ? (
        <section
          aria-label="Only skipped Focus Items left"
          className="focus-clear"
        >
          <p>You skipped {screen.remaining}, go through them again?</p>
          <button className="btn primary" onClick={() => setSkipped([])}>
            Go through them again
          </button>
        </section>
      ) : item && itemApi ? (
        <Item
          key={item.key}
          api={itemApi}
          now={now}
          data={data}
          item={item}
          ticket={ticket.data?.id === ticketOf(item.row) ? ticket.data : undefined}
          loc={loc}
          navigate={navigate}
          skip={() => setSkipped([...skipped, item])}
        />
      ) : (
        <p className="focus-loading">Loading…</p>
      )}
    </main>
  );
}

// Note is the brief note under the header; elsewhere marks "Dealt with
// elsewhere", which moving on ends.
interface Note {
  text: string;
  elsewhere?: boolean;
}

// NOTE_MS is how long a note stays.
const NOTE_MS = 4000;

// WRITES are the Api calls a panel or the Question form makes to act on
// a Ticket.
const WRITES: ReadonlySet<PropertyKey> = new Set<keyof Api>(["act", "unlink"]);

// recordingWrites is api with each write calling onWrite before it's
// forwarded, so the live notice it causes finds it recorded, and
// onRefused if it's refused, as a refused write left the item where it
// was. Every other call goes straight to api.
function recordingWrites(
  api: Api,
  { onWrite, onRefused }: { onWrite(): void; onRefused(): void },
): Api {
  return new Proxy(api, {
    get(target, prop) {
      const v = Reflect.get(target, prop);
      if (typeof v !== "function") return v;
      if (!WRITES.has(prop)) return v.bind(target);
      return (...args: unknown[]) => {
        onWrite();
        return (v.apply(target, args) as Promise<unknown>).catch(
          (e: unknown) => {
            onRefused();
            throw e;
          },
        );
      };
    },
  });
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
  // skip sends the item to the back of the queue.
  skip(): void;
}

// Item is one Focus Item, top to bottom: where it's from, why it waits,
// then its Question with the answer form, or its reason's panel with the
// panel's own buttons, and Skip.
function Item({
  api,
  now,
  data,
  item: { row, question },
  ticket,
  loc,
  navigate,
  skip,
}: ItemProps) {
  const project = data.projects.find((p) => p.key === row.project);
  const Panel = panelFor(row.reason, ticket?.state)?.Component;
  const waited = duration(now, row.since);
  // A Stage Ticket opens in its Requirement, with it selected; a CI item
  // opens on the first Ticket it lists.
  const ticketId = ticketOf(row);
  const inbox = {
    view: "inbox" as const,
    project: loc.project,
    requirement: row.requirement?.stage && row.requirement.spec,
    ticket: ticketId,
  };
  const open = clickTo(navigate, inbox);
  const skipButton = (
    <button type="button" className="btn" onClick={skip}>
      Skip
    </button>
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
          <p className="focus-skip">{skipButton}</p>
        </>
      ) : row.base_missing ? (
        <>
          <BaseMissingFix row={row} navigate={navigate} />
          <p className="focus-skip">{skipButton}</p>
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
          {ticket && <p className="focus-skip">{skipButton}</p>}
        </>
      )}
      {ticketId && (
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
