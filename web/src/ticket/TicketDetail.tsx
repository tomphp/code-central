// The selected Ticket, shared by every view: its header, the action panel
// slot first, then the Ticket itself.
import { useState } from "react";
import type { Api } from "../api/api";
import { moveTargets } from "../api/actions/move";
import type { EditView, ReportedModels, State, TicketView } from "../api/types";
import { ago, CLAUDE_CODE_DEFAULT, REASON_LABEL } from "../format";
import { Markdown } from "../Markdown";
import { EditForm } from "../edit/EditForm";
import { ticketEditable } from "../edit/editable";
import { ActionPanel } from "../panels/ActionPanel";
import { LatestEventLine } from "../LatestEvent";
import { LinkBadge } from "./LinkBadge";
import { OpenExternal } from "./OpenExternal";
import { Live } from "./Live";
import { WaitingBadge } from "./WaitingBadge";
import { useAct } from "./useAct";
import { CISection } from "./CISection";
import { Triage } from "./Triage";
import { ModelChoice } from "./ModelChoice";

export interface TicketPaneProps {
  api: Api;
  project: string;
  ticket: TicketView;
  now: Date;
  // reported is the Project's Reported Models, once read: what the Model
  // and effort pickers offer, and the Factory's flags.
  reported?: ReportedModels;
  // openSpec opens the Ticket's Spec, where the view can show it; without
  // it the Spec is named only.
  openSpec?(spec: string): void;
  // archived is whether the Ticket's Project is Archived, which takes no
  // edits.
  archived?: boolean;
}

export function TicketPane(props: TicketPaneProps) {
  const t = props.ticket;
  // The form owns its draft, so it stays open if the Ticket stops being
  // editable under it; Edit is offered only while it is closed.
  const [editing, setEditing] = useState(false);
  return (
    <>
      <header className="ticket-head">
        <span className="tid">{t.id}</span> <span className="state">{t.state}</span>
        <WaitingBadge waiting={t.waiting_for_spec} />
        {t.needs_me_reason && (
          <span className={`reason reason-${t.needs_me_reason}`}>
            <span className="dot" aria-hidden="true" /> {REASON_LABEL[t.needs_me_reason]}
          </span>
        )}
        {editing ? (
          <EditForm
            title={t.title}
            text={t.description}
            label="description"
            save={({ title, text }) => props.api.act(props.project, t.id, "edit_ticket", { title, description: text })}
            close={() => setEditing(false)}
          />
        ) : (
          <>
            <h2>{t.title}</h2>
            {ticketEditable(t, !!props.archived) && (
              <button type="button" className="btn" onClick={() => setEditing(true)}>
                Edit
              </button>
            )}
          </>
        )}
      </header>
      <ActionPanel {...props} />
      <TicketDetail {...props} editing={editing} />
    </>
  );
}

export type TicketDetailProps = TicketPaneProps & {
  // editing is whether the pane's edit form is open, which holds the
  // description.
  editing?: boolean;
};

// TicketDetail is everything about a Ticket below its action panel: Spec,
// Labels, an Incoming Issue's triage actions (in the action panel instead
// while it is a New issue), description, Blockers (with a gated one's CI
// result, and whether it no longer blocks), the model and effort its next
// turn runs with and their pickers, its CI section when it has CI Runs
// (in the action panel instead while CI failed), Claim activity with
// each Claim's Live section, move, and a collapsed history of moves,
// comments and edits merged by time.
export function TicketDetail({ api, project, ticket: t, now, reported, openSpec, editing }: TicketDetailProps) {
  const claims = [...t.claims].sort((a, b) => b.number - a.number);
  const specName = `${t.spec.id} ${t.spec.title}`;
  return (
    <article aria-label="Ticket details" className="ticket-body">
      <div className="meta">
        {t.spec.id &&
          (openSpec ? (
            <button className="link" onClick={() => openSpec(t.spec.id)}>
              {specName}
            </button>
          ) : (
            <span>{specName}</span>
          ))}
        {t.labels.map((l) => (
          <span key={l} className="label">
            {l}
          </span>
        ))}
        {t.link && <LinkBadge link={t.link} />}
        <OpenExternal link={t.link} />
      </div>
      {t.link && !t.link.detached && <LinkedNote key={t.id} api={api} project={project} ticket={t} />}
      {/* A New issue's triage actions are its action panel, so they
          aren't shown here too. */}
      {t.triage_actions && t.needs_me_reason !== "new_issue" && <Triage api={api} project={project} ticket={t} />}
      {editing ? null : t.description ? <Markdown>{t.description}</Markdown> : <p className="muted">No description.</p>}
      {t.tags.prototype !== undefined && <PrototypeAt tag={t.tags.prototype} />}
      {t.blockers.length > 0 && (
        <p className="muted small">
          Blocked by{" "}
          {t.blockers.map((b, i) => (
            <span key={b.id}>
              {i > 0 && ", "}
              {b.id} {b.title} ({b.state}
              {b.ci_result && <> · CI {b.ci_result}</>}
              {b.cleared && <> · no longer blocking</>})
            </span>
          ))}
        </p>
      )}
      <ModelChoice api={api} project={project} ticket={t} reported={reported} />
      {/* A CI failed Ticket's CI section is its action panel, with the
          failure options, so it isn't shown here too. */}
      {t.needs_me_reason !== "ci_failed" && t.needs_me_reason !== "ci_cancelled" && <CISection ticket={t} api={api} project={project} />}
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
                api={api}
                project={project}
                id={t.id}
                claim={c.number}
                defaultOpen={!c.ended && t.state === "In Progress"}
              />
            </li>
          ))}
        </ul>
      )}
      <MoveControl key={`${t.id} ${t.state}`} api={api} project={project} ticket={t} />
      <History ticket={t} now={now} />
    </article>
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
function LinkedNote({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const { busy, error, run } = useAct();
  return (
    <div className="linked-note muted small">
      The title and description are read-only while linked: change them in {t.link!.integration}.{" "}
      <button type="button" className="btn" disabled={busy} onClick={() => run(() => api.unlink(project, t.id))}>
        Unlink
      </button>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </div>
  );
}

function MoveControl({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const targets = moveTargets(t.state);
  const [to, setTo] = useState<State>(targets[0]);
  const { busy, error, run } = useAct();
  return (
    <form
      className="move"
      onSubmit={(e) => {
        e.preventDefault();
        run(() => api.act(project, t.id, "move", { to_state: to }));
      }}
    >
      <label>
        <span className="muted small">Move to</span>{" "}
        <select value={to} onChange={(e) => setTo(e.target.value as State)}>
          {targets.map((s) => (
            <option key={s}>{s}</option>
          ))}
        </select>
      </label>{" "}
      <button className="btn" disabled={busy}>
        Move
      </button>
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
  const items: HistoryItem[] = [
    ...t.moves.map((m, i): HistoryItem => ({
      kind: "text",
      time: m.time,
      key: `m${i}`,
      text: `${m.from ? `${m.from} → ` : ""}${m.to} by ${m.actor}`,
    })),
    ...t.comments.map((c): HistoryItem => ({
      kind: "text",
      time: c.time,
      key: `c${c.number}`,
      text: `${c.actor}: ${c.body.split("\n")[0]}`,
    })),
    ...t.edits.map((e): HistoryItem => ({ kind: "edit", time: e.time, key: `e${e.number}`, edit: e })),
  ].sort((a, b) => b.time.localeCompare(a.time));
  return (
    <details>
      <summary>
        History ({plural(t.moves.length, "move")}, {plural(t.comments.length, "comment")},{" "}
        {plural(t.edits.length, "edit")})
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
