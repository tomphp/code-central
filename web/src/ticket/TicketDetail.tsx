// The selected Ticket, read-only, shared by every view: its header, the
// action panel slot first, then the Ticket itself.
import type { Api } from "../api/api";
import type { TicketView } from "../api/types";
import { ago, REASON_LABEL } from "../format";
import { ActionPanel } from "../panels/ActionPanel";

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
      <TicketDetail ticket={t} now={props.now} />
    </>
  );
}

export function TicketDetail({ ticket: t, now }: { ticket: TicketView; now: Date }) {
  const claim = t.claims.find((c) => !c.ended);
  const history = [
    ...t.moves.map((m) => ({ time: m.time, text: `${m.from ? `${m.from} → ` : ""}${m.to} by ${m.actor}` })),
    ...t.comments.map((c) => ({ time: c.time, text: `${c.actor}: ${c.body.split("\n")[0]}` })),
  ].sort((a, b) => b.time.localeCompare(a.time));
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
      {claim && (
        <p className="muted small">
          Claimed {ago(now, claim.started)} · last activity {ago(now, claim.last_activity)}
          {claim.session_id && <> · session {claim.session_id}</>}
        </p>
      )}
      <details>
        <summary>
          History ({t.moves.length} moves, {t.comments.length} comments)
        </summary>
        <ul className="history">
          {history.map((h, i) => (
            <li key={i}>
              <span className="muted">{ago(now, h.time)}</span> {h.text}
            </li>
          ))}
        </ul>
      </details>
    </article>
  );
}
