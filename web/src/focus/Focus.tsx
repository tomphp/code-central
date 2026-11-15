// Focus Mode: one Focus Item at a time from Needs Me, filling the window
// with no nav, list or side pane. A view over the Focus session, which
// picks what to show and notes what happened to the item on screen; the
// data comes from live.tsx, as the Inbox's does. FocusBody lays it out. It
// never closes by itself.
import type { Api } from "../api/api";
import type { FocusClear } from "../api/types";
import type { InboxData } from "../live";
import { useProjectScope } from "../projectScope";
import type { UiLocation } from "../url";
import { ActivityDashboard } from "./ActivityDashboard";
import { FocusBody } from "./FocusBody";
import { useFocusSession } from "./session";

export interface FocusProps {
  api: Api;
  now: Date;
  version: number;
  data?: InboxData;
  // focusClear is what to show when nothing needs the human: All clear,
  // or the Activity Dashboard.
  focusClear: FocusClear;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function Focus({ api, now, version, data, focusClear, loc, navigate }: FocusProps) {
  // The session lives for this visit only: leaving Focus Mode or reloading
  // forgets its skips.
  const { project } = useProjectScope();
  const session = useFocusSession({ api, version, project });
  const dashboard =
    data && focusClear === "activity_dashboard" ? (
      <ActivityDashboard api={api} now={now} version={version} data={data} loc={loc} navigate={navigate} />
    ) : undefined;

  return <FocusBody api={api} now={now} data={data} session={session} dashboard={dashboard} loc={loc} project={project} navigate={navigate} />;
}
