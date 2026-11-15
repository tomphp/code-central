// SessionsTable is Usage's Recent sessions table, with the Subagent
// disclosures whose state it keeps.
import { Fragment, useState } from "react";
import { sessionKey, type SessionUsageFilter, type UsageSession } from "../api/types";
import { count } from "../format";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { TokenCount, Tokens } from "./TokenCount";

export interface SessionsTableProps {
  sessions: UsageSession[];
  // here is Usage's address, the session open on the line chart among it.
  here: UiLocation;
  projectName(key: string): string;
  navigate(patch: Partial<UiLocation>): void;
}

// SessionsTable is the recent sessions, newest first, each a Claim's main
// session with a disclosure, collapsed until opened, for its Subagents.
// Each row's Ticket links to that Ticket and its session opens its line
// chart. Which disclosures are open is the table's own, not the address's.
export function SessionsTable({ sessions, here, projectName, navigate }: SessionsTableProps) {
  // A Subagent's chart in the address starts its parent's disclosure open.
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() =>
    new Set(here.session?.agent ? [sessionKey({ project: here.session.project, session: here.session.session })] : []),
  );
  const toggle = (key: string) =>
    setExpanded((open) => {
      const next = new Set(open);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  return (
    <>
      <h3 className="usage-subhead">Recent sessions</h3>
      <table aria-label="Recent sessions" className="data-table data-table-mono">
        <thead>
          <tr>
            <th scope="col">Ticket</th>
            <th scope="col">Project</th>
            <th scope="col" className="num">Claim</th>
            <th scope="col">Session</th>
            <th scope="col">Kind</th>
            <th scope="col">Model</th>
            <th scope="col">Started</th>
            <th scope="col" className="num">Peak Context Size</th>
            <th scope="col" className="num">Tokens</th>
            <th scope="col" className="num">Responses</th>
          </tr>
        </thead>
        <tbody>
          {sessions.map((s) => {
            const key = sessionKey({ project: s.project, session: s.session_id });
            const open = expanded.has(key);
            return (
              <Fragment key={key}>
                <SessionRow s={s} here={here} projectName={projectName} navigate={navigate} open={open} onToggle={() => toggle(key)} />
                {open && s.subagents.map((a) => <SessionRow key={a.agent} s={a} here={here} projectName={projectName} navigate={navigate} />)}
              </Fragment>
            );
          })}
        </tbody>
      </table>
    </>
  );
}

interface SessionRowProps {
  s: UsageSession;
  here: UiLocation;
  projectName(key: string): string;
  navigate(patch: Partial<UiLocation>): void;
  // open is whether a main session's Subagents show, and onToggle shows
  // or hides them; a Subagent's row has neither.
  open?: boolean;
  onToggle?(): void;
}

// SessionRow is one recent session's row: a main session's, with its
// tokens and its Subagents' total and the disclosure for them, or a
// Subagent's, indented under it.
function SessionRow({ s, here, projectName, navigate, open, onToggle }: SessionRowProps) {
  const to: Partial<UiLocation> = { view: "inbox", project: s.project, ticket: s.ticket };
  const session: SessionUsageFilter = { project: s.project, session: s.session_id, ...(s.agent ? { agent: s.agent } : {}) };
  const sessionTo: Partial<UiLocation> = { session };
  const isOpen = here.session !== undefined && sessionKey(here.session) === sessionKey(session);
  const n = s.subagents.length;
  const className = [s.agent ? "usage-subagent" : "", isOpen ? "usage-open" : ""].filter(Boolean).join(" ");
  return (
    <tr className={className || undefined}>
      <th scope="row">
        <a href={hrefFor({ view: "inbox", ...to })} onClick={clickTo(navigate, to)}>
          {s.ticket}
        </a>
      </th>
      <td>{projectName(s.project)}</td>
      <td className="num">{s.claim}</td>
      <td className="usage-session">
        <a href={hrefFor({ ...here, ...sessionTo })} aria-current={isOpen ? "true" : undefined} onClick={clickTo(navigate, sessionTo)}>
          {s.agent ? subagentName(s) : s.session_id}
        </a>
        {onToggle && n > 0 && (
          <button type="button" className="link usage-disclosure" aria-expanded={open} onClick={onToggle}>
            <span aria-hidden="true">{open ? "▾" : "▸"}</span> {n} {n === 1 ? "subagent" : "subagents"}
          </button>
        )}
      </td>
      <td>{s.kind || "—"}</td>
      <td>{s.models.length > 0 ? s.models.join(", ") : "—"}</td>
      <td title={s.started}>{started(s.started)}</td>
      <Tokens n={s.peak} />
      {n ? (
        <td className="num">
          <TokenCount n={s.tokens} /> (<TokenCount n={s.total_tokens ?? s.tokens} /> with subagents)
        </td>
      ) : (
        <Tokens n={s.tokens} />
      )}
      <td className="num">{count(s.responses)}</td>
    </tr>
  );
}

// started is a session's start in local time, as short as the locale
// puts it: "9 Nov, 14:25".
export function started(at: string): string {
  return new Date(at).toLocaleString([], { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

// subagentName is a Subagent's name: its Agent Type and description, as
// "Explore · Map Usage sessions", or whichever of them it has, or else
// its tool-use ID, agent.
export function subagentName({ agent, agent_type, description }: Pick<UsageSession, "agent" | "agent_type" | "description">): string {
  return [agent_type, description].filter(Boolean).join(" · ") || (agent ?? "");
}
