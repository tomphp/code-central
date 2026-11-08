// Focus Mode: one Focus Item at a time from Needs Me, filling the window
// with no nav, list or side pane. A layout only: the data comes from
// live.ts, as the Inbox's does. Its root's class scopes the Focus styles.
import type { InboxData } from "../live";
import type { UiLocation } from "../url";
import "./focus.css";

export interface FocusProps {
  data?: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function Focus({ data, loc, navigate }: FocusProps) {
  const project = data?.projects.find((p) => p.key === loc.project);
  // Unlike the Inbox, Focus Mode takes Stage Tickets as themselves, so
  // in scope is every Needs Me row in the Project.
  const rows = (data?.needsMe.groups ?? []).flatMap((g) => g.tickets).filter((r) => !loc.project || r.project === loc.project);

  return (
    <main aria-label="Focus Mode" className="focus">
      <header className="focus-head">
        {loc.project && (
          <span className="focus-project">
            {project?.display_name ?? loc.project} <span className="muted">{loc.project}</span>
          </span>
        )}
        <button className="btn focus-exit" onClick={() => navigate({ view: "inbox" })}>
          Exit
        </button>
      </header>
      {!data ? (
        <p className="focus-loading">Loading…</p>
      ) : (
        rows.length === 0 && (
          <section aria-label="All clear" className="focus-clear">
            <h2>All clear</h2>
            <p>Nothing needs you.</p>
          </section>
        )
      )}
    </main>
  );
}
