// UsageBody is Usage's page below its header, presentational: the figures
// the view read, the sort of the calls table and the open session's chart
// come in as props, and the view fetches them.
import { Fragment, useState, type ReactNode } from "react";
import { sessionKey, type SessionUsageFilter, type UsageKind, type UsageOperation, type UsageSession, type UsageView } from "../api/types";
import { Empty, Loading } from "../controls/Empty";
import { ErrorBanner } from "../controls/ErrorBanner";
import { bytes, count } from "../format";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { TokenChart } from "./TokenChart";
import "./usage.css";

export interface Sort {
  key: keyof UsageOperation;
  descending: boolean;
}

export const DEFAULT_SORT: Sort = { key: "calls", descending: true };

// nextSort is the sort after a click on a column's header. A first click
// sorts numbers largest first and operations A to Z; a second reverses it.
export function nextSort(s: Sort, key: keyof UsageOperation): Sort {
  return s.key === key ? { key, descending: !s.descending } : { key, descending: key !== "op" };
}

export interface UsageBodyProps {
  // data is what the read returned; left out while it loads.
  data?: UsageView;
  // error is the read's failure, if it failed.
  error?: string;
  days: number;
  // scopeName is the Project's name, or "All Projects".
  scopeName: string;
  // here is Usage's address, the session open on the line chart among it.
  here: UiLocation;
  projectName(key: string): string;
  navigate(patch: Partial<UiLocation>): void;
  sort: Sort;
  onSort(key: keyof UsageOperation): void;
  // session is the open session's chart, shown above the rest.
  session?: ReactNode;
}

export function UsageBody({ data, error, days, scopeName, here, projectName, navigate, sort, onSort, session }: UsageBodyProps) {
  const empty = data && !data.operations.length && !data.kinds.length && !data.sessions.length;
  const over = `in ${scopeName} over the last ${days} days`;
  return (
    <div className="view-body">
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {!data && !error && <Loading />}
      {empty && <Empty>No usage yet {over}.</Empty>}
      {session}
      {data && !empty && (
        <>
          <section aria-labelledby="usage-calls">
            <h2 id="usage-calls">Code Central calls</h2>
            {data.operations.length ? (
              <CallsTable operations={data.operations} sort={sort} onSort={onSort} />
            ) : (
              <Empty>No Code Central calls {over}.</Empty>
            )}
          </section>
          <section aria-labelledby="usage-context">
            <h2 id="usage-context">Context Size</h2>
            {data.sessions.length ? (
              <>
                <KindsTable kinds={data.kinds} />
                <SessionsTable sessions={data.sessions} here={here} projectName={projectName} navigate={navigate} />
              </>
            ) : (
              <Empty>No sessions {over}. Only sessions the Factory runs are counted.</Empty>
            )}
          </section>
          <section aria-labelledby="usage-tokens">
            <h2 id="usage-tokens">Token usage</h2>
            {data.kinds.length ? (
              <TokenChart days={data.days} kinds={data.kinds.map((k) => k.kind)} />
            ) : (
              <Empty>No token usage {over}. Only sessions the Factory runs are counted.</Empty>
            )}
          </section>
        </>
      )}
    </div>
  );
}

// A Column is one column of the calls table: its header, how its cells
// read, and the unit their exact figure is given in on hover.
interface Column {
  key: keyof UsageOperation;
  label: string;
  unit?: "calls" | "bytes" | "errors";
}

const COLUMNS: Column[] = [
  { key: "op", label: "Operation" },
  { key: "calls", label: "Calls", unit: "calls" },
  { key: "request_bytes", label: "Request total", unit: "bytes" },
  { key: "average_request_bytes", label: "Request average", unit: "bytes" },
  { key: "response_bytes", label: "Response total", unit: "bytes" },
  { key: "average_response_bytes", label: "Response average", unit: "bytes" },
  { key: "errors", label: "Errors", unit: "errors" },
];

interface CallsTableProps {
  operations: UsageOperation[];
  sort: Sort;
  onSort(key: keyof UsageOperation): void;
}

// CallsTable is the Code Central calls table in the order sort gives; a
// click on a header calls onSort with its column.
function CallsTable({ operations, sort, onSort }: CallsTableProps) {
  const rows = [...operations].sort((a, b) => {
    const x = a[sort.key];
    const y = b[sort.key];
    const order = typeof x === "string" ? x.localeCompare(y as string) : x - (y as number);
    return sort.descending ? -order : order;
  });

  return (
    <table aria-label="Code Central calls" className="data-table data-table-mono">
      <thead>
        <tr>
          {COLUMNS.map((c) => (
            <th
              key={c.key}
              scope="col"
              className={c.unit ? "num" : undefined}
              aria-sort={sort.key === c.key ? (sort.descending ? "descending" : "ascending") : undefined}
            >
              <button className="usage-sort" onClick={() => onSort(c.key)}>
                {c.label}
              </button>
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.op}>
            <th scope="row">{r.op}</th>
            {COLUMNS.slice(1).map((c) => {
              const n = r[c.key] as number;
              return (
                <td key={c.key} className="num" title={`${count(n)} ${c.unit}`}>
                  {c.unit === "bytes" ? bytes(n) : count(n)}
                </td>
              );
            })}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// Tokens is a token count's cell, with thousands separators and the
// exact figure, in tokens, on hover.
function Tokens({ n }: { n: number }) {
  return (
    <td className="num">
      <TokenCount n={n} />
    </td>
  );
}

// TokenCount is a token count with thousands separators and the exact
// figure, in tokens, on hover.
function TokenCount({ n }: { n: number }) {
  return <span title={`${count(n)} tokens`}>{count(n)}</span>;
}

// KindsTable is Context Size by kind of session: how many sessions there
// were, and the average and largest of their peaks.
function KindsTable({ kinds }: { kinds: UsageKind[] }) {
  return (
    <table aria-label="Context Size by kind" className="data-table data-table-mono">
      <thead>
        <tr>
          <th scope="col">Kind</th>
          <th scope="col" className="num">Sessions</th>
          <th scope="col" className="num">Average peak</th>
          <th scope="col" className="num">Largest peak</th>
        </tr>
      </thead>
      <tbody>
        {kinds.map((k) => (
          <tr key={k.kind}>
            <th scope="row">{k.kind}</th>
            <td className="num">{count(k.sessions)}</td>
            <Tokens n={k.average_peak} />
            <Tokens n={k.largest_peak} />
          </tr>
        ))}
      </tbody>
    </table>
  );
}

interface SessionsTableProps {
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
function SessionsTable({ sessions, here, projectName, navigate }: SessionsTableProps) {
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
