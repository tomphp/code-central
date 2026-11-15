// UsageBody is Usage's page below its header, presentational: the figures
// the view read, the sort of the calls table and the open session's chart
// come in as props, and the view fetches them. Its parts are components of
// their own in this folder.
import type { ReactNode } from "react";
import type { UsageOperation, UsageView } from "../api/types";
import { Empty, Loading } from "../controls/Empty";
import { ErrorBanner } from "../controls/ErrorBanner";
import type { UiLocation } from "../url";
import { CallsTable, DEFAULT_SORT, nextSort, type Sort } from "./CallsTable";
import { KindsTable } from "./KindsTable";
import { SessionsTable, started, subagentName } from "./SessionsTable";
import { TokenChart } from "./TokenChart";
import "./usage.css";

export { DEFAULT_SORT, nextSort, started, subagentName };
export type { Sort };

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
