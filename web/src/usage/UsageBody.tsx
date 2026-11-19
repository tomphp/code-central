// UsageBody is Usage's page below its header, presentational: the figures
// the view read, the sort of the calls table and the open tab come in as
// props, and the view fetches them. The loading banner sits above the
// tab strip, the error banner below it, and each tab's panel shows its part or says it has
// nothing to show. The Storage tab's banners are its own read's, the
// other tabs' Usage's. Its parts are components of their own in this folder;
// the Session popup the view shows over it is SessionPopup.
import type { StorageView, UsageOperation, UsageView } from "../api/types";
import { Empty, Loading } from "../controls/Empty";
import { ErrorBanner } from "../controls/ErrorBanner";
import { Tabs } from "../controls/Tabs";
import { USAGE_TABS, type UiLocation, type UsageTab } from "../url";
import { CallsTable, DEFAULT_SORT, nextSort, type Sort } from "./CallsTable";
import { KindsTable } from "./KindsTable";
import { SessionsTable } from "./SessionsTable";
import { StoragePanel } from "./StoragePanel";
import { TokenChart } from "./TokenChart";
import "./usage.css";

export { DEFAULT_SORT, nextSort };
export type { Sort };

// TAB_LABELS are the labels of Usage's tabs.
const TAB_LABELS: Record<UsageTab, string> = {
  tokens: "Token usage",
  context: "Context Size",
  calls: "Code Central calls",
  storage: "Storage",
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
  // here is Usage's address, the session open in its Session popup among it.
  here: UiLocation;
  projectName(key: string): string;
  navigate(patch: Partial<UiLocation>): void;
  sort: Sort;
  onSort(key: keyof UsageOperation): void;
  // tab is the open tab, and onTab switches it.
  tab: UsageTab;
  onTab(tab: UsageTab): void;
  // storage is the Storage tab's read, left out while it loads, and
  // storageError its failure; project is the scope's Project, which
  // narrows its counts, and now the clock its services' renewals are
  // measured against.
  storage?: StorageView;
  storageError?: string;
  project?: string;
  now: Date;
}

export function UsageBody(props: UsageBodyProps) {
  const { data, days, scopeName, here, projectName, navigate, sort, onSort, tab, onTab, storage, project, now } = props;
  const over = `in ${scopeName} over the last ${days} days`;
  const loaded = tab === "storage" ? storage : data;
  const error = tab === "storage" ? props.storageError : props.error;
  return (
    <div className="view-body">
      {!loaded && !error && <Loading />}
      <Tabs label="Usage" tabs={TABS} value={tab} onChange={onTab}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        {storage && tab === "storage" && <StoragePanel storage={storage} project={project} now={now} />}
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
