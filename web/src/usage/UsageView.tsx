// Usage (Spec code-central-201): what agents have consumed in one
// Project, or in all of them, over the last 7, 30 or 90 days: the Code
// Central calls table, one row per MCP operation, sortable by each
// column; and Context Size, each kind of session's peaks and the recent
// sessions, each a main session with its Subagents nested under it, each
// linking to its Ticket and opening its Context Size, response by
// response, on a line chart; and token usage, a bar per day
// filtered by kind of session. It loads when opened and whenever its
// scope, range or session changes, never live. Back returns to the view
// it was opened from. The page below the header is UsageBody.
import { useEffect, useRef, useState } from "react";
import type { Api } from "../api/api";
import { ProjectPicker } from "../controls/ProjectPicker";
import { ViewHeader } from "../controls/ViewHeader";
import { sessionKey, USAGE_DAYS, USAGE_KEPT_DAYS, type ProjectRow, type UsageDays, type SessionUsageFilter, type SessionUsageResponse } from "../api/types";
import { count } from "../format";
import { useFetch } from "../live";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { LineChart } from "./LineChart";
import { DEFAULT_SORT, nextSort, started, subagentName, UsageBody } from "./UsageBody";
import { Button } from "../controls/Button";
import { backLabel } from "../controls/backLabel";
import { ErrorBanner } from "../controls/ErrorBanner";
import { projectName } from "../controls/ProjectName";
import "./usage.css";
import { Select } from "../controls/Select";

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
  const [sort, setSort] = useState(DEFAULT_SORT);
  const scopeName = loc.project ? projectName(projects, loc.project) : "All Projects";

  return (
    <main aria-label="Usage" className="usage view-shell">
      <ViewHeader
        title="Usage"
        left={
          <>
            <ProjectPicker projects={projects} value={loc.project} onChange={(project) => navigate({ project })} />
            <label>
              <span className="view-picker-label">Range</span>
              <Select
                value={String(days)}
                onChange={(v) => navigate({ days: Number(v) as UsageDays })}
                options={USAGE_DAYS.map((d) => ({ value: String(d), label: `Last ${d} days` }))}
              />
            </label>
          </>
        }
        right={<Button onClick={() => navigate(loc.back ?? { view: "inbox" })}>{backLabel(loc)}</Button>}
      />
      <UsageBody
        data={usage.data}
        error={usage.error?.message}
        days={days}
        scopeName={scopeName}
        here={loc}
        projectName={(key) => projectName(projects, key)}
        navigate={navigate}
        sort={sort}
        onSort={(key) => setSort((s) => nextSort(s, key))}
        // The open session, keyed so another session starts from nothing.
        session={loc.session && <SessionChart key={sessionKey(loc.session)} api={api} session={loc.session} here={loc} navigate={navigate} />}
      />
    </main>
  );
}

// sessionName is a session's ID, with its subagent's after it.
function sessionName(session: string, agent?: string): string {
  return agent ? `${session} · ${agent}` : session;
}

interface SessionChartProps {
  api: Api;
  session: SessionUsageFilter;
  here: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

// SessionChart is one session's Context Size, response by response, as
// a line chart, each point's exact figures on hover. It reads every day
// Usage keeps, whatever the range, and closes to Usage as it was.
function SessionChart({ api, session, here, navigate }: SessionChartProps) {
  const read = useFetch(() => api.sessionUsage(session), [api, session.project, session.session, session.agent]);
  const ref = useRef<HTMLElement>(null);
  // Opened from a row far down the recent sessions, bring it into view.
  useEffect(() => {
    ref.current?.scrollIntoView?.({ block: "nearest" });
  }, []);
  const responses = read.data?.responses;
  // A Subagent is named from the read, so a bookmarked chart names it
  // without the list; until it arrives, by its tool-use ID.
  const subagent = session.agent ? subagentName({ agent: session.agent, ...read.data }) : undefined;
  const name = `Session ${sessionName(session.session, subagent)}`;
  const parent: Partial<UiLocation> = { session: { project: session.project, session: session.session } };
  const peak = Math.max(0, ...(responses ?? []).map((r) => r.context_size));

  return (
    <section ref={ref} aria-label={name} className="usage-chart">
      <header className="usage-chart-head">
        <h3 className="usage-subhead">{name}</h3>
        {session.agent && (
          <a href={hrefFor({ ...here, ...parent })} onClick={clickTo(navigate, parent)}>
            Parent session {session.session}
          </a>
        )}
        {responses?.length ? (
          <span className="muted">
            {count(responses.length)} {responses.length === 1 ? "response" : "responses"} · peak {count(peak)} tokens
          </span>
        ) : null}
        <Button className="usage-chart-close" onClick={() => navigate({ session: undefined })}>
          Close session
        </Button>
      </header>
      {read.error && <ErrorBanner>{read.error.message}</ErrorBanner>}
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
