// Focus Mode's body: its layout, from the session it is given. The header
// names the Project in scope, says how many Focus Items are left and holds
// + New ▾ and Exit; under it, the session's note, then the Focus Item on
// screen, or All clear (or the dashboard in its place), or the offer to go
// through the skipped ones again, or Loading… while there is nothing yet.
// Its root's class scopes the Focus styles, so the panels inside read
// larger here and nowhere else.
//
// Submitting from + New ▾ says a brief "Requirement added" or "Map added"
// in the session's note; there's only ever one note, the latest.
import type { ReactNode } from "react";
import type { Api } from "../api/api";
import { Button } from "../controls/Button";
import { Count } from "../controls/Count";
import { Empty, Loading } from "../controls/Empty";
import { ProjectName } from "../controls/ProjectName";
import { ViewHeader } from "../controls/ViewHeader";
import type { InboxData } from "../live";
import { NewSplit } from "../requirement/NewSplit";
import type { UiLocation } from "../url";
import { FocusItemView, inboxOf } from "./FocusItemView";
import type { FocusSession } from "./session";
import "./focus.css";

// FocusBodySession is the session FocusBody lays out: what to show, and
// the callbacks that change it.
export type FocusBodySession = Omit<FocusSession, "keys">;

export interface FocusBodyProps {
  // api is what + New ▾ submits through; the item on screen acts through
  // the session's.
  api: Api;
  now: Date;
  // data is the Inbox data, none while it loads.
  data?: InboxData;
  session: FocusBodySession;
  // dashboard shows in place of All clear, when the human chose the
  // Activity Dashboard.
  dashboard?: ReactNode;
  // loc's Project is the one in scope; none means All Projects.
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function FocusBody({ api, now, data, session, dashboard, loc, navigate }: FocusBodyProps) {
  const project = data?.projects.find((p) => p.key === loc.project);
  const { item } = session;

  return (
    <main aria-label="Focus Mode" className="focus">
      <ViewHeader
        left={
          <>
            {loc.project && (
              <span className="focus-project">
                <ProjectName name={project?.display_name ?? loc.project} projectKey={loc.project} />
              </span>
            )}
            {data && !session.clear && <Count n={session.remaining} of="left" />}
          </>
        }
        right={
          <>
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
            <Button onClick={() => navigate({ view: "inbox" })}>Exit</Button>
          </>
        }
      />
      {session.note && (
        <p role="status" className="focus-note">
          {session.note}
        </p>
      )}
      {!data ? (
        <Loading />
      ) : session.clear && dashboard ? (
        dashboard
      ) : session.clear ? (
        <section aria-label="All clear" className="focus-clear">
          <h2>All clear</h2>
          <Empty>Nothing needs you.</Empty>
        </section>
      ) : session.onlySkipped ? (
        <section aria-label="Only skipped Focus Items left" className="focus-clear">
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
        <Loading />
      )}
    </main>
  );
}
