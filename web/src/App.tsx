import type { Api } from "./api/api";
import { projectOf } from "./api/api";
import { Focus } from "./focus/Focus";
import { Inbox } from "./inbox/Inbox";
import type { FocusClear } from "./api/types";
import { useInboxData, useLiveVersion, useOptions, useSpec, useTicket } from "./live";
import { OptionsView } from "./options/OptionsView";
import { ProjectOptions } from "./options/ProjectOptions";
import { defaultTicket, needsMeCount, progress, reasonsOf } from "./requirement/progress";
import { useNeedsMeTitle } from "./title";
import { useUiLocation } from "./url";
import { UsageView } from "./usage/UsageView";
import { CIView } from "./ci/CIView";
import { ActivityView } from "./activity/ActivityView";
import { BoardView } from "./board/BoardView";
import { FocusPopup } from "./board/FocusPopup";
import { usePopupTrigger } from "./focus/popupTrigger";

export interface AppProps {
  api: Api;
  // now is the clock "how long it waited" is measured against; tests pin it.
  now?: () => Date;
}

export function App({ api, now = () => new Date() }: AppProps) {
  const [loc, navigate] = useUiLocation();
  const version = useLiveVersion(api);
  const inbox = useInboxData(api, version);
  const options = useOptions(api, version);
  // While the Options load, or when the last read failed, Focus Mode shows All clear.
  const focusClear: FocusClear = (!options.error && options.data?.focus_clear) || "all_clear";
  const spec = useSpec(api, loc.requirement && projectOf(loc.requirement), loc.requirement, version);
  const requirement = spec.data?.id === loc.requirement && spec.data?.requirement ? { ...spec.data, requirement: spec.data.requirement } : undefined;
  const map = spec.data?.id === loc.requirement && spec.data?.map ? { ...spec.data, map: spec.data.map } : undefined;
  const plain = spec.data?.id === loc.requirement && !requirement && !map ? spec.data : undefined;
  const reasons = reasonsOf(inbox.data?.needsMe);
  // Within a Requirement with no Ticket chosen, the pane opens on its default one.
  const selected =
    loc.ticket ?? (requirement ? defaultTicket(progress(requirement.requirement, requirement.tickets, reasons)) : undefined);
  const ticket = useTicket(api, selected && projectOf(selected), selected, version);
  useNeedsMeTitle(inbox.data && needsMeCount(inbox.data.needsMe));
  // Under Focus Pop-up, the Focus popup opens by itself over the view
  // showing, scoped to its Project, from the same live Needs Me and the
  // Projects' Focus Kinds. Until the Options first load Focus Pop-up is
  // off; a failed read later keeps the last one, so an open popup isn't
  // lost to it.
  const popup = usePopupTrigger({
    option: options.data?.focus_popup ?? "off",
    view: loc.view,
    project: loc.project,
    needsMe: inbox.data?.needsMe,
    projects: inbox.data?.projects ?? [],
  });

  return (
    <>
      {view()}
      {popup.open && inbox.data && (
        <FocusPopup mode="queue" api={api} now={now()} version={version} data={inbox.data} project={loc.project} onClose={popup.close} navigate={navigate} />
      )}
    </>
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
        return <UsageView api={api} projects={inbox.data?.projects} loc={loc} navigate={navigate} />;
      case "ci":
        return <CIView api={api} now={now()} version={version} projects={inbox.data?.projects} loc={loc} navigate={navigate} />;
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
            <ProjectOptions api={api} now={now()} projects={inbox.data?.projects} project={loc.project} navigate={navigate} />
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
