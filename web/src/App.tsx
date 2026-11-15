import { useEffect, useMemo } from "react";
import { IdLinksContext, useIdKind } from "./idLinks";
import type { Api } from "./api/api";
import { projectOf } from "./api/api";
import { ChatDock } from "./chat/ChatDock";
import { Focus } from "./focus/Focus";
import { Inbox } from "./inbox/Inbox";
import type { FocusClear } from "./api/types";
import { Live, useApi, useInbox, useOptions, useSpec, useTicket, useVersion } from "./live";
import { OptionsView } from "./options/OptionsView";
import { ProjectOptions } from "./options/ProjectOptions";
import { NeedsMeProvider } from "./needsme/context";
import { needsMeOf } from "./needsme/model";
import { defaultTicket, progress } from "./requirement/progress";
import { useNeedsMeTitle } from "./title";
import { projectScopeOf, ProjectScopeContext } from "./projectScope";
import { useUiLocation } from "./url";
import { UsageView } from "./usage/UsageView";
import { CIView } from "./ci/CIView";
import { ActivityView } from "./activity/ActivityView";
import { BoardView } from "./board/BoardView";
import { FocusPopup } from "./board/FocusPopup";
import { usePopupTrigger } from "./focus/session";

export interface AppProps {
  api: Api;
  // now is the clock "how long it waited" is measured against; tests pin it.
  now?: () => Date;
}

// App mounts <Live>, the UI's one change feed, and reads through it.
export function App({ api, now = () => new Date() }: AppProps) {
  return (
    <Live api={api}>
      <Views now={now} />
    </Live>
  );
}

function Views({ now }: { now: () => Date }) {
  const api = useApi();
  const version = useVersion();
  const [loc, navigate, replace] = useUiLocation();
  const inbox = useInbox();
  const options = useOptions();
  // While the Options load, or when the last read failed, Focus Mode shows All clear.
  const focusClear: FocusClear = (!options.error && options.data?.focus_clear) || "all_clear";
  const spec = useSpec(loc.requirement && projectOf(loc.requirement), loc.requirement);
  const requirement = spec.data?.id === loc.requirement && spec.data?.requirement ? { ...spec.data, requirement: spec.data.requirement } : undefined;
  const map = spec.data?.id === loc.requirement && spec.data?.map ? { ...spec.data, map: spec.data.map } : undefined;
  const plain = spec.data?.id === loc.requirement && !requirement && !map ? spec.data : undefined;
  // The Needs Me read model every view asks, built once per Inbox load.
  const needsMe = useMemo(() => needsMeOf(inbox.data), [inbox.data]);
  // Within a Requirement with no Ticket chosen, the pane opens on its default one.
  const selected =
    loc.ticket ?? (requirement ? defaultTicket(progress(requirement.requirement, requirement.tickets ?? [], needsMe.reasonOf)) : undefined);
  const ticket = useTicket(selected && projectOf(selected), selected);
  useNeedsMeTitle(inbox.data && needsMe.count());
  // An ID Link asks the Inbox to open an ID whose kind isn't known; once it
  // is, the address is replaced with the Spec or Ticket it selects, so Back
  // skips the open= address.
  const opening = loc.view === "inbox" ? loc.open : undefined;
  const openKind = useIdKind(api, opening, needsMe, !!inbox.data);
  useEffect(() => {
    if (!opening || !openKind) return;
    replace({ open: undefined, ...(openKind === "spec" ? { requirement: opening } : { ticket: opening }) });
  }, [opening, openKind, replace]);
  const idLinks = useMemo(() => ({ loc, navigate }), [loc, navigate]);
  // Under Focus Pop-up, the Focus popup opens by itself over the view
  // showing, scoped to its Project, from the same read model's Focus
  // Items. Until the Options first load Focus Pop-up is off; a failed read
  // later keeps the last one, so an open popup isn't lost to it.
  const scope = projectScopeOf(loc.project, inbox.data?.projects, !!inbox.error);
  const popup = usePopupTrigger({
    option: options.data?.focus_popup ?? "off",
    view: loc.view,
    project: scope.project,
    needsMe: inbox.data && needsMe,
  });

  return (
    <ProjectScopeContext.Provider value={scope}>
      <NeedsMeProvider value={needsMe}>
        <IdLinksContext.Provider value={idLinks}>
        {view()}
        <ChatDock project={inbox.data?.projects.find((p) => p.key === scope.project)} />
        {popup.open && inbox.data && (
          <FocusPopup mode="queue" api={api} now={now()} version={version} data={inbox.data} project={scope.project} onClose={popup.close} navigate={navigate} />
        )}
        </IdLinksContext.Provider>
      </NeedsMeProvider>
    </ProjectScopeContext.Provider>
  );

  // Each view is a layout over the same live data, the Board included;
  // a later one is another case here, reusing the shared Ticket pane and
  // panels.
  function view() {
    const banner = inbox.error && <p className="banner">{inbox.error.message}</p>;
    switch (loc.view) {
      case "focus":
        return (
          <>
            {banner}
            <Focus api={api} now={now()} version={version} data={inbox.data} focusClear={focusClear} loc={loc} navigate={navigate} />
          </>
        );
      case "options":
        return <OptionsView api={api} options={options} loc={loc} navigate={navigate} />;
      case "usage":
        return <UsageView projects={inbox.data?.projects} loc={loc} navigate={navigate} />;
      case "ci":
        return <CIView now={now()} projects={inbox.data?.projects} loc={loc} navigate={navigate} />;
      case "activity":
        return (
          <>
            {banner}
            <ActivityView api={api} now={now()} version={version} data={inbox.data} loc={loc} navigate={navigate} />
          </>
        );
      case "board":
        return (
          <>
            {banner}
            <BoardView api={api} now={now()} version={version} data={inbox.data} loc={loc} navigate={navigate} onFocusPopupGone={popup.dismiss} />
          </>
        );
      case "project-options":
        return (
          <>
            {banner}
            <ProjectOptions api={api} now={now()} projects={inbox.data?.projects} project={scope.project} navigate={navigate} />
          </>
        );
      case "inbox":
        return (
          <>
            {banner}
            <Inbox
              api={api}
              now={now()}
              data={inbox.data}
              loc={{ ...loc, ticket: selected }}
              navigate={navigate}
              requirement={requirement}
              map={map}
              spec={plain}
              requirementError={spec.error && spec.data?.id !== loc.requirement ? spec.error.message : undefined}
              ticket={ticket.data}
              ticketError={ticket.error && ticket.data?.id !== selected ? ticket.error.message : undefined}
            />
          </>
        );
    }
  }
}
