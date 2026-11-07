// The selected Ticket, shared by every view: its header, the action panel
// slot first, then the Ticket itself.
import { useState } from "react";
import type { Api } from "../api/api";
import { STATES, type EditView, type State, type TicketView } from "../api/types";
import { ago, REASON_LABEL } from "../format";
import { ActionPanel } from "../panels/ActionPanel";
import { useAct } from "./useAct";

export interface TicketPaneProps {
  api: Api;
  project: string;
  ticket: TicketView;
  now: Date;
}

export function TicketPane(props: TicketPaneProps) {
  const t = props.ticket;
  return (
    <>
      <header className="ticket-head">
        <span className="tid">{t.id}</span> <span className="state">{t.state}</span>
        {t.needs_me_reason && (
          <span className={`reason reason-${t.needs_me_reason}`}>
            <span className="dot" aria-hidden="true" /> {REASON_LABEL[t.needs_me_reason]}
          </span>
        )}
        <h2>{t.title}</h2>
      </header>
      <ActionPanel {...props} />
      <TicketDetail {...props} />
    </>
  );
}

export type TicketDetailProps = TicketPaneProps;

// TicketDetail is everything about a Ticket below its action panel: Spec,
// Labels, description, Blockers, Claim activity, move, and a collapsed
// history of moves, comments and edits merged by time.
export function TicketDetail({ api, project, ticket: t, now }: TicketDetailProps) {
  const claims = [...t.claims].sort((a, b) => b.number - a.number);
  return (
    <article aria-label="Ticket details" className="ticket-body">
      <div className="meta">
        {t.spec.id && (
          <span>
            {t.spec.id} {t.spec.title}
          </span>
        )}
        {t.labels.map((l) => (
          <span key={l} className="label">
            {l}
          </span>
        ))}
      </div>
      {t.description ? <div className="description">{t.description}</div> : <p className="muted">No description.</p>}
      {t.blockers.length > 0 && (
        <p className="muted small">
          Blocked by{" "}
          {t.blockers.map((b, i) => (
            <span key={b.id}>
              {i > 0 && ", "}
              {b.id} {b.title} ({b.state})
            </span>
          ))}
        </p>
      )}
      {claims.length > 0 && (
        <ul aria-label="Claims" className="claims muted small">
          {claims.map((c) => (
            <li key={c.number}>
              Claim {c.number} · started {ago(now, c.started)} · last activity {ago(now, c.last_activity)}
              {c.ended && (
                <>
                  {" "}
                  · ended{c.ended_by && <> by {c.ended_by}</>} {ago(now, c.ended)}
                </>
              )}
              {c.session_id && <> · session {c.session_id}</>}
            </li>
          ))}
        </ul>
      )}
      <MoveControl key={`${t.id} ${t.state}`} api={api} project={project} ticket={t} />
      <History ticket={t} now={now} />
    </article>
  );
}

// The human moves a Ticket to any State but its own and the two entered
// only by posting a Round (Needs Answer) or a Failure Report (Failed).
const NOT_MOVABLE: readonly State[] = ["Needs Answer", "Failed"];

function MoveControl({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const targets = STATES.filter((s) => s !== t.state && !NOT_MOVABLE.includes(s));
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
            <div key={f} className="description">
              {fields.length > 1 && <strong>{f[0].toUpperCase() + f.slice(1)}: </strong>}
              {edit.previous[f]}
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
