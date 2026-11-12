// The Activity view: the Activity Dashboard in a view of its own, opened
// from the Inbox's Admin section. It shows the dashboard whatever needs the
// human and whatever Focus Mode shows when nothing does, for one Project or
// every active one, chosen with its Projects picker. Back returns to the
// view it was opened from.
import type { Api } from "../api/api";
import type { ProjectRow } from "../api/types";
import { ActivityDashboard } from "../focus/ActivityDashboard";
import type { InboxData } from "../live";
import type { UiLocation } from "../url";
import "../focus/focus.css";
import "./activity.css";

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
  const back = loc.back?.view === "focus" ? "focus" : "inbox";

  return (
    <main aria-label="Activity" className="activity">
      <header className="activity-head">
        <h1>Activity</h1>
        <label>
          <span className="activity-picker-label">Projects</span>
          <select value={loc.project ?? ""} onChange={(e) => navigate({ project: e.target.value || undefined })}>
            <option value="">All Projects</option>
            {projects.map((p) => (
              <option key={p.key} value={p.key}>
                {p.archived ? `${p.display_name} (Archived)` : p.display_name}
              </option>
            ))}
            {loc.project && !projects.some((p) => p.key === loc.project) && <option value={loc.project}>{loc.project}</option>}
          </select>
        </label>
        <button className="btn activity-back" onClick={() => navigate(loc.back ?? { view: "inbox" })}>
          {back === "focus" ? "Back to Focus Mode" : "Back to the Inbox"}
        </button>
      </header>
      {data ? <ActivityDashboard api={api} now={now} version={version} data={data} loc={loc} navigate={navigate} /> : <p className="muted">Loading…</p>}
    </main>
  );
}
