// SessionPopup is Usage's Session popup (Spec code-central-752): one
// session's Context Size, response by response, as a line chart, each
// point's exact figures on hover or focus, in a popup over the Context
// Size tab. It reads every day Usage keeps, whatever the range, and
// closes with ×, Escape or a click outside. A Subagent's popup links to
// its parent session's, which the address switches it to in place.
// SessionPopupBody is the popup over a read handed in.
import { USAGE_KEPT_DAYS, type SessionUsageFilter, type SessionUsageResponse } from "../api/types";
import { Loading } from "../controls/Empty";
import { ErrorBanner } from "../controls/ErrorBanner";
import { PopupShell } from "../controls/PopupShell";
import { count } from "../format";
import { useSessionUsage } from "../live";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { LineChart } from "./LineChart";
import { started, subagentName } from "./SessionsTable";
import "./usage.css";

export interface SessionPopupProps {
  session:SessionUsageFilter;
  // here is Usage's address, the session among it.
  here: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
  onClose(): void;
}

// SessionPopup reads the session and shows it. Key it by the session, so
// another session starts from nothing.
export function SessionPopup({ session, ...props }: SessionPopupProps) {
  const read = useSessionUsage(session);
  // A Subagent is named from the read, so a bookmarked popup names it
  // without the list; until it arrives, by its tool-use ID.
  const subagent = session.agent ? subagentName({ agent: session.agent, ...read.data }) : undefined;
  return <SessionPopupBody {...props} session={session} responses={read.data?.responses} subagent={subagent} error={read.error?.message} />;
}

export interface SessionPopupBodyProps extends SessionPopupProps {
  // responses are the session's, oldest first; left out while they load.
  responses?: SessionUsageResponse[];
  // subagent is the Subagent's name, for a Subagent's session.
  subagent?: string;
  // error is the read's failure, if it failed.
  error?: string;
}

export function SessionPopupBody({ session, responses, subagent, error, here, navigate, onClose }: SessionPopupBodyProps) {
  const parent: Partial<UiLocation> = { session: { project: session.project, session: session.session } };
  const peak = Math.max(0, ...(responses ?? []).map((r) => r.context_size));
  return (
    <PopupShell title={<span className="session-popup-title">Session {subagent ? `${session.session} · ${subagent}` : session.session}</span>} size="wide" onClose={onClose}>
      {(session.agent || !!responses?.length) && (
        <p className="session-popup-meta">
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
        </p>
      )}
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <div className="session-popup-chart">
        {!responses && !error && <Loading />}
        {responses && !responses.length && <p className="muted">No responses for this session in the {USAGE_KEPT_DAYS} days Usage keeps.</p>}
        {responses && responses.length > 0 && (
          <LineChart
            label="Context Size per response"
            xLabel="Response"
            values={responses.map((r) => r.context_size)}
            pointLabel={(i) => `Response ${i + 1}: ${count(responses[i].context_size)} tokens`}
            tip={(i) => <ResponseFigures n={i + 1} r={responses[i]} />}
          />
        )}
      </div>
    </PopupShell>
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
