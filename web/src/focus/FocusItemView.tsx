// The Focus Item view: one Focus Item, top to bottom, shared by Focus Mode
// and the Focus popup. Where it's from, why it waits, then its Question
// with the answer form, or its reason's panel, and the Base diverged and
// Base missing fixes. What differs by host is passed in: the Api to act
// through, and the optional Skip and Open in Inbox.
import type { Api } from "../api/api";
import type { NeedsMeRow, TicketView } from "../api/types";
import { duration, shortCommit, ticketOf } from "../format";
import { FocusBadge } from "./FocusBadge";
import { ShortId } from "../inbox/ShortId";
import { WithApi, type InboxData } from "../live";
import { Markdown } from "../Markdown";
import { panelFor } from "../panels/ActionPanel";
import { QuestionAnswer } from "../panels/round/QuestionAnswer";
import { PrototypeBadge } from "../ticket/PrototypeBadge";
import { RetroBadge } from "../ticket/RetroBadge";
import { clickTo, hrefFor, type UiLocation } from "../url";
import type { FocusItem } from "./session";
import { Button } from "../controls/Button";
import { Loading } from "../controls/Empty";
import { ProjectName } from "../controls/ProjectName";
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
  // inOneProject drops the Project name above the item, when the host's
  // header already names the one Project in scope.
  inOneProject?: boolean;
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
  inOneProject,
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
    <WithApi api={api}>
    <article
      aria-label="Focus Item"
      className={question ? "focus-item focus-item-question" : "focus-item"}
    >
      {!inOneProject && (
        <p className="focus-from">
          <span className="focus-item-project">
            <ProjectName name={project?.display_name ?? row.project} projectKey={row.project} />
          </span>
        </p>
      )}
      <p className="focus-badges">
        <FocusBadge reason={row.reason} />
        {row.prototype && <PrototypeBadge />}
        {row.retro && <RetroBadge />}
      </p>
      {row.id ? (
        <p className="focus-ticket">
          <ShortId id={row.id} />
        </p>
      ) : (
        row.ci_item && (
          // A CI item's line is the Tickets it lists.
          <p className="focus-ticket">
            {row.ci_item.tickets.map((t, i) => (
              <span key={t.id}>
                {i > 0 && ", "}
                <ShortId id={t.id} />
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
      <p className="focus-why">{waited === "just now" ? waited : `waiting ${waited}`}</p>
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
            project={row.project}
            ticket={row.id}
            q={question.q}
            submitOnModEnter
          />
          {skipButton && <p className="focus-skip">{skipButton}</p>}
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
      ) : row.repository_missing ? (
        <>
          <RepositoryMissingFix row={row} navigate={navigate} />
          {skipButton && <p className="focus-skip">{skipButton}</p>}
        </>
      ) : (
        <>
          {ticket ? (
            Panel && (
              <Panel
                key={ticket.state}
                project={row.project}
                ticket={ticket}
                now={now}
              />
            )
          ) : (
            <Loading />
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
    </WithApi>
  );
}

// BaseDivergedFix is a Base diverged item, a Project's or, with several
// Repositories, one Repository's: both commits and the fix, which is the
// human's by hand; the Factory carries on by itself.
function BaseDivergedFix({ row }: { row: NeedsMeRow }) {
  const b = row.base_diverged!;
  const base = b.base ?? "the base";
  return (
    <section aria-label="Base diverged" className="panel">
      {b.repository ? (
        <p>
          The Factory can't fast-forward {base} in Repository <code>{b.repository}</code>: it holds commits the remote's doesn't. Nothing touching{" "}
          {b.repository} lands and no new branch is cut until they agree.
        </p>
      ) : (
        <p>
          The Factory can't fast-forward {base}: it holds commits the remote's
          doesn't. Nothing lands and no new branch is cut until they agree.
        </p>
      )}
      <p>
        Local <code>{shortCommit(b.local)}</code> · remote{" "}
        <code>{shortCommit(b.remote)}</code>
      </p>
      <p>
        Reconcile {base} with the remote by hand, for example by resetting or
        rebasing it. The Factory carries on by itself on its next poll.
      </p>
    </section>
  );
}

// BaseMissingFix is a Base missing item, a Project's or, with several
// Repositories, one Repository's: the branch and the fix, with a link to
// where the base is changed.
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
      {b.repository ? (
        <p>
          The base of Repository <code>{b.repository}</code>, <code>{b.base}</code>, exists neither locally nor on the remote. Nothing touching{" "}
          {b.repository} lands and no new branch is cut until it does.
        </p>
      ) : (
        <p>
          The base, <code>{b.base}</code>, exists neither locally nor on the
          remote. Nothing lands and no new branch is cut until it does.
        </p>
      )}
      <p>
        Create the branch, or change the base in{" "}
        <a href={hrefFor(options)} onClick={clickTo(navigate, options)}>
          Project Options
        </a>
        . The Factory carries on by itself on its next poll.
      </p>
    </section>
  );
}

// RepositoryMissingFix is a Repository missing item: the path the Factory
// left untouched, the Clone URL (none for a local-only Repository), what
// is wrong there and the fix, with a link to where the local path is set.
function RepositoryMissingFix({
  row,
  navigate,
}: {
  row: NeedsMeRow;
  navigate: (patch: Partial<UiLocation>) => void;
}) {
  const m = row.repository_missing!;
  const options = { view: "project-options" as const, project: row.project };
  // The fix names Project Options itself; that mention is the link, so the
  // name isn't said twice.
  const fix = m.fix.replace(/\.$/, "");
  const named = fix.endsWith("Project Options");
  const fixBefore = named ? fix.slice(0, -"Project Options".length) : `${fix} (`;
  const link = (
    <>
      <a href={hrefFor(options)} onClick={clickTo(navigate, options)}>
        Project Options
      </a>
      {!named && ")"}
    </>
  );
  return (
    <section aria-label="Repository missing" className="panel">
      <p>
        The Factory has no clone of Repository <strong>{m.repository}</strong>,
        so no new Claim starts until it has. It leaves the path as it is.
      </p>
      <dl className="focus-repository-missing">
        <dt>Path</dt>
        <dd>
          <code>{m.path}</code>
        </dd>
        {m.clone_url && (
          <>
            <dt>Clone URL</dt>
            <dd>
              <code>{m.clone_url}</code>
            </dd>
          </>
        )}
        <dt>Problem</dt>
        <dd>{m.problem}</dd>
      </dl>
      <p>
        {fixBefore}
        {link}. The Factory carries on by itself on its next poll.
      </p>
    </section>
  );
}
