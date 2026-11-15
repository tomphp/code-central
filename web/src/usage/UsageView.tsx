// Usage (Spec code-central-201): what agents have consumed in one
// Project, or in all of them, over the last 7, 30 or 90 days, in three
// tabs in this order (Spec code-central-752): Token usage, a bar per day
// filtered by kind of session; Context Size, each kind of session's peaks
// and then the recent sessions, each a main session with its Subagents
// nested under it, each linking to its Ticket and opening its Session
// popup; and the Code Central calls table, one row per MCP operation,
// sortable by each column. The scope and range in the header apply to
// every tab. The open tab is in the address as tab= (none = Token usage),
// and so is the open session, whose Session popup shows over the Context
// Size tab whatever tab= says. ×, Escape or a click outside closes it to
// Usage as it was, on the Context Size tab. Usage loads when opened and
// whenever its scope or range changes, never on switching tab or opening
// a session, and never live. Back returns to the view it was opened from.
// The page below the header is UsageBody.
import { useState } from "react";
import type { Api } from "../api/api";
import { ProjectPicker } from "../controls/ProjectPicker";
import { useProjectScope } from "../projectScope";
import { ViewHeader } from "../controls/ViewHeader";
import { sessionKey, USAGE_DAYS, type ProjectRow, type UsageDays } from "../api/types";
import { useFetch } from "../live";
import { USAGE_TABS, type UiLocation } from "../url";
import { SessionPopup } from "./SessionPopup";
import { DEFAULT_SORT, nextSort, UsageBody } from "./UsageBody";
import { Button } from "../controls/Button";
import { backLabel } from "../controls/backLabel";
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
  const { project, sole, resolved } = useProjectScope();
  const days = loc.days ?? USAGE_DAYS[0];
  // A session in the address opens over the Context Size tab, whatever tab= says.
  const tab = loc.session ? "context" : (loc.tab ?? USAGE_TABS[0]);
  const usage = useFetch(resolved ? () => api.usage({ project, days }) : null, [api, resolved, project, days]);
  const [sort, setSort] = useState(DEFAULT_SORT);
  const scopeName = project ? projectName(projects, project) : "All Projects";

  return (
    <main aria-label="Usage" className="usage view-shell">
      <ViewHeader
        title="Usage"
        left={
          <>
            <ProjectPicker projects={projects} value={project} withoutAll={sole} onChange={(project) => navigate({ project })} />
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
        tab={tab}
        // The Session popup covers the tabs, but a switch under it would close
        // the session too, since it shows over Context Size only.
        onTab={(to) => {
          if (to !== tab) navigate({ tab: to, session: undefined });
        }}
      />
      {loc.session && (
        <SessionPopup
          // Keyed so another session, such as a Subagent's parent, starts from nothing.
          key={sessionKey(loc.session)}
          api={api}
          session={loc.session}
          here={loc}
          navigate={navigate}
          // It closes to the Context Size tab it showed over, though the address may have left the tab out.
          onClose={() => navigate({ session: undefined, tab: "context" })}
        />
      )}
    </main>
  );
}
