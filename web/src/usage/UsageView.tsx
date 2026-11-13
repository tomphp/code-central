// Usage (Spec code-central-201): what agents have consumed in one
// Project, or in all of them, over the last 7, 30 or 90 days: the Code
// Central calls table, one row per MCP operation, sortable by each
// column; and Context Size, each kind of session's peaks and the recent
// sessions, each linking to its Ticket and opening its Context Size,
// response by response, on a line chart; and token usage, a bar per day
// filtered by kind of session. It loads when opened and whenever its
// scope, range or session changes, never live. Back returns to the view
// it was opened from.
import { useEffect, useRef, useState } from "react";
import type { Api } from "../api/api";
import { ProjectPicker } from "../controls/ProjectPicker";
import { ViewHeader } from "../controls/ViewHeader";
import {
  sessionKey,
  USAGE_DAYS,
  USAGE_KEPT_DAYS,
  type ProjectRow,
  type UsageDays,
  type UsageKind,
  type UsageOperation,
  type UsageSession,
  type SessionUsageFilter,
  type SessionUsageResponse,
} from "../api/types";
import { bytes, count } from "../format";
import { useFetch } from "../live";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { LineChart } from "./LineChart";
import { TokenChart } from "./TokenChart";
import "./usage.css";

export interface UsageViewProps {
  api: Api;
  // projects are every Project, Archived ones too, for the scope picker.
  projects?: ProjectRow[];
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function UsageView({ api, projects = [], loc, navigate }: UsageViewProps) {
  const days = loc.days ?? USAGE_DAYS[0];
  const usage = useFetch(() => api.usage({ project: loc.project, days }), [api, loc.project, days]);
  const back = loc.back?.view === "focus" ? "focus" : "inbox";
  const scopeName = loc.project ? (projects.find((p) => p.key === loc.project)?.display_name ?? loc.project) : "All Projects";
  // Rows answer the current scope and range only once they've loaded.
  const data = usage.data;
  const empty = data && !data.operations.length && !data.kinds.length && !data.sessions.length;
  const over = `in ${scopeName} over the last ${days} days`;
  const projectName = (key: string) => projects.find((p) => p.key === key)?.display_name ?? key;

  return (
    <main aria-label="Usage" className="usage">
      <ViewHeader
        title="Usage"
        left={
          <>
            <ProjectPicker projects={projects} value={loc.project} onChange={(project) => navigate({ project })} />
            <label>
          <span className="view-picker-label">Range</span>
          <select value={days} onChange={(e) => navigate({ days: Number(e.target.value) as UsageDays })}>
            {USAGE_DAYS.map((d) => (
              <option key={d} value={d}>
                Last {d} days
              </option>
            ))}
          </select>
        </label>
          </>
        }
        right={
          <button className="btn" onClick={() => navigate(loc.back ?? { view: "inbox" })}>
            {back === "focus" ? "Back to Focus Mode" : "Back to the Inbox"}
          </button>
        }
      />
      <div className="usage-body">
        {usage.error && (
          <p role="alert" className="warning">
            {usage.error.message}
          </p>
        )}
        {empty && <p className="muted">No usage yet {over}.</p>}
        {/* The open session, above the rest so it shows whatever they hold,
            keyed so another session starts from nothing. */}
        {loc.session && <SessionChart key={sessionKey(loc.session)} api={api} session={loc.session} navigate={navigate} />}
        {data && !empty && (
          <>
            <section aria-labelledby="usage-calls">
              <h2 id="usage-calls">Code Central calls</h2>
              {data.operations.length ? <CallsTable operations={data.operations} /> : <p className="muted">No Code Central calls {over}.</p>}
            </section>
            <section aria-labelledby="usage-context">
              <h2 id="usage-context">Context Size</h2>
              {data.sessions.length ? (
                <>
                  <KindsTable kinds={data.kinds} />
                  <SessionsTable sessions={data.sessions} here={loc} projectName={projectName} navigate={navigate} />
                </>
              ) : (
                <p className="muted">No sessions {over}. Only sessions the Factory runs are counted.</p>
              )}
            </section>
            <section aria-labelledby="usage-tokens">
              <h2 id="usage-tokens">Token usage</h2>
              {data.kinds.length ? (
                <TokenChart days={data.days} kinds={data.kinds.map((k) => k.kind)} />
              ) : (
                <p className="muted">No token usage {over}. Only sessions the Factory runs are counted.</p>
              )}
            </section>
          </>
        )}
      </div>
    </main>
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

interface Sort {
  key: keyof UsageOperation;
  descending: boolean;
}

// CallsTable is the Code Central calls table, most calls first until the
// human sorts by another column. A first click sorts numbers largest
// first and operations A to Z; a second reverses it.
function CallsTable({ operations }: { operations: UsageOperation[] }) {
  const [sort, setSort] = useState<Sort>({ key: "calls", descending: true });
  const rows = [...operations].sort((a, b) => {
    const x = a[sort.key];
    const y = b[sort.key];
    const order = typeof x === "string" ? x.localeCompare(y as string) : x - (y as number);
    return sort.descending ? -order : order;
  });
  const sortBy = (key: keyof UsageOperation) =>
    setSort((s) => (s.key === key ? { key, descending: !s.descending } : { key, descending: key !== "op" }));

  return (
    <table aria-label="Code Central calls" className="usage-table">
      <thead>
        <tr>
          {COLUMNS.map((c) => (
            <th
              key={c.key}
              scope="col"
              className={c.unit ? "num" : undefined}
              aria-sort={sort.key === c.key ? (sort.descending ? "descending" : "ascending") : undefined}
            >
              <button className="usage-sort" onClick={() => sortBy(c.key)}>
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
    <td className="num" title={`${count(n)} tokens`}>
      {count(n)}
    </td>
  );
}

// KindsTable is Context Size by kind of session: how many sessions there
// were, and the average and largest of their peaks.
function KindsTable({ kinds }: { kinds: UsageKind[] }) {
  return (
    <table aria-label="Context Size by kind" className="usage-table">
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

// SessionsTable is the recent sessions, newest first, each a Claim's
// main session or one of its subagents, its Ticket a link to that Ticket
// and its session a link that opens its line chart.
function SessionsTable({ sessions, here, projectName, navigate }: SessionsTableProps) {
  return (
    <>
      <h3 className="usage-subhead">Recent sessions</h3>
      <table aria-label="Recent sessions" className="usage-table">
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
            <th scope="col" className="num">Responses</th>
          </tr>
        </thead>
        <tbody>
          {sessions.map((s) => {
            const to: Partial<UiLocation> = { view: "inbox", project: s.project, ticket: s.ticket };
            const session: SessionUsageFilter = { project: s.project, session: s.session_id, ...(s.agent ? { agent: s.agent } : {}) };
            const sessionTo: Partial<UiLocation> = { session };
            const isOpen = here.session !== undefined && sessionKey(here.session) === sessionKey(session);
            return (
              <tr key={`${s.session_id} ${s.agent ?? ""}`} className={isOpen ? "usage-open" : undefined}>
                <th scope="row">
                  <a href={hrefFor({ view: "inbox", ...to })} onClick={clickTo(navigate, to)}>
                    {s.ticket}
                  </a>
                </th>
                <td>{projectName(s.project)}</td>
                <td className="num">{s.claim}</td>
                <td className="usage-session">
                  <a
                    href={hrefFor({ ...here, ...sessionTo })}
                    aria-current={isOpen ? "true" : undefined}
                    onClick={clickTo(navigate, sessionTo)}
                  >
                    {sessionName(s.session_id, s.agent)}
                  </a>
                </td>
                <td>{s.kind}</td>
                <td>{s.models.length > 0 ? s.models.join(", ") : "—"}</td>
                <td title={s.started}>{started(s.started)}</td>
                <Tokens n={s.peak} />
                <td className="num">{count(s.responses)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </>
  );
}

// started is a session's start in local time, as short as the locale
// puts it: "9 Nov, 14:25".
function started(at: string): string {
  return new Date(at).toLocaleString([], { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

// sessionName is a session's ID, with its subagent's after it.
function sessionName(session: string, agent?: string): string {
  return agent ? `${session} · ${agent}` : session;
}

interface SessionChartProps {
  api: Api;
  session: SessionUsageFilter;
  navigate(patch: Partial<UiLocation>): void;
}

// SessionChart is one session's Context Size, response by response, as
// a line chart, each point's exact figures on hover. It reads every day
// Usage keeps, whatever the range, and closes to Usage as it was.
function SessionChart({ api, session, navigate }: SessionChartProps) {
  const read = useFetch(() => api.sessionUsage(session), [api, session.project, session.session, session.agent]);
  const ref = useRef<HTMLElement>(null);
  // Opened from a row far down the recent sessions, bring it into view.
  useEffect(() => ref.current?.scrollIntoView?.({ block: "nearest" }), []);
  const responses = read.data?.responses;
  const name = `Session ${sessionName(session.session, session.agent)}`;
  const peak = Math.max(0, ...(responses ?? []).map((r) => r.context_size));

  return (
    <section ref={ref} aria-label={name} className="usage-chart">
      <header className="usage-chart-head">
        <h3 className="usage-subhead">{name}</h3>
        {responses?.length ? (
          <span className="muted">
            {count(responses.length)} {responses.length === 1 ? "response" : "responses"} · peak {count(peak)} tokens
          </span>
        ) : null}
        <button className="btn usage-chart-close" onClick={() => navigate({ session: undefined })}>
          Close session
        </button>
      </header>
      {read.error && (
        <p role="alert" className="warning">
          {read.error.message}
        </p>
      )}
      {responses && !responses.length && <p className="muted">No responses for this session in the last {USAGE_KEPT_DAYS} days.</p>}
      {responses && responses.length > 0 && (
        <LineChart
          label="Context Size per response"
          xLabel="Response"
          values={responses.map((r) => r.context_size)}
          pointLabel={(i) => `Response ${i + 1}: ${count(responses[i].context_size)} tokens`}
          tip={(i) => <ResponseFigures n={i + 1} r={responses[i]} />}
        />
      )}
    </section>
  );
}

// ResponseFigures is one response's exact figures: when it came, its
// Context Size and its four token counts.
function ResponseFigures({ n, r }: { n: number; r: SessionUsageResponse }) {
  const rows: [string, string][] = [
    ["Context Size", `${count(r.context_size)} tokens`],
    ["Input", count(r.input_tokens)],
    ["Cache write", count(r.cache_write_tokens)],
    ["Cache read", count(r.cache_read_tokens)],
    ["Output", count(r.output_tokens)],
  ];
  return (
    <>
      <strong>Response {n}</strong> <span className="muted">{started(r.time)}</span>
      <dl>
        {rows.map(([label, figure]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{figure}</dd>
          </div>
        ))}
      </dl>
    </>
  );
}
