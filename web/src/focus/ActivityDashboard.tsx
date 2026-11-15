// The Activity Dashboard: what Focus Mode shows instead of All clear when
// the human has chosen it, and the Activity view's body. Its Tickets come
// from the Inbox data Focus Mode already has; this view adds what the
// body needs besides. It follows the live feed's ticket.changed notices,
// so an entry pulses when its Ticket changes, which includes each new
// Session Event. When a Project in scope that isn't Archived has CI
// Tracking on, it also reads the running CI Runs, again on every
// live-feed change (code-central-277). ActivityDashboardBody shows it all.
import { useEffect, useState } from "react";
import type { Api } from "../api/api";
import { useFetch, type InboxData } from "../live";
import { useProjectScope } from "../projectScope";
import type { UiLocation } from "../url";
import { ActivityDashboardBody } from "./ActivityDashboardBody";

export interface ActivityDashboardProps {
  api: Api;
  now: Date;
  // version is the live feed's, so the running CI Runs follow every change.
  version: number;
  data: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function ActivityDashboard({ api, now, version, data, loc, navigate }: ActivityDashboardProps) {
  // changes counts the ticket.changed notices for each Ticket since the
  // dashboard showed: the version number alone doesn't say which Ticket
  // changed, so the dashboard follows the live feed itself.
  const [changes, setChanges] = useState<Record<string, number>>({});
  useEffect(
    () =>
      api.subscribe({
        onEvent: (e) => e.type === "ticket.changed" && setChanges((c) => ({ ...c, [e.id]: (c[e.id] ?? 0) + 1 })),
      }),
    [api],
  );
  const { project } = useProjectScope();
  const ciTracked = data.projects.some((p) => !p.archived && p.ci.tracking && (!project || p.key === project));
  const ciRuns = useFetch(ciTracked ? () => api.ciOverview({ project, running: true }) : null, [api, project, ciTracked, version]);

  return (
    <ActivityDashboardBody now={now} data={data} changes={changes} ciRuns={ciTracked ? ciRuns : undefined} loc={loc} project={project} navigate={navigate} />
  );
}
