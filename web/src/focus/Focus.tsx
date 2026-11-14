// Focus Mode: one Focus Item at a time from Needs Me, filling the window
// with no nav, list or side pane. A layout only, over the Focus session,
// which picks what to show and notes what happened to the item on screen;
// the data comes from live.ts, as the Inbox's does. Its root's class
// scopes the Focus styles, so the panels inside read larger here and
// nowhere else. It never closes by itself.
//
// Submitting from + New ▾ says a brief "Requirement added" or "Map added"
// in the session's note; there's only ever one note, the latest.
import type { Api } from "../api/api";
import type { FocusClear } from "../api/types";
import type { InboxData } from "../live";
import { NewSplit } from "../requirement/NewSplit";
import type { UiLocation } from "../url";
import { ActivityDashboard } from "./ActivityDashboard";
import { FocusItemView, inboxOf } from "./FocusItemView";
import { useFocusSession } from "./session";
import { Button } from "../controls/Button";
import "./focus.css";

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

export function Focus({
  api,
  now,
  version,
  data,
  focusClear,
  loc,
  navigate,
}: FocusProps) {
  const project = data?.projects.find((p) => p.key === loc.project);
  // The session lives for this visit only: leaving Focus Mode or reloading
  // forgets its skips.
  const session = useFocusSession({ api, version, project: loc.project });
  const { item } = session;

  return (
    <main aria-label="Focus Mode" className="focus">
      <header className="focus-head">
        {loc.project && (
          <span className="focus-project">
            {project?.display_name ?? loc.project}{" "}
            <span className="muted">{loc.project}</span>
          </span>
        )}
        {data && !session.clear && (
          <span className="focus-left">{session.remaining} left</span>
        )}
        {/* Submitting goes nowhere: whatever is on screen stays, skips and
            all, with a note on every screen. */}
        <div className="focus-new">
          <NewSplit
            variant="plain"
            api={api}
            projects={data?.projects.filter((p) => !p.archived) ?? []}
            project={loc.project}
            onSubmitted={({ kind }) => session.say(`${kind} added`)}
          />
        </div>
        <Button  onClick={() => navigate({ view: "inbox" })}>
          Exit
        </Button>
      </header>
      {session.note && (
        <p role="status" className="focus-note">
          {session.note}
        </p>
      )}
      {!data ? (
        <p className="focus-loading">Loading…</p>
      ) : session.clear && focusClear === "activity_dashboard" ? (
        <ActivityDashboard
          api={api}
          now={now}
          version={version}
          data={data}
          loc={loc}
          navigate={navigate}
        />
      ) : session.clear ? (
        <section aria-label="All clear" className="focus-clear">
          <h2>All clear</h2>
          <p>Nothing needs you.</p>
        </section>
      ) : session.onlySkipped ? (
        <section
          aria-label="Only skipped Focus Items left"
          className="focus-clear"
        >
          <p>You skipped {session.remaining}, go through them again?</p>
          <Button variant="primary" onClick={session.again}>
            Go through them again
          </Button>
        </section>
      ) : item ? (
        <FocusItemView
          key={item.key}
          api={session.api}
          now={now}
          data={data}
          item={item}
          ticket={session.ticket}
          navigate={navigate}
          skip={session.skip}
          inbox={inboxOf(item.row, loc.project)}
        />
      ) : (
        <p className="focus-loading">Loading…</p>
      )}
    </main>
  );
}
