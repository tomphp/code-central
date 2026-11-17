// The selected Ticket, shared by every view: TicketPane reads what the
// pane needs beyond the Ticket, and TicketDetail draws it from props.
import { useState } from "react";
import { moveTargets } from "../api/actions/move";
import type { EditView, ReportedModels, State, TicketView } from "../api/types";
import { ago, CLAUDE_CODE_DEFAULT } from "../format";
import { Markdown } from "../Markdown";
import { EditForm } from "../edit/EditForm";
import { retroable, ticketEditable } from "../edit/editable";
import { NewRetro } from "../requirement/NewRequirement";
import { RetroList } from "../spec/RetroList";
import { ActionPanel } from "../panels/ActionPanel";
import { LatestEventLine } from "../LatestEvent";
import { LinkBadge } from "./LinkBadge";
import { OpenExternal } from "./OpenExternal";
import { Live } from "./Live";
import { PullRequestBadge } from "./PullRequestBadge";
import { WaitingBadge } from "./WaitingBadge";
import { PartlyLandedBadge } from "./PartlyLandedBadge";
import { useAct } from "./useAct";
import { CISection } from "./CISection";
import { TicketRepositories } from "./TicketRepositories";
import { Triage } from "./Triage";
import { ModelChoice } from "./ModelChoice";
import { ShortId, shortOf } from "../inbox/ShortId";
import { Button } from "../controls/Button";
import { ReasonBadge } from "../controls/ReasonBadge";
import { Select } from "../controls/Select";
import { StateChip } from "../controls/StateChip";
import { useApi, useSpec } from "../live";
import { isCIItemReason } from "../needsme/model";
import { AutoAcceptSwitch } from "./AutoAcceptSwitch";
import { AUTO_ACCEPT_LABEL, canAutoAccept, moveText, wouldAutoAccept } from "./autoAccept";

export interface TicketDetailProps {
  project: string;
  // ticket is the Ticket once read; until then the pane says Loading…, or
  // shows error, the read's error.
  ticket?: TicketView;
  error?: string;
  // specLabels are the Labels of the Ticket's Spec once read, which the
  // Auto-accept switch waits for.
  specLabels?: string[];
  now: Date;
  // reported is the Project's Reported Models, once read: what the Model
  // and effort pickers offer, and the Factory's flags.
  reported?: ReportedModels;
  // openSpec opens the Ticket's Spec, where the view can show it; without
  // it the Spec is named only.
  openSpec?(spec: string): void;
  // openTicket selects another Ticket, as a new Retro is; without it the
  // Retro button isn't offered.
  openTicket?(ticket: string): void;
  // archived is whether the Ticket's Project is Archived, which takes no
  // edits.
  archived?: boolean;
  // onMoved is told once the Move control has moved the Ticket: the Board's
  // popup closes then.
  onMoved?(): void;
}

export type TicketPaneProps = Omit<TicketDetailProps, "specLabels">;

// TicketPane is the Ticket pane's view: it reads the Ticket's Spec on the
// live version, for its Labels, and draws TicketDetail.
export function TicketPane(props: TicketPaneProps) {
  const spec = useSpec(props.project, props.ticket?.spec.id || undefined).data;
  return <TicketDetail {...props} specLabels={spec?.labels} />;
}

// TicketDetail is the Ticket pane, drawn from its props: its header, the
// action panel slot first, then the Ticket itself.
export function TicketDetail(props: TicketDetailProps) {
  const api = useApi();
  // The form owns its draft, so it stays open if the Ticket stops being
  // editable under it; Edit is offered only while it is closed.
  const [editing, setEditing] = useState(false);
  const t = props.ticket;
  if (!t) {
    return props.error ? (
      <p role="alert" className="warning">
        {props.error}
      </p>
    ) : (
      <p className="empty">Loading…</p>
    );
  }
  return (
    <>
      <header className="ticket-head">
        <ShortId id={t.id} /> <StateChip state={t.state} />
        <WaitingBadge waiting={t.waiting_for_spec} />
        <PartlyLandedBadge partly={t.partly_landed} />
        {/* The In Review reason only repeats the State chip. */}
        {t.needs_me_reason && t.needs_me_reason !== "in_review" && <ReasonBadge reason={t.needs_me_reason} />}
        {editing ? (
          <EditForm
            title={t.title}
            text={t.description}
            label="description"
            save={({ title, text }) => api.act(props.project, t.id, "edit_ticket", { title, description: text })}
            close={() => setEditing(false)}
          />
        ) : (
          <>
            <h2>{t.title}</h2>
            <span className="ticket-head-actions">
              {ticketEditable(t, !!props.archived) && (
                <Button type="button"  onClick={() => setEditing(true)}>
                  Edit
                </Button>
              )}
              {retroable(t, !!props.archived) && props.openTicket && (
                <NewRetro target={t.id} onSubmitted={(r) => props.openTicket?.(r.id)} />
              )}
            </span>
          </>
        )}
      </header>
      <ActionPanel project={props.project} ticket={t} now={props.now} />
      <TicketBody {...props} ticket={t} editing={editing} />
    </>
  );
}

type TicketBodyProps = Omit<TicketDetailProps, "ticket" | "error"> & {
  ticket: TicketView;
  // editing is whether the pane's edit form is open, which holds the
  // description.
  editing: boolean;
};

// TicketBody is everything about a Ticket below its action panel: Spec,
// Labels, an Incoming Issue's triage actions (in the action panel instead
// while it is a New issue), description, Blockers (with a gated one's CI
// result, and whether it no longer blocks), its Auto-accept switch and the
// model and effort its next turn runs with and their pickers, its
// Repositories section while the Project has several, its CI section when it has CI Runs
// (in the action panel instead while CI failed), Claim activity with
// each Claim's Live section, move, and a collapsed history of moves,
// comments and edits merged by time.
function TicketBody({ project, ticket: t, specLabels, now, reported, openSpec, openTicket, archived, editing, onMoved }: TicketBodyProps) {
  const claims = [...(t.claims ?? [])].sort((a, b) => b.number - a.number);
  return (
    <article aria-label="Ticket details" className="ticket-body">
      <div className="meta">
        {t.spec.id && (
          <>
            Spec <SpecRef ticket={t} openSpec={openSpec} />
          </>
        )}
        {t.labels.map((l) => (
          <span key={l} className="label">
            {l}
          </span>
        ))}
        {t.link && <LinkBadge link={t.link} />}
        <PullRequestBadge pr={t.pull_request} />
        <OpenExternal link={t.link} />
      </div>
      {t.link && !t.link.detached && <LinkedNote key={t.id} project={project} ticket={t} />}
      {/* A New issue's triage actions are its action panel, so they
          aren't shown here too. */}
      {t.triage_actions && t.needs_me_reason !== "new_issue" && <Triage project={project} ticket={t} />}
      {editing ? null : t.description ? <Markdown>{t.description}</Markdown> : <p className="muted">No description.</p>}
      {t.tags.prototype !== undefined && <PrototypeAt tag={t.tags.prototype} />}
      {t.blockers.length > 0 && (
        <p className="muted small">
          Blocked by{" "}
          {t.blockers.map((b, i) => (
            <span key={b.id}>
              {i > 0 && ", "}
              <span className="tid" title={`${b.id} ${b.title}`}>
                {shortOf(b.id)}
              </span>{" "}
              ({b.state}
              {b.cleared && <> · no longer blocking</>})
            </span>
          ))}
        </p>
      )}
      {openTicket && <RetroList retros={t.retros} select={openTicket} />}
      {/* The controls share a grid, so each lines up after its label. */}
      <section aria-label="Ticket controls" className="ticket-controls">
        <TicketAutoAccept project={project} ticket={t} specLabels={specLabels} openSpec={openSpec} archived={archived} />
        <ModelChoice project={project} ticket={t} reported={reported} />
        <MoveControl key={`${t.id} ${t.state}`} project={project} ticket={t} onMoved={onMoved} />
      </section>
      <TicketRepositories ticket={t} />
      {/* A Ticket in a CI item has the CI section in its action panel,
          with the failure options, so it isn't shown here too. */}
      {!isCIItemReason(t.needs_me_reason) && (
        <CISection ticket={t} project={project} />
      )}
      {claims.length > 0 && (
        <ul aria-label="Claims" className="claims muted small">
          {claims.map((c) => (
            <li key={c.number}>
              <div className="claim-line">
                Claim {c.number} · started {ago(now, c.started)} · last activity {ago(now, c.last_activity)}
                {c.latest_event && (
                  <>
                    {" "}
                    · <LatestEventLine event={c.latest_event} now={now} />
                  </>
                )}
                {c.ended && (
                  <>
                    {" "}
                    · ended{c.ended_by && <> by {c.ended_by}</>} {ago(now, c.ended)}
                  </>
                )}
                {" "}
                · model {c.model || CLAUDE_CODE_DEFAULT} · effort {c.effort || CLAUDE_CODE_DEFAULT}
                {c.session_id && <> · session {c.session_id}</>}
              </div>
              <Live
                key={`${t.id}#${c.number}`}
                project={project}
                id={t.id}
                claim={c.number}
                defaultOpen={!c.ended && t.state === "In Progress"}
              />
            </li>
          ))}
        </ul>
      )}
      <History ticket={t} now={now} />
    </article>
  );
}

// TicketAutoAccept is the Ticket's Auto-accept switch, offered unless the
// Ticket can't be auto-accepted or is Accepted, Done or Cancelled. It
// reflects only the Ticket's own Label; when its Spec carries the Label, a
// note says so. It waits for the Spec's Labels.
function TicketAutoAccept({ project, ticket: t, specLabels, openSpec, archived }: Pick<TicketBodyProps, "project" | "ticket" | "specLabels" | "openSpec" | "archived">) {
  if (!specLabels || !canAutoAccept(t, specLabels) || t.state === "Accepted" || t.state === "Done" || t.state === "Cancelled") return null;
  return (
    <AutoAcceptSwitch
      project={project}
      target={t.id}
      on={t.labels.includes(AUTO_ACCEPT_LABEL)}
      accepts={t.state === "In Review" && wouldAutoAccept(t, specLabels) ? 1 : 0}
      archived={archived}
      viaSpec={
        specLabels.includes(AUTO_ACCEPT_LABEL) && (
          <>
            Auto-accepted via its Spec's Label: <SpecRef ticket={t} openSpec={openSpec} />
          </>
        )
      }
    />
  );
}

// SpecRef is the Ticket's Spec by its Short ID, always in the Ticket's
// Project, with its title on hover; a link where openSpec can open it.
function SpecRef({ ticket: t, openSpec }: { ticket: TicketView; openSpec?(spec: string): void }) {
  const name = `${t.spec.id} ${t.spec.title}`;
  return openSpec ? (
    <button className="link" title={name} aria-label={name} onClick={() => openSpec(t.spec.id)}>
      {shortOf(t.spec.id)}
    </button>
  ) : (
    <span className="tid" title={name}>
      {shortOf(t.spec.id)}
    </span>
  );
}

// PrototypeAt is where a Prototype is, from the prototype Tag its agent
// sets as "<branch>@<commit> <worktree path>": the branch has no @ or
// space, and the path is everything after the first space. A Tag that
// doesn't parse is shown as it is.
function PrototypeAt({ tag }: { tag: string }) {
  const m = /^([^@\s]+)@(\S+) (.+)$/.exec(tag);
  return (
    <p className="muted small">
      Prototype at{" "}
      {m ? (
        <>
          <code>{m[3]}</code> on <code>{m[1]}</code> (<code>{m[2]}</code>)
        </>
      ) : (
        <code>{tag}</code>
      )}
    </p>
  );
}

// LinkedNote says a linked Ticket's definition belongs to its Integration
// (ticket 42) and offers Unlink, which hands it back to Code Central.
function LinkedNote({ project, ticket: t }: { project: string; ticket: TicketView }) {
  const api = useApi();
  const { busy, error, run } = useAct();
  return (
    <div className="linked-note muted small">
      The title and description are read-only while linked: change them in {t.link!.integration}.{" "}
      <Button type="button"  disabled={busy} onClick={() => run(() => api.unlink(project, t.id))}>
        Unlink
      </Button>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </div>
  );
}

function MoveControl({ project, ticket: t, onMoved }: { project: string; ticket: TicketView; onMoved?(): void }) {
  const api = useApi();
  const targets = moveTargets(t.state);
  const [to, setTo] = useState<State>(targets[0]);
  const { busy, error, run } = useAct();
  return (
    <form
      className="move"
      onSubmit={(e) => {
        e.preventDefault();
        void run(() => api.act(project, t.id, "move", { to_state: to })).then((ok) => ok && onMoved?.());
      }}
    >
      <span>Move to</span>
      <span className="ticket-control">
        <Select aria-label="Move to" value={to} onChange={(v) => setTo(v as State)} options={targets.map((s) => ({ value: s }))} />
        <Button  disabled={busy}>
          Move
        </Button>
      </span>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </form>
  );
}

type HistoryItem =
  | { kind: "text"; time: string; key: string; text: string }
  | { kind: "edit"; time: string; key: string; edit: EditView };

function History({ ticket: t, now }: { ticket: TicketView; now: Date }) {
  const { moves = [], comments = [], edits = [] } = t;
  const items: HistoryItem[] = [
    ...moves.map((m, i): HistoryItem => ({
      kind: "text",
      time: m.time,
      key: `m${i}`,
      text: moveText(m, t),
    })),
    ...comments.map((c): HistoryItem => ({
      kind: "text",
      time: c.time,
      key: `c${c.number}`,
      text: `${c.actor}: ${c.body.split("\n")[0]}`,
    })),
    ...edits.map((e): HistoryItem => ({ kind: "edit", time: e.time, key: `e${e.number}`, edit: e })),
  ].sort((a, b) => b.time.localeCompare(a.time));
  return (
    <details>
      <summary>
        History ({plural(moves.length, "move")}, {plural(comments.length, "comment")},{" "}
        {plural(edits.length, "edit")})
      </summary>
      <ul className="history">
        {items.map((h) => (
          <li key={h.key}>
            <span className="muted">{ago(now, h.time)}</span> {h.kind === "edit" ? <Edit edit={h.edit} /> : h.text}
          </li>
        ))}
      </ul>
    </details>
  );
}

const EDIT_FIELD_ORDER = ["title", "description", "body"];
const MARKDOWN_FIELDS = new Set(["description", "body"]);

// Edit reads "description edited by agent" and shows the text it replaced
// on demand. Undoing an edit is done by hand: there is no restore.
function Edit({ edit }: { edit: EditView }) {
  const [open, setOpen] = useState(false);
  const fields = Object.keys(edit.previous).sort((a, b) => EDIT_FIELD_ORDER.indexOf(a) - EDIT_FIELD_ORDER.indexOf(b));
  return (
    <>
      <span>
        {fields.join(" and ")} edited by {edit.actor}
      </span>{" "}
      <button type="button" className="link" onClick={() => setOpen(!open)}>
        {open ? "hide previous" : "show previous"}
      </button>
      {open && (
        <div className="previous">
          {fields.map((f) => (
            <div key={f}>
              {fields.length > 1 && <strong>{f[0].toUpperCase() + f.slice(1)}: </strong>}
              {MARKDOWN_FIELDS.has(f) ? <Markdown>{edit.previous[f]}</Markdown> : edit.previous[f]}
            </div>
          ))}
        </div>
      )}
    </>
  );
}

function plural(n: number, word: string) {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}
