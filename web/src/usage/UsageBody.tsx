// UsageBody is Usage's page below its header, presentational: the figures
// the view read, the sort of the calls table, the open tab and the open
// session's chart come in as props, and the view fetches them. The
// loading and error banners sit above the tab strip, and each tab's
// panel shows its part or says it has nothing to show. Its parts are
// components of their own in this folder.
import type { ReactNode } from "react";
import type { UsageOperation, UsageView } from "../api/types";
import { Empty, Loading } from "../controls/Empty";
import { ErrorBanner } from "../controls/ErrorBanner";
import { Tabs } from "../controls/Tabs";
import { USAGE_TABS, type UiLocation, type UsageTab } from "../url";
import { CallsTable, DEFAULT_SORT, nextSort, type Sort } from "./CallsTable";
import { KindsTable } from "./KindsTable";
import { SessionsTable, started, subagentName } from "./SessionsTable";
import { TokenChart } from "./TokenChart";
import "./usage.css";

export { DEFAULT_SORT, nextSort, started, subagentName };
export type { Sort };

// TAB_LABELS are the labels of Usage's tabs.
const TAB_LABELS: Record<UsageTab, string> = {
  tokens: "Token usage",
  context: "Context Size",
  calls: "Code Central calls",
};
const TABS = USAGE_TABS.map((value) => ({ value, label: TAB_LABELS[value] }));

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
  // tab is the open tab, and onTab switches it.
  tab: UsageTab;
  onTab(tab: UsageTab): void;
  // session is the open session's chart, shown above the tabs.
  session?: ReactNode;
}

export function UsageBody({ data, error, days, scopeName, here, projectName, navigate, sort, onSort, tab, onTab, session }: UsageBodyProps) {
  const over = `in ${scopeName} over the last ${days} days`;
  return (
    <div className="view-body">
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {!data && !error && <Loading />}
      {session}
      <Tabs label="Usage" tabs={TABS} value={tab} onChange={onTab}>
        {data && tab === "tokens" &&
          (data.kinds.length ? (
            <TokenChart days={data.days} kinds={data.kinds.map((k) => k.kind)} />
          ) : (
            <Empty>No token usage {over}. Only sessions the Factory runs are counted.</Empty>
          ))}
        {data && tab === "context" &&
          (data.sessions.length ? (
            <>
              <KindsTable kinds={data.kinds} />
              <SessionsTable sessions={data.sessions} here={here} projectName={projectName} navigate={navigate} />
            </>
          ) : (
            <Empty>No sessions {over}. Only sessions the Factory runs are counted.</Empty>
          ))}
        {data && tab === "calls" &&
          (data.operations.length ? (
            <CallsTable operations={data.operations} sort={sort} onSort={onSort} />
          ) : (
            <Empty>No Code Central calls {over}.</Empty>
          ))}
      </Tabs>
    </div>
  );
}
