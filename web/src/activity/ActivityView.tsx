// The Activity view: the Activity Dashboard in a view of its own, opened
// from the Inbox's Admin section. It shows the dashboard whatever needs the
// human and whatever Focus Mode shows when nothing does, for one Project or
// every active one, chosen with its Projects picker. Back returns to the
// view it was opened from.
import type { Api } from "../api/api";
import { backLabel } from "../controls/backLabel";
import { Loading } from "../controls/Empty";
import { ProjectPicker } from "../controls/ProjectPicker";
import { ViewHeader } from "../controls/ViewHeader";
import type { ProjectRow } from "../api/types";
import { ActivityDashboard } from "../focus/ActivityDashboard";
import type { InboxData } from "../live";
import type { UiLocation } from "../url";
import { Button } from "../controls/Button";
import "../focus/focus.css";

export interface ActivityViewProps {
  api: Api;
  now: Date;
  // version is the live feed's, so the dashboard follows every change.
  version: number;
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function ActivityView({ api, now, version, data, loc, navigate }: ActivityViewProps) {
  const projects: ProjectRow[] = data?.projects ?? [];

  return (
    <main aria-label="Activity" className="activity view-shell">
      <ViewHeader
        title="Activity"
        left={<ProjectPicker projects={projects} value={loc.project} onChange={(project) => navigate({ project })} />}
        right={
          <Button  onClick={() => navigate(loc.back ?? { view: "inbox" })}>
            {backLabel(loc)}
          </Button>
        }
      />
      {data ? <ActivityDashboard api={api} now={now} version={version} data={data} loc={loc} navigate={navigate} /> : <Loading />}
    </main>
  );
}
